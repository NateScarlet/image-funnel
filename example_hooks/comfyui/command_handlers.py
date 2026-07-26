#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
命令处理器模块：将 comfyui.py 中的命令调度逻辑拆分为独立 Handler。

每个 Handler 接收 CommandContext 并在其中执行对应操作。
"""

import logging
import uuid
from typing import Dict, List, Set, Any, Optional, Protocol, cast

from graphql_utils import GraphQLClient, progress_notification
from .workflow_prompt_pair import WorkflowPromptPair
from .prompt_fragment import PromptFragment
from .seed_manager import SeedManager
from .filename_manager import FilenameManager
from .weight_manager import WeightManager
from .submission import submit as _submit_fn
from . import variant_engine
from . import operation_history

_LOGGER = logging.getLogger(__name__)


class CommandContext:
    """命令处理器上下文，封装图片环境及参数"""

    def __init__(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
        history: operation_history.OperationHistory,
        client: GraphQLClient,
        hook_name: str,
    ):
        self.img_id = img_id
        self.path = path
        self.prompt = prompt
        self.workflow = workflow
        self.args = args
        self.comfyui_url = comfyui_url
        self.jobs = jobs
        self.label_to_set = label_to_set
        self.client = client
        self.history = history
        self.hook_name = hook_name
        self._progress_tag = str(uuid.uuid4())
        self.skipped: bool = False
        self.skip_reason: str = ""

    @property
    def progress_tag(self) -> str:
        """进度通知 tag，调用时生成固定 UUID"""
        return self._progress_tag

    def update_label(self) -> None:
        if self.label_to_set and self.img_id:
            self.client.update_image_label(self.img_id, self.label_to_set)

    def skip(self, reason: str = "") -> None:
        """标记当前图片处理已被跳过，reason 说明原因"""
        self.skipped = True
        self.skip_reason = reason


class CommandHandler(Protocol):
    """命令处理器协议"""

    def run(self, ctx: CommandContext) -> None:
        """执行指令并提交 ComfyUI 任务。
        如果需要跳过处理，可以调用 ctx.skip() 并直接 return 返回。
        若执行失败则直接抛出异常。
        """
        ...


class QueueHandler:
    """queue 命令：提交工作流到 ComfyUI"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(ctx.workflow, ctx.prompt)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        with progress_notification(
            ctx.client, ctx.progress_tag, "hooks", "提交 ComfyUI 任务", total=ctx.jobs
        ) as update:
            for q_idx in range(ctx.jobs):
                if ctx.jobs > 1:
                    _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, ctx.jobs)
                update(q_idx + 1, ctx.jobs, f"提交第 {q_idx + 1} 次任务")
                if seed_mgr.update_seeds() == 0:
                    raise ValueError(
                        f"Failed to update any seeds for image: {ctx.path}."
                    )
                filename_mgr.update_output_filenames()
                _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

        ctx.update_label()


class AddHandler:
    """add 命令：添加提示词并提交"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        fragments = pair.locate_prompts(
            nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
        )

        prompt_str_arg = " ".join(ctx.args.prompt)
        fragment = next(fragments, None)
        if fragment:
            fragment.add(prompt_str_arg, raw=ctx.args.raw, no_skip=ctx.args.no_skip)

            # 默认从非目标区域移除重复提示词，--keep 保留现有行为
            if not getattr(ctx.args, "keep", False):
                _remove_from_other_regions(
                    pair, prompt_str_arg, fragment.node_id, fragment.region, ctx.args
                )

        _submit_simple(ctx.prompt, ctx.workflow, ctx.comfyui_url, ctx.jobs, ctx.path)
        ctx.update_label()


class RemoveHandler:
    """remove 命令：移除提示词并提交"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        fragments = pair.locate_prompts(
            nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
        )
        hard = getattr(ctx.args, "hard", False)

        if ctx.args.all:
            fragments = [
                PromptFragment(pair, nid) for nid in _clip_text_encode_nodes(ctx.prompt)
            ]

        prompt_list: List[str] = list(ctx.args.prompt)
        remove_prompts: Set[str] = set()
        for p in prompt_list:
            remove_prompts.update((p, p.replace("_", " "), p.replace(" ", "_")))

        any_processed = False
        for fragment in fragments:
            for prompt_str_arg in remove_prompts:
                if fragment.remove(
                    prompt_str_arg,
                    raw=ctx.args.raw,
                    hard=hard,
                    no_skip=ctx.args.no_skip,
                ):
                    any_processed = True

        if not any_processed:
            ctx.skip("未找到待移除的提示词")
            return

        _submit_simple(ctx.prompt, ctx.workflow, ctx.comfyui_url, ctx.jobs, ctx.path)
        ctx.update_label()


