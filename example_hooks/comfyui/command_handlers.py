#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
命令处理器模块：将 comfyui.py 中的命令调度逻辑拆分为独立 Handler。

每个 Handler 接收 CommandContext 并在其中执行对应操作。
"""

import copy
import datetime
import logging
import uuid
from typing import (
    Any,
    Callable,
    Dict,
    Iterable,
    List,
    Optional,
    Protocol,
    Set,
    Tuple,
    cast,
)

from graphql_utils import GraphQLClient, ProgressUpdate, progress_notification
from .prompt_fragment import PromptFragment
from .seed_manager import SeedManager
from .weight_manager import WeightManager
from .workflow_prompt_pair import WorkflowPromptPair
from .submission import submit as _submit_fn
from .remix import (
    NodeStep,
    RemixRequest,
    VariantDimension,
    build_matrix,
    locate_fragments,
    ASPECT_KIND,
    CFG_KIND,
    LORA_KIND,
    PROMPT_KIND,
)
from . import operation_history

_LOGGER = logging.getLogger(__name__)

# 即将提交的 prompt 与基线完全一致时的跳过原因（所有提交路径共用同一文案）
SKIP_REASON_NO_CHANGE = "重混未产生任何改动"


class CommandContext:
    """命令处理器上下文，封装图片环境及参数"""

    def __init__(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        request: RemixRequest,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
        history: operation_history.OperationHistory,
        client: GraphQLClient,
        hook_name: str,
        baseline_prompt: Dict[str, Any],
        now: datetime.datetime,
    ):
        self.img_id = img_id
        self.path = path
        self.prompt = prompt
        self.workflow = workflow
        self.request = request
        self.comfyui_url = comfyui_url
        self.jobs = jobs
        self.label_to_set = label_to_set
        self.client = client
        self.history = history
        self.hook_name = hook_name
        # 预处理完成、尚未应用任何编辑时的 prompt 快照：入列前以它判定本次提交是否
        # 只是重跑一张与源图完全相同的图
        self.baseline_prompt = baseline_prompt
        # 本图统一的时间取值，供预处理与提交路径共用
        self.now = now
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


class RemixHandler:
    """remix 命令：按旗标集合执行 ComfyUI 编辑并提交。

    是否真的入列由 `_submit_jobs` 按「即将提交的 prompt 是否与基线一致」判定，
    因此无编辑项、无生效编辑、种子策略固定等空转情形都不会产生新图片。
    """

    def run(self, ctx: CommandContext) -> None:
        request = ctx.request
        if not request.has_edits:
            pair = WorkflowPromptPair(ctx.workflow, ctx.prompt)
            if not _submit_jobs(ctx, pair, None):
                ctx.skip(SKIP_REASON_NO_CHANGE)
                return
            ctx.update_label()
            return

        base_workflow = copy.deepcopy(ctx.workflow)
        base_prompt = copy.deepcopy(ctx.prompt)
        matrix = build_matrix(request, WorkflowPromptPair(base_workflow, base_prompt))

        submitted = 0
        with progress_notification(
            ctx.client, ctx.progress_tag, "ComfyUI 重混", "重混并提交"
        ) as update:
            for index, selection in enumerate(matrix.iter_variants()):
                # 每个变体都从同一份基线出发：相对表达式的「当前值」不随变体顺序漂移
                pair = WorkflowPromptPair(
                    copy.deepcopy(base_workflow), copy.deepcopy(base_prompt)
                )
                _apply_edits(pair, request, selection, ctx.history)
                if _submit_jobs(ctx, pair, index, update, submitted + 1, len(matrix)):
                    submitted += 1

        if submitted == 0:
            ctx.skip(SKIP_REASON_NO_CHANGE)
            return
        ctx.update_label()


def _submit_jobs(
    ctx: CommandContext,
    pair: WorkflowPromptPair,
    variant_index: Optional[int],
    update: Optional[ProgressUpdate] = None,
    variant_no: int = 0,
    variant_total: int = 0,
) -> bool:
    """提交单张图片的工作流至多 jobs 次，返回是否真的发生了提交。

    入列前的最终关卡，queue 路径、无编辑项路径与每个变体路径共用：种子更新是在提交
    边界才发生的隐式编辑，因此先更新种子再比较——种子策略随机化时比较结果自然不等，
    而固定种子且无其他改动时即将提交的 prompt 与基线完全相同，提交出去只会得到另一
    张与源图一模一样的图，此时跳过（`--no-skip` 例外）。

    种子更新策略：改动提示词内容的编辑（新增 / 移除 / 重放历史移除）以及重新入列
    必定更新，否则提交出去的就是与上一张完全相同的图；纯参数调整（提示词权重、
    Lora、CFG、长宽比）只在用户显式要求（--update-seed）、需要多次提交（多 job）
    或有多档变体时更新，好让同一噪波下的不同参数可以直接对照。
    """
    seed_mgr = SeedManager(pair, pair.seed_nodes)
    enable_seed_update = (
        not ctx.request.has_edits
        or ctx.request.edits_prompt_content
        or ctx.request.update_seed
        or ctx.jobs > 1
        or (variant_index or 0) > 0
    )

    submitted = False
    for j_idx in range(ctx.jobs):
        if ctx.jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", j_idx + 1, ctx.jobs)
        if enable_seed_update:
            if seed_mgr.update_seeds() == 0:
                raise ValueError(f"Failed to update any seeds for image: {ctx.path}.")
        if pair.prompt == ctx.baseline_prompt and not ctx.request.no_skip:
            continue
        if update is not None:
            progress = f"变体 {variant_no}/{variant_total}，" if variant_total else ""
            update(f"{progress}第 {j_idx + 1}/{ctx.jobs} 次提交")
        _submit_fn(pair.prompt, pair.workflow, ctx.comfyui_url)
        submitted = True
    return submitted


# #region 变体内编辑


def _apply_edits(
    pair: WorkflowPromptPair,
    request: RemixRequest,
    selection: List[Tuple[VariantDimension, Any]],
    history: operation_history.OperationHistory,
) -> None:
    """在单个变体内按固定顺序执行全部编辑。

    顺序：重放历史移除 → 移除 → 调整提示词权重 → 新增 → 调整 Lora/CFG/长宽比。
    编辑是否真的产生改动不做判断，交由提交边界按 prompt 与基线的深比较统一判定。
    """
    if request.replay_removals:
        _replay_history_removals(pair, history, request)
    if request.remove_tags:
        _remove_tags(pair, request)

    # 片段在移除之后才定位：移除会改动文本，目标区域可能因此消失
    fragments = locate_fragments(pair, request)
    weight_manager = WeightManager(pair)
    prompt_edits = [(d, s) for d, s in selection if d.kind == PROMPT_KIND]
    node_edits = [(d, s) for d, s in selection if d.kind != PROMPT_KIND]

    for dimension, step in prompt_edits:
        edit = dimension.edit
        weight_manager.modify_prompt_weights(
            fragments, edit.tag, step, request.skip_add
        )

    if request.add_tags:
        _add_tags(pair, fragments, request)

    for dimension, step in node_edits:
        if dimension.kind == LORA_KIND:
            weight_manager.modify_lora_weights(dimension.edit.name, step)
        else:
            _apply_node_steps(weight_manager, step, dimension.kind)


def _write_cfg_step(manager: WeightManager, node_id: str, value: Any) -> None:
    """把 CFG 档位写入目标 KSampler 节点。"""
    manager.modify_cfg_weights(value, [node_id])


def _write_aspect_step(manager: WeightManager, node_id: str, value: Any) -> None:
    """把长宽比档位写入目标 latent 节点。"""
    manager.modify_aspect_ratio(value[0], value[1], [node_id])


# 档位是「节点 → 目标值」的维度各自如何写入工作流
_NODE_STEP_WRITERS: Dict[str, Callable[[WeightManager, str, Any], None]] = {
    CFG_KIND: _write_cfg_step,
    ASPECT_KIND: _write_aspect_step,
}


def _apply_node_steps(manager: WeightManager, step: NodeStep, kind: str) -> None:
    """把「节点 → 目标值」档位逐节点写入工作流。"""
    writer = _NODE_STEP_WRITERS.get(kind)
    if writer is None:
        raise ValueError(f"Unknown variant dimension kind '{kind}'")
    for node_id, value in step:
        writer(manager, node_id, value)


def _replay_history_removals(
    pair: WorkflowPromptPair,
    history: operation_history.OperationHistory,
    request: RemixRequest,
) -> None:
    """重放历史移除记录。移除已无范围概念，历史记录中的范围参数一律忽略。"""
    records = history.list_remove()
    for record in records:
        _remove_from_clip_nodes(
            pair, [record.prompt], raw=record.raw, hard=record.hard, no_skip=False
        )


def _remove_tags(pair: WorkflowPromptPair, request: RemixRequest) -> None:
    _remove_from_clip_nodes(
        pair,
        list(request.remove_tags),
        raw=request.raw,
        hard=request.hard,
        no_skip=request.no_skip,
    )


def _spellings(prompts: Iterable[str]) -> Set[str]:
    """提示词的匹配形态：原词、空格形态与下划线形态互通，三者都要尝试匹配。"""
    spellings: Set[str] = set()
    for prompt_str in prompts:
        spellings.update(
            (prompt_str, prompt_str.replace("_", " "), prompt_str.replace(" ", "_"))
        )
    return spellings


def _remove_from_clip_nodes(
    pair: WorkflowPromptPair,
    prompts: List[str],
    raw: bool,
    hard: bool,
    no_skip: bool,
) -> None:
    """从工作流所有 CLIPTextEncode 节点移除给定提示词。"""
    for node_id in _clip_text_encode_nodes(pair.prompt):
        fragment = PromptFragment(pair, node_id)
        for spelling in _spellings(prompts):
            fragment.remove(spelling, raw=raw, hard=hard, no_skip=no_skip)


def _add_tags(
    pair: WorkflowPromptPair,
    fragments: List[PromptFragment],
    request: RemixRequest,
) -> None:
    """把标签追加到首个目标片段，默认同时清理其他区域中的同名提示词（--keep 保留）。"""
    fragment = next(iter(fragments), None)
    if fragment is None:
        return

    for tag in request.add_tags:
        fragment.add(tag, raw=request.raw, no_skip=request.no_skip)
        if not request.keep:
            _remove_from_other_regions(
                pair, tag, fragment.node_id, fragment.region, request
            )


# #endregion


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
    request: RemixRequest,
) -> None:
    """从目标节点中非目标区域移除重复的提示词（与移除动作相同的匹配逻辑）。"""
    from .prompt_locator import REGION_START_RE

    spellings = _spellings([prompt_str])

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
        for spelling in spellings:
            # no_skip=True 静默跳过不存在的情况
            fragment.remove(spelling, raw=request.raw, hard=False, no_skip=True)


# #endregion


def handle_set_model_format_cmd(args: Any) -> None:
    """处理 /set-model-format <model> <format> 命令，保存映射配置到 $IMAGE_FUNNEL_DATA_DIR/comfyui_model_formats.toml"""
    model_name = cast(Optional[str], getattr(args, "model", None))
    format_val = cast(Optional[str], getattr(args, "format", None))
    if not model_name or not format_val:
        raise ValueError(
            "Both model and format arguments are required for set-model-format command."
        )

    from .model_format import ModelFormatConfig

    config = ModelFormatConfig.load()
    config.models[model_name] = format_val
    saved_path = config.save()
    _LOGGER.info(
        "Updated model format for %r to %r. Config saved at: %s",
        model_name,
        format_val,
        saved_path,
    )
    print(
        f"Updated model format for '{model_name}' to '{format_val}'. Saved at: {saved_path}"
    )


# #region 命令注册表

# queue 是「无编辑项的重混」，与 remix 共用同一个处理器
_remix_handler = RemixHandler()
COMMAND_HANDLERS: Dict[str, CommandHandler] = {
    "queue": _remix_handler,
    "remix": _remix_handler,
}

# #endregion
