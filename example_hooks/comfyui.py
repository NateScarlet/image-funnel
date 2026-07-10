#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# dependencies = [
#   "Pillow",
#   "requests",
# ]
# ///

import os
import sys

# 重新配置标准输出和标准错误流的编码和错误处理，在 Windows 环境下防止 'gbk' 无法编码特定 Unicode 字符（例如 \ufffd）抛出 UnicodeEncodeError
if sys.platform.startswith("win"):
    reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
    if reconfigure_stdout is not None:
        try:
            reconfigure_stdout(encoding="utf-8", errors="replace")
        except Exception:
            pass
    reconfigure_stderr = getattr(sys.stderr, "reconfigure", None)
    if reconfigure_stderr is not None:
        try:
            reconfigure_stderr(encoding="utf-8", errors="replace")
        except Exception:
            pass

import json
from typing import Dict, List, Tuple, Any, Optional, Set, Iterator, cast
import logging
import argparse

from graphql_utils import update_image_label, fetch_images
from workflow_prompt_pair import WorkflowPromptPair
from prompt_fragment import PromptFragment
from prompt_locator import START_REGION_PREFIX as _START_REGION_PREFIX

_LOGGER = logging.getLogger(__name__)


def _write_action_override(action: str) -> None:
    """向 IMAGE_FUNNEL_ACTION 文件写入操作覆盖，通知 Runner 跳过默认行为"""
    action_path = os.getenv("IMAGE_FUNNEL_ACTION", "")
    if not action_path:
        raise ValueError("IMAGE_FUNNEL_ACTION environment variable is not set")
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


def submit_workflow(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
    jobs: int,
    image_path: str,
) -> Tuple[bool, bool]:
    """
    更新种子和文件名后提交工作流到 ComfyUI。
    返回 (any_success, has_error)。
    """
    pair = WorkflowPromptPair(workflow, prompt)
    any_success = False
    has_error = False
    for q_idx in range(jobs):
        if jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
        if pair.update_seeds() == 0:
            _LOGGER.error(
                f"Failed to update any seeds for image: {image_path}. Cannot queue duplicate workflow without changing seeds."
            )
            has_error = True
            break
        pair.update_output_filenames()
        if pair.submit(comfyui_url):
            any_success = True
        else:
            has_error = True
    return any_success, has_error


