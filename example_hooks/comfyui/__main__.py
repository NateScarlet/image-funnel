#!/usr/bin/env python
# -*- coding: utf-8 -*-

import json
import os
import sys

from typing import Dict, List, Tuple, Any, Optional, Set, Iterator, cast
import logging
import argparse

from graphql_utils import fetch_images
from .workflow_prompt_pair import WorkflowPromptPair
from .filename_manager import FilenameManager
from .weight_manager import WeightManager
from .prompt_locator import REGION_START_RE
from .command_handlers import COMMAND_HANDLERS, CommandContext
from .config import ComfyUIConfig

_LOGGER = logging.getLogger(__name__)


def _write_action_override(action: str, action_path: str) -> None:
    """向 action_path 文件写入操作覆盖，通知 Runner 跳过默认行为"""
    if not action_path:
        raise ValueError("action_path is empty")
    with open(action_path, "w", encoding="utf-8") as f:
        f.write(action)


# 捕获 PIL 导入错误并给出清晰提示
try:
    from PIL import Image
except ImportError:
    print(
        "Error: Missing Pillow library. Please install it in your Python environment to handle image metadata:",
        file=sys.stderr,
    )
    print("      pip install Pillow", file=sys.stderr)
    sys.exit(1)


def is_under_directory(child_path: str, parent_path: str) -> bool:
    """
    判断 child_path 是否在 parent_path 内部（或为同一路径）。
    支持 Windows 下不区分大小写的包含关系判断。
    """
    child_norm = os.path.normcase(os.path.abspath(child_path))
    parent_norm = os.path.normcase(os.path.abspath(parent_path))
    try:
        common = os.path.normcase(os.path.commonpath([parent_norm, child_norm]))
        return common == parent_norm
    except ValueError:
        return False


def get_relative_output_dir(
    image_path: str, comfyui_output_dir_env: str, hook_output_dir: str
) -> str:
    """
    计算并返回目标输出目录相对于 ComfyUI 输出根目录的相对路径（使用正斜杠）。
    如果路径校验失败，或者找不到根目录，则抛出 ValueError。
    """
    image_path_abs = os.path.abspath(image_path)

    # 1. 寻找 comfyui_output_dir (输出根目录)
    if comfyui_output_dir_env:
        comfyui_output_dir = os.path.abspath(comfyui_output_dir_env)
        # 验证图片是否在 comfyui_output_dir 下
        if not is_under_directory(image_path_abs, comfyui_output_dir):
            raise ValueError(
                f"Image '{image_path}' is not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
            )
    else:
        # 从右向左寻找最后一个名为 "output" 的目录
        curr = os.path.dirname(image_path_abs)
        comfyui_output_dir = None
        while True:
            parent, name = os.path.split(curr)
            if not name:  # 到了根目录
                break
            if name.lower() == "output":
                comfyui_output_dir = curr
                break
            curr = parent

        if not comfyui_output_dir:
            raise ValueError(
                f"Could not find any directory named 'output' in the path of image '{image_path}'"
            )

    # 2. 决定目标目录 target_dir
    if hook_output_dir:
        if os.path.isabs(hook_output_dir):
            # 如果是绝对目录，必须在 comfyui_output_dir 内，否则报错
            target_dir = os.path.abspath(hook_output_dir)
            if not is_under_directory(target_dir, comfyui_output_dir):
                raise ValueError(
                    f"HOOK_OUTPUT_DIR '{hook_output_dir}' is an absolute path but not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
                )
        else:
            # 默认相对于输出目录
            target_dir = os.path.abspath(
                os.path.join(comfyui_output_dir, hook_output_dir)
            )
    else:
        target_dir = os.path.dirname(image_path_abs)

    # 3. 计算相对路径
    rel_dir = os.path.relpath(target_dir, comfyui_output_dir)
    rel_dir = rel_dir.replace("\\", "/")
    return rel_dir