# #region CLIPTextEncode 节点辅助


def _clip_text_encode_nodes(prompt: Dict[str, Any]) -> List[str]:
    """返回 prompt 中所有 CLIPTextEncode 类型节点的 ID 列表"""
    return [
        nid
        for nid, node in prompt.items()
        if isinstance(node, dict)
        and cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
    ]


def _remove_from_other_regions(
    pair: WorkflowPromptPair,
    prompt_str: str,
    target_node_id: str,
    target_region: str,
    args: Any,
) -> None:
    """从目标节点中非目标区域移除重复的提示词（与 /remove 相同的匹配逻辑）"""
    from .prompt_locator import REGION_START_RE

    # 生成变体（与 /remove 逻辑一致：原词、下划线、空格三种形态）
    remove_prompts: Set[str] = set()
    remove_prompts.update(
        (prompt_str, prompt_str.replace("_", " "), prompt_str.replace(" ", "_"))
    )

    wf_text = pair.get_workflow_node_text(target_node_id)
    if not wf_text:
        return

    # 扫描目标节点中的所有区域
    all_regions: Set[str] = set()
    for m in REGION_START_RE.finditer(wf_text):
        all_regions.add(m.group(1))

    # 从非目标区域移除
    for region in all_regions:
        if region == target_region:
            continue
        fragment = PromptFragment(pair, target_node_id, region=region)
        for variant in remove_prompts:
            # no_skip=True 静默跳过不存在的情况
            fragment.remove(variant, raw=args.raw, hard=False, no_skip=True)


# #endregion


class RemoveAgainHandler:
    """remove-again 命令：重放历史移除操作，合并为一个变体"""

    def run(self, ctx: CommandContext) -> None:
        records = ctx.history.list_remove()

        if not records:
            ctx.skip("无历史移除记录")
            return

        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        any_processed = False

        for record in records:
            # 根据历史记录中的范围参数定位目标片段
            fragments = pair.locate_prompts(
                nodes=record.node, regions=record.region, is_neg=record.neg
            )

            if record.all:
                fragments = [
                    PromptFragment(pair, nid)
                    for nid in _clip_text_encode_nodes(ctx.prompt)
                ]

            # 生成提示词的变体形式（空格/下划线互换）
            remove_prompts: Set[str] = set()
            remove_prompts.update(
                (
                    record.prompt,
                    record.prompt.replace("_", " "),
                    record.prompt.replace(" ", "_"),
                )
            )

            for fragment in fragments:
                for prompt_str_arg in remove_prompts:
                    if fragment.remove(
                        prompt_str_arg,
                        raw=record.raw,
                        hard=record.hard,
                        no_skip=record.no_skip,
                    ):
                        any_processed = True

        if not any_processed:
            ctx.skip("历史移除内容已不存在")
            return

        _submit_simple(ctx.prompt, ctx.workflow, ctx.comfyui_url, ctx.jobs, ctx.path)
        ctx.update_label()


