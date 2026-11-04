#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
命令处理器模块：将 comfyui.py 中的命令调度逻辑拆分为独立 Handler。

每个 Handler 接收 CommandContext 并在其中执行对应操作。
"""

import copy
import datetime
import json
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
from .prompt_fragment import PromptFragment, RemovalHit
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


class SubmissionLedger:
    """一次钩子执行内已提交的 prompt 台账。

    prompt 是提交给 ComfyUI 的完整 API 结构，内容相同就意味着产出一张完全一样的图，
    同一执行内重复提交没有意义。台账由最外层入口创建并注入，跨图片共享。
    """

    def __init__(self) -> None:
        self._digests: Set[str] = set()

    @staticmethod
    def digest(prompt: Dict[str, Any]) -> str:
        """把 prompt 归一化为可比较的文本：键序不影响判定。"""
        return json.dumps(prompt, sort_keys=True, ensure_ascii=False)

    def is_submitted(self, prompt: Dict[str, Any]) -> bool:
        return self.digest(prompt) in self._digests

    def record(self, prompt: Dict[str, Any]) -> None:
        self._digests.add(self.digest(prompt))


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
        submission_ledger: SubmissionLedger,
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
        # 本次 hook 执行内已提交的 prompt 台账，由最外层入口创建并在各图片间共享
        self.submission_ledger = submission_ledger
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
    """remix/queue 命令：按旗标集合执行 ComfyUI 编辑并提交。

    种子以 job 为单位：每个 job 先生成一份基线（至多更新一次种子），同一 job 内的
    所有变体共用该种子。是否入列由三道判定决定：编辑后相对所属 job 基线是否有改动、
    是否与源图基线相同（空转保护）、本次执行内是否已提交过同一 prompt。`-S` 放行全部。
    """

    def run(self, ctx: CommandContext) -> None:
        baselines = _job_baselines(ctx)
        submitted = 0
        if ctx.request.has_edits:
            submitted = self._submit_variants(ctx, baselines)
        else:
            # 无编辑项时基线本身即变体
            for pair in baselines:
                if _submit_pair(ctx, pair):
                    submitted += 1

        if submitted == 0:
            ctx.skip(SKIP_REASON_NO_CHANGE)
            return
        ctx.update_label()

    def _submit_variants(
        self, ctx: CommandContext, baselines: List[WorkflowPromptPair]
    ) -> int:
        request = ctx.request
        # 矩阵只依赖基线取值，因此每个 job 各自求一次：相对表达式以所属 job 基线的当前值为准
        matrices = [
            build_matrix(
                request,
                WorkflowPromptPair(
                    copy.deepcopy(baseline.workflow), copy.deepcopy(baseline.prompt)
                ),
            )
            for baseline in baselines
        ]
        total = sum(len(matrix) for matrix in matrices)
        variant_no = 0
        submitted = 0

        with progress_notification(
            ctx.client, ctx.progress_tag, "ComfyUI 重混", "重混并提交"
        ) as update:
            for baseline, matrix in zip(baselines, matrices):
                for selection in matrix.iter_variants():
                    variant_no += 1
                    # 每个变体都从所属 job 的基线出发，不继承其他变体的中间状态
                    pair = WorkflowPromptPair(
                        copy.deepcopy(baseline.workflow),
                        copy.deepcopy(baseline.prompt),
                    )
                    _apply_edits(pair, request, selection, ctx.history)
                    if not request.no_skip and pair.prompt == baseline.prompt:
                        # 编辑后与所属 job 基线相同：该变体本次不参与
                        continue
                    if _submit_pair(ctx, pair, update, f"变体 {variant_no}/{total}"):
                        submitted += 1
        return submitted


def _job_baselines(ctx: CommandContext) -> List[WorkflowPromptPair]:
    """按 -j 生成每个 job 的基线：从前一个 job 拷贝、至多更新一次种子，并按 prompt 去重。

    种子是 job 级取值：job 1 沿用源图种子，job 2..N 各取一个新种子。基线逐 job 递延，
    因此 increment/decrement 等策略会逐轮推进；固定种子的工作流各 job 得到完全相同的
    prompt，去重后只剩一份基线，`-j N` 不会排出 N 张相同的图。`-S` 绕过去重，让每个
    job 各自提交。
    """
    baselines: List[WorkflowPromptPair] = []
    current = WorkflowPromptPair(copy.deepcopy(ctx.workflow), copy.deepcopy(ctx.prompt))
    for job_index in range(ctx.jobs):
        if job_index > 0:
            current = WorkflowPromptPair(
                copy.deepcopy(current.workflow), copy.deepcopy(current.prompt)
            )
        if _job_updates_seed(ctx.request, job_index):
            if SeedManager(current, current.seed_nodes).update_seeds() == 0:
                raise ValueError(f"Failed to update any seeds for image: {ctx.path}.")
        if not ctx.request.no_skip and any(
            existing.prompt == current.prompt for existing in baselines
        ):
            continue
        baselines.append(current)
    return baselines


def _job_updates_seed(request: RemixRequest, job_index: int) -> bool:
    """job 1 沿用源图种子（重新入列或 -u 时除外）；job 2..N 各取一个新种子。"""
    return job_index > 0 or not request.has_edits or request.update_seed


def _submit_pair(
    ctx: CommandContext,
    pair: WorkflowPromptPair,
    update: Optional[ProgressUpdate] = None,
    progress: str = "",
) -> bool:
    """提交一份工作流副本，返回是否真的发生了提交。

    入列前的最终关卡，queue 路径、无编辑项路径与每个变体路径共用：先与源图基线
    深比较（fixed 种子工作流上的重新入列因此仍被空转保护跳过），再查执行内台账
    （同一执行内完全相同的 prompt 只发一次）。`-S` 绕过以上全部判定。
    """
    if not ctx.request.no_skip:
        if pair.prompt == ctx.baseline_prompt:
            return False
        if ctx.submission_ledger.is_submitted(pair.prompt):
            return False
    if update is not None:
        update(progress)
    _submit_fn(pair.prompt, pair.workflow, ctx.comfyui_url)
    ctx.submission_ledger.record(pair.prompt)
    return True


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

    anchor: Optional[RemovalHit] = None
    if request.remove_tags:
        # 移除本身产出唯一命中行的位置，供同一变体内的新增原地插入
        anchor = _remove_tags(pair, request)
        if request.regions or request.nodes:
            # 显式 --region / --node 始终优先于锚点
            anchor = None

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
        _add_tags(pair, fragments, request, anchor)

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


def _remove_tags(
    pair: WorkflowPromptPair, request: RemixRequest
) -> Optional[RemovalHit]:
    """按请求参数全局移除提示词，返回本次唯一命中行的原地插入锚点。"""
    return _remove_from_clip_nodes(
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
) -> Optional[RemovalHit]:
    """从工作流所有 CLIPTextEncode 节点移除给定提示词。

    返回本次移除唯一命中的那一行（供同一变体内的新增原地插入）；未命中或命中多行
    返回 None。命中位置由移除本身产出，与移除共用同一份文本快照，不再单独预扫描。
    """
    hits: List[RemovalHit] = []
    for node_id in _clip_text_encode_nodes(pair.prompt):
        fragment = PromptFragment(pair, node_id)
        for spelling in _spellings(prompts):
            hits.extend(
                fragment.remove(spelling, raw=raw, hard=hard, no_skip=no_skip).hits
            )
    if len(hits) != 1:
        return None
    return hits[0]


def _add_tags(
    pair: WorkflowPromptPair,
    fragments: List[PromptFragment],
    request: RemixRequest,
    anchor: Optional[RemovalHit],
) -> None:
    """把标签追加到首个目标片段，默认同时清理其他区域中的同名提示词（--keep 保留）。

    存在移除锚点（本次唯一被移除的行）时改为从该行之后原地插入：锚点优先于默认
    positive/negative 区域，但显式 --region / --node 已排除锚点。多条标签一次性
    写入后再清理其它区域，避免插入之间文本被重排导致行号漂移。
    """
    if anchor is not None:
        fragment = PromptFragment(pair, anchor.node_id, region=anchor.region)
        inserted = fragment.insert(
            request.add_tags,
            anchor.anchor,
            raw=request.raw,
            no_skip=request.no_skip,
        )
        if inserted is not None:
            if not request.keep:
                for tag in request.add_tags:
                    _remove_from_other_regions(
                        pair, tag, anchor.node_id, anchor.region, request
                    )
            return
        # 锚点依附行在中间编辑后已不存在：回落默认目标

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