def extract_region_names_from_images(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 workflow 元数据中扫描区域标记，逐个 yield 区域名称。
    区域标记使用标准 // #region {name} 语法，存储在 CLIPTextEncode 节点的文本 widget 中。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        with Image.open(path) as img:
            workflow_str = img.info.get("workflow")
            if not workflow_str:
                continue
            workflow = json.loads(workflow_str)
            for node in workflow.get("nodes", []):
                widgets_values = node.get("widgets_values")
                if not isinstance(widgets_values, list):
                    continue
                widget_values_list: list[Any] = cast("list[Any]", widgets_values)
                for raw_val in widget_values_list:
                    if not isinstance(raw_val, str):
                        continue
                    for m in REGION_START_RE.finditer(raw_val):
                        name = m.group(1)
                        if name and name not in seen:
                            seen.add(name)
                            yield name


def extract_lora_names(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 prompt 元数据中提取所有 lora 文件名（不含扩展名），逐个 yield。
    委托 WorkflowPromptPair.collect_lora_names 处理。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        with Image.open(path) as img:
            prompt_str = img.info.get("prompt")
            if not prompt_str:
                continue
            prompt_data: Dict[str, Any] = json.loads(prompt_str)
            for n in WeightManager.collect_lora_names(prompt_data):
                name_no_ext, _ = os.path.splitext(n)
                if name_no_ext not in seen:
                    seen.add(name_no_ext)
                    yield name_no_ext


def main() -> None:
    config = ComfyUIConfig.from_env()

    log_level = getattr(logging, config.logging_level, logging.WARNING)
    logging.basicConfig(
        level=log_level,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    args = parse_args()
    _LOGGER.debug("args %s", (args,))

    max_match = args.max_match if args.max_match is not None else config.max_match
    if max_match < 0:
        raise ValueError(f"--max-match must be non-negative, got: {max_match}")

    jobs = args.jobs if args.jobs is not None else config.jobs
    if jobs <= 0:
        raise ValueError(f"--jobs/HOOK_JOBS must be a positive integer, got: {jobs}")

    # 汇总最终要处理 of (image_id, path) 列表
    targets: List[Tuple[str, str]] = []
    if args.command in ["add", "remove", "adjust"]:
        _LOGGER.debug(f"Command is {args.command}, fetching images via GraphQL...")
        targets = fetch_images(config.required_rating)
    else:
        # queue 场景
        if config.image_paths:
            for idx, path in enumerate(config.image_paths):
                img_id = config.image_ids[idx] if idx < len(config.image_ids) else ""
                targets.append((img_id, path))

    if max_match > 0 and len(targets) > max_match:
        _LOGGER.warning(
            "Skipping: matched %d images exceeds --max-match limit of %d",
            len(targets),
            max_match,
        )
        _write_action_override("KEEP", config.action_path)
        sys.exit(0)

    if not targets:
        _write_action_override("KEEP", config.action_path)
        if not config.trigger:
            raise ValueError("Environment variable IMAGE_FUNNEL_TRIGGER is missing")
        is_non_manual = config.trigger not in ["image_dispatch", "note_dispatch"]
        if is_non_manual:
            _LOGGER.info("No images found to process. Skipping.")
            sys.exit(0)
        else:
            _LOGGER.error("No images found to process.")
            sys.exit(1)

    _LOGGER.debug(
        "Found %d image(s) to process, command: %s", len(targets), args.command
    )

    has_errors = False
    success_count = 0

    for idx, (img_id, path) in enumerate(targets):
        if not os.path.exists(path):
            _LOGGER.error(f"File does not exist: {path}")
            has_errors = True
            continue
        _LOGGER.debug("[%d/%d] Processing image: %s", idx + 1, len(targets), path)

        prompt: Dict[str, Any]
        workflow: Dict[str, Any]

        try:
            with Image.open(path) as img:
                info: Any = img.info
                prompt_str: Optional[str] = info.get("prompt")
                workflow_str: Optional[str] = info.get("workflow")

                if not prompt_str:
                    _LOGGER.error(
                        f"This PNG image does not contain prompt metadata from ComfyUI: {path}"
                    )
                    has_errors = True
                    continue

                # 必须同时包含 prompt 且 workflow 有效，否则由于无法更新种子会导致重复生成
                if not workflow_str:
                    _LOGGER.error(
                        f"This PNG image does not contain workflow metadata from ComfyUI: {path}"
                    )
                    has_errors = True
                    continue

                prompt = json.loads(prompt_str)
                workflow_data: Any = json.loads(workflow_str)

                if not isinstance(workflow_data, dict) or "nodes" not in workflow_data:
                    _LOGGER.error(
                        f"This PNG image contains an invalid ComfyUI workflow (missing 'nodes'): {path}"
                    )
                    has_errors = True
                    continue
                workflow = cast(Dict[str, Any], workflow_data)
        except (OSError, ValueError) as e:
            _LOGGER.error(f"Failed to read PNG properties: {e}")
            has_errors = True
            continue

        try:
            if config.hook_output_dir == ":inherit:":
                _LOGGER.debug(
                    "HOOK_OUTPUT_DIR is ':inherit:', skipping output directory adjustment."
                )
            else:
                rel_dir = get_relative_output_dir(
                    path, config.comfyui_output_dir, config.hook_output_dir
                )
                pair = WorkflowPromptPair(workflow, prompt)
                FilenameManager(
                    pair, pair.date_filename_nodes, pair.title_to_node
                ).adjust_output_directory(rel_dir)
        except (ValueError, OSError) as e:
            _LOGGER.error(f"Failed to adjust output directory: {e}")
            has_errors = True
            continue

        handler = COMMAND_HANDLERS.get(args.command)
        if handler is None:
            raise ValueError(f"Unknown command: '{args.command}'")
        ctx = CommandContext(
            img_id=img_id,
            path=path,
            prompt=prompt,
            workflow=workflow,
            args=args,
            comfyui_url=config.comfyui_url,
            jobs=jobs,
            label_to_set=config.label_to_set,
        )
        handler.run(ctx)
        if not ctx.skipped:
            success_count += 1

    print(f"processed {success_count}/{len(targets)} image(s) successfully.")

    if success_count == 0:
        _write_action_override("KEEP", config.action_path)

    if has_errors:
        sys.exit(1)
    else:
        sys.exit(0)


def get_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="ComfyUI Funnel Hook Script")
    parser.add_argument(
        "--max-match",
        type=int,
        default=None,
        metavar="N",
        help="最大匹配图片数量，默认使用 HOOK_MAX_MATCH 环境变量值或 4，0 代表不限制",
    )
    subparsers = parser.add_subparsers(
        dest="command", required=True, help="Sub-commands"
    )

    queue_parser = subparsers.add_parser(
        "queue", help="Queue the image back to ComfyUI"
    )
    queue_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )

    add_parser = subparsers.add_parser(
        "add", help="Add prompt to image metadata and queue"
    )
    add_parser.add_argument(
        "--no-skip",
        "-S",
        action="store_true",
        help="Do not skip if prompt already exists",
    )
    add_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    add_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    add_parser.add_argument(
        "--raw", action="store_true", help="Add raw text without trailing comma"
    )
    add_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; priority after node",
    )
    add_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; highest priority",
    )
    add_parser.add_argument("prompt", nargs="+", help="The prompt text to add")

    remove_parser = subparsers.add_parser(
        "remove", help="Remove prompt from image metadata and queue"
    )
    remove_parser.add_argument(
        "--no-skip", "-S", action="store_true", help="Do not skip if prompt not found"
    )
    remove_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    remove_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    remove_parser.add_argument(
        "--raw",
        action="store_true",
        help="Remove raw text substring without line matching",
    )
    remove_parser.add_argument(
        "--hard",
        action="store_true",
        help="Directly delete the text instead of commenting it out",
    )
    remove_parser.add_argument(
        "--all",
        action="store_true",
        help="Remove from all CLIP input nodes, ignoring region/node constraints",
    )
    remove_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; searches all matching areas",
    )
    remove_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; searches all matching nodes",
    )
    remove_parser.add_argument("prompt", nargs="+", help="The prompt text to remove")

    # adjust command
    adjust_parser = subparsers.add_parser(
        "adjust", help="Adjust prompt weights or existing Lora weights"
    )
    adjust_subparsers = adjust_parser.add_subparsers(
        dest="adjust_type", required=True, help="Adjustment types"
    )

    # 1. adjust lora
    lora_parser = adjust_subparsers.add_parser(
        "lora", help="Adjust existing Lora weights"
    )
    lora_parser.add_argument("name", help="Target Lora name (substring match)")
    lora_parser.add_argument(
        "weight",
        help="Weight value or range (e.g. 0.8, 0.5:1.0:0.1, x-0.1:x+0.2:0.1, +-0.3:0.1)",
    )
    lora_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    lora_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    lora_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )

    # 2. adjust prompt
    prompt_parser = adjust_subparsers.add_parser("prompt", help="Adjust prompt weights")
    prompt_parser.add_argument("text", help="Target prompt text to adjust")
    prompt_parser.add_argument(
        "weight",
        help="Weight value or range (e.g. 1.2, 0.8:1.2:0.1, x-0.1:x+0.2:0.1, +-0.3:0.1)",
    )
    prompt_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    prompt_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    prompt_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    prompt_parser.add_argument(
        "--skip-add",
        action="store_true",
        help="Skip adding the prompt if it does not exist",
    )
    prompt_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    prompt_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; priority after node",
    )
    prompt_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; highest priority",
    )

    # 3. adjust cfg
    cfg_parser = adjust_subparsers.add_parser("cfg", help="Adjust KSampler CFG scale")
    cfg_parser.add_argument(
        "weight",
        help="CFG value or range (e.g. 7.0, 5.0:9.0:0.5, x-0.5:x+0.5:0.5, +-1.0:0.5)",
    )
    cfg_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    cfg_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    cfg_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    cfg_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target KSampler node ID, can be specified multiple times; if omitted, adjusts all KSampler nodes",
    )

    # 4. adjust aspect
    aspect_parser = adjust_subparsers.add_parser(
        "aspect", help="Adjust image aspect ratio (total pixels unchanged)"
    )
    aspect_parser.add_argument(
        "ratio",
        help="Target ratio or range (e.g. 16:9, +1, -1, +-1, +-2:2, swap)",
    )
    aspect_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    aspect_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    aspect_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    aspect_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target latent node ID containing width and height inputs, can be specified multiple times; if omitted, adjusts all latent nodes",
    )

    return parser


def parse_args(args: Optional[List[str]] = None):
    return get_parser().parse_args(args)


if __name__ == "__main__":
    main()
