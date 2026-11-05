#!/usr/bin/env python
# -*- coding: utf-8 -*-

import copy
import datetime
import os
import sys
import uuid
from collections import Counter

from typing import Dict, List, Tuple, Any, Optional, Set, Iterator, Iterable, cast
import logging
import argparse

from graphql_utils import GraphQLClient, progress_notification
from .output_directory import get_source_rel_dir, resolve_output_rel_dir
from .png_metadata import load_prompt_and_workflow
from .workflow_prompt_pair import WorkflowPromptPair
from .filename_manager import FilenameManager
from .weight_manager import WeightManager
from .prompt_locator import REGION_START_RE
from .command_handlers import (
    COMMAND_HANDLERS,
    CommandContext,
    SubmissionLedger,
    handle_set_model_format_cmd,
)
from .model_format import format_workflow_prompt_pair
from . import remix

from .config import ComfyUIConfig
from .db import SQLiteContext
from . import operation_history

_LOGGER = logging.getLogger(__name__)

# 补全排序偏好：这两个区域是提示词最常用的区域，总是排在其他区域之前
_PREFERRED_REGION_NAMES = ("positive", "negative")


def _write_action_override(action: str, action_path: str) -> None:
    """向 action_path 文件写入操作覆盖，通知 Runner 跳过默认行为"""
    if not action_path:
        raise ValueError("action_path is empty")
    with open(action_path, "w", encoding="utf-8") as f:
        f.write(action)


def extract_region_names(
    workflows: Iterable[Dict[str, Any]],
) -> Iterator[str]:
    """从已解析的 workflow 数据中扫描区域标记，按固定顺序 yield 区域名称。

    positive、negative 是提示词最常用的两个区域，补全时固定排在最前；
    其余区域保持扫描顺序（稳定排序，不按字母重排）。只为实际存在的区域产出名称。
    """
    seen: Set[str] = set()
    scanned_names: List[str] = []
    for workflow in workflows:
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
                        scanned_names.append(name)

    # 置顶需要先看到全部区域，因此这里先收集再按偏好顺序产出
    for preferred_name in _PREFERRED_REGION_NAMES:
        if preferred_name in seen:
            yield preferred_name
    for name in scanned_names:
        if name not in _PREFERRED_REGION_NAMES:
            yield name


def extract_lora_names(prompts: Iterable[Dict[str, Any]]) -> Iterator[str]:
    """从已解析的 prompt 元数据中提取所有 lora 文件名（不含扩展名），逐个 yield。"""
    seen: Set[str] = set()
    for prompt_data in prompts:
        for n in WeightManager.collect_lora_names(prompt_data):
            name_no_ext, _ = os.path.splitext(n)
            if name_no_ext not in seen:
                seen.add(name_no_ext)
                yield name_no_ext


def _targets_from_config(config: ComfyUIConfig) -> List[Tuple[str, str]]:
    """把钩子上下文注入的图片路径列表转成 (image_id, path) 目标。

    image_id 缺失时留空，后续打标等依赖 ID 的操作自然跳过。
    """
    return [
        (config.image_ids[idx] if idx < len(config.image_ids) else "", path)
        for idx, path in enumerate(config.image_paths)
    ]