class AdjustHandler:
    """adjust 命令：调整权重/比例并提交"""

    def run(self, ctx: CommandContext) -> None:
        node_ids: Optional[List[str]] = getattr(ctx.args, "node", None)
        enable_seed_update = ctx.args.update_seed or (ctx.jobs > 1)

        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        weight_mgr = WeightManager(pair)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        if ctx.args.adjust_type == "cfg":
            variant_gen = variant_engine.generate_cfg_variants(
                weight_mgr, ctx.prompt, ctx.args.weight, node_ids
            )
        elif ctx.args.adjust_type in ("lora", "l"):
            variant_gen = variant_engine.generate_lora_variants(
                weight_mgr, ctx.args.name, ctx.args.weight
            )
        elif ctx.args.adjust_type == "aspect":
            variant_gen = variant_engine.generate_aspect_variants(
                weight_mgr, ctx.prompt, pair.nodes_cache, ctx.args.ratio, node_ids
            )
        elif ctx.args.adjust_type in ("prompt", "p"):
            fragments = pair.locate_prompts(
                nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
            )
            variant_gen = variant_engine.generate_prompt_variants(
                weight_mgr, fragments, ctx.args.text, ctx.args.weight, ctx.args.skip_add
            )
        else:
            raise ValueError(f"unexpected adjust type '{ctx.args.adjust_type}'")

        # 收集所有变体以计算总进度
        variants = list(variant_gen)
        variant_count = len(variants)
        # 当 variant_count == 0 且 no_skip 时，仍会提交 ctx.jobs 次
        total_steps = variant_count * ctx.jobs if variant_count > 0 else ctx.jobs

        with progress_notification(
            ctx.client, ctx.progress_tag, "hooks", "调整权重并提交", total=total_steps
        ) as update:
            step = 0
            for v_idx, _ in enumerate(variants):
                if v_idx > 0:
                    enable_seed_update = True
                for j_idx in range(ctx.jobs):
                    step += 1
                    update(
                        step,
                        total_steps,
                        f"变体 {v_idx + 1}/{variant_count}，第 {j_idx + 1} 次提交",
                    )
                    if enable_seed_update:
                        if seed_mgr.update_seeds() == 0:
                            raise ValueError(
                                f"Failed to update any seeds for image: {ctx.path}."
                            )
                    filename_mgr.update_output_filenames()
                    _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

            if variant_count == 0 and ctx.args.no_skip:
                for j_idx in range(ctx.jobs):
                    step += 1
                    update(
                        step,
                        total_steps,
                        f"无变体，第 {j_idx + 1} 次提交",
                    )
                    if enable_seed_update:
                        if seed_mgr.update_seeds() == 0:
                            raise ValueError(
                                f"Failed to update any seeds for image: {ctx.path}."
                            )
                    filename_mgr.update_output_filenames()
                    _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

        processed = variant_count > 0 or ctx.args.no_skip
        if not processed:
            ctx.skip("无有效变体（提示词不存在或权重未变化）")
            return

        ctx.update_label()


def _submit_simple(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
    jobs: int,
    path: str,
) -> None:
    """基础提交逻辑：更新种子、文件名并提交"""
    pair = WorkflowPromptPair(workflow, prompt)
    seed_mgr = SeedManager(pair, pair.seed_nodes)
    filename_mgr = FilenameManager(pair, pair.date_filename_nodes, pair.title_to_node)

    for q_idx in range(jobs):
        if jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
        if seed_mgr.update_seeds() == 0:
            raise ValueError(f"Failed to update any seeds for image: {path}.")
        filename_mgr.update_output_filenames()
        _submit_fn(prompt, workflow, comfyui_url)


# #region 命令注册表

COMMAND_HANDLERS: Dict[str, CommandHandler] = {
    "queue": QueueHandler(),
    "add": AddHandler(),
    "remove": RemoveHandler(),
    "remove-again": RemoveAgainHandler(),
    "adjust": AdjustHandler(),
}

# #endregion