def extract_region_names_from_images(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 workflow 元数据中扫描区域标记，逐个 yield 区域名称。
    区域标记由 `_START_REGION_PREFIX` 定义，存储在 CLIPTextEncode 节点的文本 widget 中。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        try:
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
                        val_text: str = raw_val
                        if _START_REGION_PREFIX not in val_text:
                            continue
                        for line in val_text.splitlines():
                            stripped = line.strip()
                            if stripped.startswith(_START_REGION_PREFIX):
                                name = stripped[len(_START_REGION_PREFIX) :].strip()
                                if name and name not in seen:
                                    seen.add(name)
                                    yield name
        except Exception:
            continue


def extract_lora_names(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 prompt 元数据中提取所有 lora 文件名（不含扩展名），逐个 yield。
    委托 WorkflowPromptPair.collect_lora_names 处理。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        try:
            with Image.open(path) as img:
                prompt_str = img.info.get("prompt")
                if not prompt_str:
                    continue
                prompt_data: Dict[str, Any] = json.loads(prompt_str)
                for n in WorkflowPromptPair.collect_lora_names(prompt_data):
                    name_no_ext, _ = os.path.splitext(n)
                    if name_no_ext not in seen:
                        seen.add(name_no_ext)
                        yield name_no_ext
        except Exception:
            continue


def main() -> None:
    # 从 HOOK_LOGGING_LEVEL 环境变量读取日志级别，默认 WARNING
    log_level_str = os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper()
    log_level = getattr(logging, log_level_str, logging.WARNING)
    logging.basicConfig(
        level=log_level,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    args = parse_args()
    _LOGGER.debug("args %s", (args,))

    # 解析 --max-match：默认从 HOOK_MAX_MATCH 环境变量读取，未设置则为 4
    max_match = args.max_match
    if max_match is None:
        max_match_str = os.getenv("HOOK_MAX_MATCH", "4")
        max_match = int(max_match_str)  # 非法值让 ValueError 向上传播
    if max_match < 0:
        raise ValueError(f"--max-match must be non-negative, got: {max_match}")

    image_paths_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "")
    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    comfyui_url: str = os.getenv("COMFYUI_URL", "http://127.0.0.1:8188")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    # 过滤器星级检查（针对 add/remove 指令的选做项）
    required_rating_str = os.getenv("HOOK_IMAGE_RATING")
    required_rating: Optional[int] = None
    if required_rating_str:
        required_rating = int(required_rating_str)

    jobs = args.jobs if args.jobs is not None else int(os.getenv("HOOK_JOBS", "1"))

    try:
        image_paths: List[str] = json.loads(image_paths_str) if image_paths_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_PATHS: {e}")
        sys.exit(1)

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS: {e}")
        sys.exit(1)

    # 汇总最终要处理 of (image_id, path) 列表
    targets: List[Tuple[str, str]] = []
    if args.command in ["add", "remove", "adjust"]:
        _LOGGER.debug(f"Command is {args.command}, fetching images via GraphQL...")
        targets = fetch_images(required_rating)
    else:
        # queue 场景
        if image_paths:
            for idx, path in enumerate(image_paths):
                img_id = image_ids[idx] if idx < len(image_ids) else ""
                targets.append((img_id, path))

    if max_match > 0 and len(targets) > max_match:
        _LOGGER.warning(
            "Skipping: matched %d images exceeds --max-match limit of %d",
            len(targets),
            max_match,
        )
        _write_action_override("KEEP")
        sys.exit(0)

    if not targets:
        _write_action_override("KEEP")
        trigger = os.getenv("IMAGE_FUNNEL_TRIGGER", "")
        if not trigger:
            raise ValueError("Environment variable IMAGE_FUNNEL_TRIGGER is missing")
        is_non_manual = trigger not in ["image_dispatch", "note_dispatch"]
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
        except Exception as e:
            _LOGGER.error(f"Failed to read PNG properties: {e}")
            has_errors = True
            continue

        try:
            hook_output_dir = os.getenv("HOOK_OUTPUT_DIR", "")
            if hook_output_dir == ":inherit:":
                _LOGGER.debug(
                    "HOOK_OUTPUT_DIR is ':inherit:', skipping output directory adjustment."
                )
            else:
                comfyui_output_dir_env = os.getenv("COMFYUI_OUTPUT_DIR", "")
                rel_dir = get_relative_output_dir(
                    path, comfyui_output_dir_env, hook_output_dir
                )
                pair = WorkflowPromptPair(workflow, prompt)
                pair.adjust_output_directory(rel_dir)
        except Exception as e:
            _LOGGER.error(f"Failed to adjust output directory: {e}")
            has_errors = True
            continue

        if args.command == "queue":
            any_success, submit_error = submit_workflow(
                prompt, workflow, comfyui_url, jobs, path
            )
            if submit_error:
                has_errors = True
            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

        elif args.command == "adjust":
            node_ids: Optional[List[str]] = getattr(args, "node", None)

            if args.adjust_type == "cfg":
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_cfg_variants(args.weight, node_ids)
            elif args.adjust_type in ("lora", "l"):
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_lora_variants(args.name, args.weight)
            elif args.adjust_type == "aspect":
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_aspect_variants(args.ratio, node_ids)
            elif args.adjust_type in ("prompt", "p"):
                is_neg = args.neg
                pair = WorkflowPromptPair(workflow, prompt)
                fragments = pair.locate_prompts(
                    nodes=args.node,
                    regions=args.region,
                    is_neg=is_neg,
                )
                variant_gen = pair.generate_prompt_variants(
                    fragments, args.text, args.weight, args.skip_add
                )
            else:
                raise ValueError(f"unexpected adjust type '{args.adjust_type}'")

            enable_seed_update = args.update_seed or (jobs > 1)
            any_image_success = False
            variant_count = 0

            for _ in variant_gen:
                variant_count += 1
                if variant_count > 1:
                    enable_seed_update = True

                for q_idx in range(jobs):
                    if jobs > 1 or variant_count > 1:
                        _LOGGER.debug(
                            "  -> Queueing variant %d run %d/%d",
                            variant_count,
                            q_idx + 1,
                            jobs,
                        )
                    if enable_seed_update:
                        if pair.update_seeds() == 0:
                            _LOGGER.error(
                                f"Failed to update any seeds for image: {path}. Cannot queue duplicate workflow without changing seeds."
                            )
                            has_errors = True
                            break
                    pair.update_output_filenames()
                    if pair.submit(comfyui_url):
                        any_image_success = True
                    else:
                        has_errors = True

            if variant_count == 0 and args.no_skip:
                _LOGGER.debug(
                    "No variants generated for image %s, sending original workflow (--no-skip).",
                    path,
                )
                pair = WorkflowPromptPair(workflow, prompt)
                for q_idx in range(jobs):
                    if jobs > 1:
                        _LOGGER.debug(
                            "  -> Queueing original run %d/%d", q_idx + 1, jobs
                        )
                    if enable_seed_update:
                        if pair.update_seeds() == 0:
                            _LOGGER.error(
                                f"Failed to update any seeds for image: {path}. Cannot queue duplicate workflow without changing seeds."
                            )
                            has_errors = True
                            break
                    pair.update_output_filenames()
                    if pair.submit(comfyui_url):
                        any_image_success = True
                    else:
                        has_errors = True
            elif variant_count == 0:
                _LOGGER.debug(
                    "No variants generated for image %s, skipping submission.", path
                )
                _write_action_override("KEEP")

            if any_image_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

        elif args.command in ["add", "remove"]:
            is_neg = args.neg
            pair = WorkflowPromptPair(workflow, prompt)
            fragments = pair.locate_prompts(
                nodes=args.node, regions=args.region, is_neg=is_neg
            )
            hard = getattr(args, "hard", False)
            any_processed = False

            if args.command == "add":
                prompt_str_arg = " ".join(args.prompt)
                fragment = next(fragments, None)
                if fragment:
                    if fragment.add(prompt_str_arg, raw=args.raw, no_skip=args.no_skip):
                        any_processed = True
                    else:
                        any_processed = True  # 跳过也算成功

                if not any_processed:
                    has_errors = True
                    _LOGGER.error("No target was successfully processed for add.")
                    continue

            else:  # remove
                # 展开所有目标为具体节点列表
                if args.all:
                    clip_nodes = [
                        nid
                        for nid, node in prompt.items()
                        if cast(Dict[str, Any], node).get("class_type")
                        == "CLIPTextEncode"
                    ]
                    fragments = [PromptFragment(pair, nid) for nid in clip_nodes]

                # 为每个提示词生成原始、下划线、空格三种变体，去重
                remove_prompts: Set[str] = set()
                for p in args.prompt:
                    remove_prompts.update((p, p.replace("_", " "), p.replace(" ", "_")))
                _LOGGER.debug("Remove prompts (variants): %s", remove_prompts)

                for fragment in fragments:
                    for prompt_str_arg in remove_prompts:
                        _LOGGER.debug(
                            "Trying to remove '%s' from fragment %s",
                            prompt_str_arg,
                            fragment.node_id,
                        )
                        if fragment.remove(
                            prompt_str_arg,
                            raw=args.raw,
                            hard=hard,
                            no_skip=args.no_skip,
                        ):
                            _LOGGER.debug(
                                "Removed '%s' from fragment %s",
                                prompt_str_arg,
                                fragment.node_id,
                            )
                            any_processed = True
                        else:
                            _LOGGER.debug(
                                "Skipped '%s' in fragment %s (not found or skipped)",
                                prompt_str_arg,
                                fragment.node_id,
                            )

                if not any_processed:
                    _LOGGER.debug("No prompts were removed, skipping submission.")
                    continue

            # 提交到 ComfyUI
            any_success, submit_error = submit_workflow(
                prompt, workflow, comfyui_url, jobs, path
            )
            if submit_error:
                has_errors = True
            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

    print(f"processed {success_count}/{len(targets)} image(s) successfully.")

    if success_count == 0:
        _write_action_override("KEEP")

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