def run_comfyui(client: GraphQLClient, config: Optional[ComfyUIConfig] = None) -> None:
    if config is None:
        config = ComfyUIConfig.from_env()

    args = parse_args()
    _LOGGER.debug("args %s", (args,))

    if args.command == "set-model-format":
        handle_set_model_format_cmd(args)
        return

    # queue 是「无编辑项的重混」：既不带旗标，也不做任何编辑
    try:
        request = (
            remix.from_args(args) if args.command == "remix" else remix.RemixRequest()
        )
    except ValueError:
        # 旗标组合非法：一张都没提交过，保留指令行让用户改写后重试（报错仍照常抛出）
        _write_action_override("KEEP", config.action_path)
        raise

    max_match = args.max_match if args.max_match is not None else config.max_match
    if max_match < 0:
        raise ValueError(f"--max-match must be non-negative, got: {max_match}")

    jobs = args.jobs if args.jobs is not None else config.jobs
    if jobs <= 0:
        raise ValueError(f"--jobs/HOOK_JOBS must be a positive integer, got: {jobs}")

    # 汇总最终要处理 of (image_id, path) 列表
    targets: List[Tuple[str, str]] = []
    if args.command == "remix" and request.current:
        # --current 只处理钩子上下文注入的配套图片；缺失时直接报错中止，绝不退回
        # 「目录 + 评分」批量取图，否则「只处理这一张」会静默变成批量处理
        if not config.image_paths:
            _write_action_override("KEEP", config.action_path)
            raise ValueError(
                "--current requires at least one companion image path from the hook context."
            )
        targets = _targets_from_config(config)
    elif args.command == "remix":
        _LOGGER.debug(f"Command is {args.command}, fetching images via GraphQL...")

        directory_id = os.getenv("IMAGE_FUNNEL_DIRECTORY_ID")
        if not directory_id:
            raise ValueError(
                "Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing."
            )
        root_dir = os.getenv("IMAGE_FUNNEL_ROOT_DIR")
        if not root_dir:
            raise ValueError("Environment variable IMAGE_FUNNEL_ROOT_DIR is missing.")
        targets = client.fetch_images(directory_id, root_dir, config.required_rating)
    elif args.command == "queue" and config.trigger == "post_commit_session":
        # queue 场景（会话提交触发）：拉取目录内匹配评分且未打标的图片
        _LOGGER.debug("Command is queue, fetching images via GraphQL...")

        directory_id = os.getenv("IMAGE_FUNNEL_DIRECTORY_ID")
        if not directory_id:
            raise ValueError(
                "Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing."
            )
        root_dir = os.getenv("IMAGE_FUNNEL_ROOT_DIR")
        if not root_dir:
            raise ValueError("Environment variable IMAGE_FUNNEL_ROOT_DIR is missing.")
        # 仅入列未打标的图片：已打标（如入列成功后设置的标签）视为已入列过，避免重复提交
        targets = client.fetch_images(
            directory_id, root_dir, config.required_rating, filter_label=[""]
        )
    elif config.image_paths:
        # queue 场景（手动派发或笔记触发）：使用传入的图片
        targets = _targets_from_config(config)

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

    # 在执行前记录操作历史，失败的操作也需要被追溯和重放
    op_history = operation_history.OperationHistory(SQLiteContext.from_env())
    op_history.record_remix(request)

    # 构建进度通知 tag
    progress_tag = str(uuid.uuid4())

    # 一次 hook 执行共用一份提交台账：同一执行内完全相同的 prompt 只入列一次
    submission_ledger = SubmissionLedger()

    # 根据命令构造频道和标题
    if args.command == "queue":
        channel = "ComfyUI 队列提交"
        title = "提交 ComfyUI 任务"
    elif request.has_edits:
        channel = "ComfyUI 重混"
        title = "重混并提交"
    else:
        channel = "ComfyUI 队列提交"
        title = "提交 ComfyUI 任务"

    has_errors = False
    success_count = 0
    skip_reasons: list[str] = []
    # 同名节点提示同时进入最终摘要：进度通知会被后续图片覆盖并在退出时撤回
    duplicate_notices: list[str] = []
    # 一次 hook 执行内只取一次时间：日期文件名模板据此在各张图上求值出稳定的快照
    now = datetime.datetime.now()

    with progress_notification(client, progress_tag, channel, title) as update:
        for idx, (img_id, path) in enumerate(targets):
            if not os.path.exists(path):
                _LOGGER.error(f"File does not exist: {path}")
                has_errors = True
                continue
            _LOGGER.debug("[%d/%d] Processing image: %s", idx + 1, len(targets), path)

            # 更新进度通知
            update(f"处理图片 {os.path.basename(path)} ({idx + 1}/{len(targets)})")

            try:
                prompt, workflow = load_prompt_and_workflow(path)
            except (OSError, ValueError) as e:
                _LOGGER.error(f"Failed to read PNG properties: {e}")
                has_errors = True
                continue

            if not prompt:
                _LOGGER.error(
                    f"This PNG image does not contain prompt metadata from ComfyUI: {path}"
                )
                has_errors = True
                continue

            # 必须同时包含 prompt 且 workflow 有效，否则由于无法更新种子会导致重复生成
            if not workflow:
                _LOGGER.error(
                    f"This PNG image does not contain workflow metadata from ComfyUI: {path}"
                )
                has_errors = True
                continue

            if "nodes" not in workflow:
                _LOGGER.error(
                    f"This PNG image contains an invalid ComfyUI workflow (missing 'nodes'): {path}"
                )
                has_errors = True
                continue

            try:
                pair = WorkflowPromptPair(workflow, prompt)
                # 一次 hook 执行内只取一次时间并全程复用：日期文件名模板在预处理阶段
                # 求值一次，基线与提交侧因此天然一致，比较无需排除任何字段
                filename_mgr = FilenameManager(
                    pair, pair.date_filename_nodes, pair.title_to_node, now
                )
                rel_dir = resolve_output_rel_dir(
                    path,
                    config.comfyui_output_dir,
                    config.hook_output_dir,
                    request.branch,
                )
                if rel_dir is None:
                    _LOGGER.debug(
                        "HOOK_OUTPUT_DIR is ':inherit:', skipping output directory adjustment."
                    )
                else:
                    # 源图当前目录用于剥离上一代目标目录；只在实际调整时才推算
                    source_rel_dir = get_source_rel_dir(path, config.comfyui_output_dir)
                    duplicate_message = filename_mgr.adjust_output_directory(
                        rel_dir, source_rel_dir
                    )
                    # 存在同名节点时复用本次执行的进度通知告知用户（不新增通知通道），
                    # 同时记入最终摘要，避免通知被后续进度覆盖后用户看不到
                    if duplicate_message:
                        update(duplicate_message)
                        duplicate_notices.append(duplicate_message)

                # 所有输出路径统一按节点模型格式重排提示词（与 adjust_output_dir 同位置、全命令覆盖）
                format_workflow_prompt_pair(pair)

                # 文件名求值与目录调整、格式重排同层：每张图预处理时刷新一次即可，
                # 提交循环不再重新求值，同一张图的多次 job 得到相同的 filename_prefix
                filename_mgr.update_output_filenames()

                # 预处理完成、尚未应用任何编辑：此快照是判定本次提交是否产生有意义改动的基线
                baseline_prompt = copy.deepcopy(prompt)
            except (ValueError, OSError) as e:
                _LOGGER.error(
                    f"Failed to preprocess image workflow/prompt (output directory, "
                    f"prompt format, filename): {e}"
                )
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
                request=request,
                comfyui_url=config.comfyui_url,
                jobs=jobs,
                label_to_set=config.label_to_set,
                client=client,
                history=op_history,
                hook_name=config.hook_name,
                baseline_prompt=baseline_prompt,
                submission_ledger=submission_ledger,
                now=now,
            )
            handler.run(ctx)
            if not ctx.skipped:
                success_count += 1
            elif ctx.skip_reason:
                skip_reasons.append(ctx.skip_reason)

    skipped = len(targets) - success_count
    skip_msg = ""
    if skipped > 0 and skip_reasons:
        summary = dict(Counter(skip_reasons))
        parts = [f"跳过 {n} 张：{r}" for r, n in summary.items()]
        skip_msg = "（" + "；".join(parts) + "）"

    # 重混可能同时含多项编辑，逐项罗列会超出可读长度，只截断整体摘要
    def describe_request() -> str:
        summary = remix.describe(request)
        if not request.has_edits:
            # 无编辑项时仍要带上 `--branch`：它不改变 prompt，但决定了落盘位置
            summary = "，".join(part for part in ("重新入列", summary) if part)
        return summary if len(summary) <= 40 else summary[:40] + "…"

    action = describe_request()
    # 同名节点提示并入最终结果摘要，保证用户一定能看到
    notice_msg = "；" + "；".join(duplicate_notices) if duplicate_notices else ""
    if success_count > 0:
        print(f"将 {success_count} 张图片{action}{skip_msg}{notice_msg}")
    elif skip_reasons:
        summary = dict(Counter(skip_reasons))
        parts = [f"{n} 张：{r}" for r, n in summary.items()]
        print(f"没有图片需要{action}（" + "；".join(parts) + "）" + notice_msg)
    else:
        print(f"没有图片需要{action}{notice_msg}")

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

    remix_parser = subparsers.add_parser(
        "remix",
        help="按旗标集合编辑当前目录图片的 ComfyUI 工作流并入列（无编辑旗标时等价 queue）",
    )
    remix_parser.add_argument(
        "--add",
        action="append",
        nargs="+",
        default=None,
        metavar="tag",
        help="新增提示词，可多次指定；恰好一个 tag 时末尾 token 可为权重表达式（表示调整该 tag 权重）",
    )
    remix_parser.add_argument(
        "--remove",
        action="append",
        nargs="+",
        default=None,
        metavar="tag",
        help="移除提示词，可多次指定；从所有 CLIPTextEncode 节点中移除",
    )
    remix_parser.add_argument(
        "--current",
        action="store_true",
        help="只处理笔记配套的当前图片（钩子上下文注入的图片），不按 HOOK_IMAGE_RATING 筛选；"
        "缺少配套图片时报错并保留指令行",
    )
    remix_parser.add_argument(
        "--branch",
        nargs="?",
        const=remix.BRANCH_VALUE_MISSING,
        default=None,
        metavar="suffix",
        help="生成结果落到按 /branch 规则命名的新目录（根目录 <suffix>，子目录 <当前目录名>,<suffix>）；"
        "优先于 HOOK_OUTPUT_DIR，不计入编辑项",
    )
    remix_parser.add_argument(
        "--remove-again",
        action="store_true",
        help="在其他编辑之前重放当前目录的历史移除操作（忽略历史记录中的范围参数）",
    )
    remix_parser.add_argument(
        "--lora",
        action="append",
        nargs="+",
        default=None,
        metavar=("name", "weight"),
        help="调整 Lora 权重（子串匹配名称），可多次指定",
    )
    remix_parser.add_argument(
        "--cfg",
        action="append",
        default=None,
        metavar="weight",
        help="调整 KSampler 的 CFG，可多次指定（各产一个变体）",
    )
    remix_parser.add_argument(
        "--aspect",
        action="append",
        default=None,
        metavar="ratio",
        help="调整图像长宽比，可多次指定（各产一个变体）",
    )
    remix_parser.add_argument(
        "--no-skip",
        "-S",
        action="store_true",
        help="不跳过：新增忽略已存在、移除忽略未找到、无改动也照常提交",
    )
    remix_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="job 数，默认使用 HOOK_JOBS 环境变量值；每个 job 使用一份种子，"
        "同一 job 内的所有变体共用该种子",
    )
    remix_parser.add_argument(
        "--neg",
        action="store_true",
        help="当没有 region 或 node 匹配时，提示词调整匹配负向区域而不是正向",
    )
    remix_parser.add_argument(
        "--raw",
        action="store_true",
        help="新增时不附加尾随逗号；移除时精确移除子串而不是整行",
    )
    remix_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="新增与调权的目标区域，可多次指定；优先匹配 node",
    )
    remix_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="目标节点，可多次指定；优先匹配 node。CFG/长宽比同样作用于这些节点",
    )
    remix_parser.add_argument(
        "--keep",
        action="store_true",
        help="新增时保留非目标区域中已有的同名提示词，不移除",
    )
    remix_parser.add_argument(
        "--skip-add",
        action="store_true",
        help="调权目标不存在时跳过添加，默认不存在时会添加",
    )
    remix_parser.add_argument(
        "--hard",
        action="store_true",
        help="移除时直接删除文本而不是注释掉（指定 --raw 时自动隐含）",
    )
    remix_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="强制每个 job 都更换种子（默认 job 1 沿用源图种子，job 2..N 各自更换）",
    )
    # 5. set-model-format
    set_fmt_parser = subparsers.add_parser(
        "set-model-format", help="Set prompt tag format for a ComfyUI model"
    )
    set_fmt_parser.add_argument(
        "model", help="Model Checkpoint filename (e.g. animaPencilXL_v10.safetensors)"
    )
    set_fmt_parser.add_argument(
        "format",
        choices=["anima", "sdxl", "disabled"],
        help="Prompt tag format style (anima, sdxl or disabled)",
    )

    return parser


def parse_args(args: Optional[List[str]] = None):
    return get_parser().parse_args(args)


if __name__ == "__main__":
    client = GraphQLClient.from_env()
    run_comfyui(client)
