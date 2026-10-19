#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
remix 模块：把单条 /remix 指令的旗标解析为一次重混请求，并展开变体矩阵。

/remix 合并了原先 add / remove / adjust / remove-again 的能力，因此解析分两步：

1. 按旗标归类。`--add` / `--remove` / `--lora` 是**可累加**旗标，重复调用合并进同
   一个变体；`--cfg` / `--aspect` 是**互斥多值**旗标，重复调用各产一个变体。
2. 展开变体矩阵。各多值维度取笛卡尔积，每个变体都从同一份基线工作流出发，
   相对表达式（如 `x-0.1`）因此不会随变体顺序漂移。
"""

import argparse
import itertools
from dataclasses import dataclass
from typing import (
    Any,
    Iterator,
    List,
    Optional,
    Sequence,
    Tuple,
    Union,
    cast,
)

from . import variant_engine
from .prompt_fragment import PromptFragment
from .variant_engine import AspectStep, CfgStep
from .weight_manager import WeightManager
from .weight_parser import looks_like_weight
from .workflow_prompt_pair import WorkflowPromptPair

# 变体档位：提示词/Lora 是单个权重，CFG 是「节点 → 权重」，
# 长宽比是「节点 → 宽高」；后两者都用元组保存以便按值去重
ScalarStep = float
NodeStep = Union[CfgStep, AspectStep]
StepValue = Union[ScalarStep, NodeStep]

# #region 请求模型


@dataclass(frozen=True)
class PromptWeightEdit:
    """一次提示词调权（`--add <tag> <weight>` 的权重部分）：权重只作用于该单个 tag。"""

    tag: str
    weight_expr: str


@dataclass(frozen=True)
class LoraEdit:
    """一次 Lora 调权（`--lora <name> <weight>`）。"""

    name: str
    weight_expr: str


@dataclass(frozen=True)
class RemixRequest:
    """一条 /remix 指令的完整语义：可累加的编辑项、互斥多值项与全局作用旗标。"""

    add_tags: Tuple[str, ...] = ()
    remove_tags: Tuple[str, ...] = ()
    replay_removals: bool = False
    prompt_weights: Tuple[PromptWeightEdit, ...] = ()
    loras: Tuple[LoraEdit, ...] = ()
    cfgs: Tuple[str, ...] = ()
    aspects: Tuple[str, ...] = ()
    neg: bool = False
    regions: Tuple[str, ...] = ()
    nodes: Tuple[str, ...] = ()
    no_skip: bool = False
    skip_add: bool = False
    update_seed: bool = False
    keep: bool = False
    raw: bool = False
    hard: bool = False

    @property
    def has_edits(self) -> bool:
        """是否含任何编辑项。不含编辑项时是否入列仍由提交边界按内容是否变化判定。"""
        return bool(
            self.add_tags
            or self.remove_tags
            or self.replay_removals
            or self.prompt_weights
            or self.loras
            or self.cfgs
            or self.aspects
        )

    @property
    def edits_prompt_content(self) -> bool:
        """是否改动了提示词内容（新增、移除、重放历史移除）而非纯参数调整。

        内容一旦改动，用源图原种子提交就会得到与上一张重复的结果，因此这类编辑
        必须更新种子；纯参数调整则相反，固定种子才便于对照。
        """
        return bool(self.add_tags or self.remove_tags or self.replay_removals)

    @property
    def targets_prompts(self) -> bool:
        """是否需要按区域/节点定位提示词片段（新增与调权都有目标概念，移除没有）。"""
        return bool(self.add_tags or self.prompt_weights)


def _split_add_occurrence(
    tokens: Sequence[str],
) -> Tuple[List[str], Optional[PromptWeightEdit]]:
    """分流一次 `--add` 的参数：末尾 token 是权重表达式时按调权解析，否则全部按新增标签。

    权重表达式只服务于单个 tag——多值加权重没有既有语义可依，直接报错而不是猜测。
    """
    if len(tokens) == 1:
        return list(tokens), None
    if not looks_like_weight(tokens[-1]):
        return list(tokens), None
    if len(tokens) > 2:
        raise ValueError(
            f"--add got multiple prompts with a trailing weight '{tokens[-1]}': "
            f"a weight applies to a single prompt, quote multi-word prompts instead"
        )
    return [tokens[0]], PromptWeightEdit(tag=tokens[0], weight_expr=tokens[-1])


def _parse_lora_occurrence(tokens: Sequence[str]) -> LoraEdit:
    """解析一次 `--lora` 的参数：恰好是 <name> <weight> 两个值。"""
    if len(tokens) != 2:
        raise ValueError(
            f"--lora expects exactly <name> <weight>, got {len(tokens)} value(s): {' '.join(tokens)}"
        )
    name, weight_expr = tokens
    if not looks_like_weight(weight_expr):
        raise ValueError(f"--lora expects a weight expression, got '{weight_expr}'")
    return LoraEdit(name=name, weight_expr=weight_expr)


def from_args(args: argparse.Namespace) -> RemixRequest:
    """把 argparse 解析结果归类为一次重混请求；旗标组合非法时立即报错中止。"""
    add_occurrences = cast("List[List[str]]", args.add or [])
    remove_occurrences = cast("List[List[str]]", args.remove or [])
    lora_occurrences = cast("List[List[str]]", args.lora or [])

    add_tags: List[str] = []
    prompt_weights: List[PromptWeightEdit] = []
    for occurrence in add_occurrences:
        tags, weight_edit = _split_add_occurrence(occurrence)
        if weight_edit is None:
            add_tags.extend(tags)
        else:
            prompt_weights.append(weight_edit)

    loras = [_parse_lora_occurrence(item) for item in lora_occurrences]

    return RemixRequest(
        add_tags=tuple(add_tags),
        remove_tags=tuple(
            tag for occurrence in remove_occurrences for tag in occurrence
        ),
        replay_removals=args.remove_again,
        prompt_weights=tuple(prompt_weights),
        loras=tuple(loras),
        cfgs=tuple(args.cfg or []),
        aspects=tuple(args.aspect or []),
        neg=args.neg,
        regions=tuple(args.region or []),
        nodes=tuple(args.node or []),
        no_skip=args.no_skip,
        skip_add=args.skip_add,
        update_seed=args.update_seed,
        keep=args.keep,
        raw=args.raw,
        hard=args.hard,
    )


def describe(request: RemixRequest) -> str:
    """把重混请求描述为一句人类可读的摘要，用于进度标题与跳过原因。"""
    parts: List[str] = []
    if request.replay_removals:
        parts.append("重放历史移除")
    for tag in request.remove_tags:
        parts.append(f"移除「{tag}」")
    for tag in request.add_tags:
        parts.append(f"新增「{tag}」")
    for edit in request.prompt_weights:
        parts.append(f"「{edit.tag}」权重 {edit.weight_expr}")
    for edit in request.loras:
        parts.append(f"Lora「{edit.name}」权重 {edit.weight_expr}")
    for expr in request.cfgs:
        parts.append(f"CFG {expr}")
    for expr in request.aspects:
        parts.append(f"长宽比 {expr}")
    return "，".join(parts)


# #endregion

# #region 变体矩阵

# 维度种类：决定档位如何被应用到工作流上
PROMPT_KIND = "prompt"
LORA_KIND = "lora"
CFG_KIND = "cfg"
ASPECT_KIND = "aspect"


@dataclass(frozen=True)
class VariantDimension:
    """一个产出多档的编辑维度。steps 为各档取值，按基线工作流的当前值算出。"""

    kind: str
    label: str
    # kind 决定 edit 与 steps 的具体类型：prompt → PromptWeightEdit + float 档，
    # lora → LoraEdit + float 档，cfg / aspect → None + 「节点 → 目标值」档
    edit: Any
    steps: Tuple[StepValue, ...]


@dataclass(frozen=True)
class VariantMatrix:
    """变体矩阵：维度定义与各变体的档位选择一一对应。

    `variants[i][j]` 是第 i 个变体在第 j 个维度上选中的档位，None 表示该维度无变化
    （恒等），此时该维度的编辑不参与本次变体。
    """

    dimensions: Tuple[VariantDimension, ...]
    variants: Tuple[Tuple[Optional[StepValue], ...], ...]

    def __len__(self) -> int:
        return len(self.variants)

    def iter_variants(
        self,
    ) -> Iterator[List[Tuple["VariantDimension", StepValue]]]:
        """按变体顺序逐个给出「维度 → 档位」配对，跳过无变化的维度。"""
        for selection in self.variants:
            yield [
                (dimension, step)
                for dimension, step in zip(self.dimensions, selection)
                if step is not None
            ]


def _merge_steps(step_groups: Sequence[Sequence[StepValue]]) -> Tuple[StepValue, ...]:
    """合并互斥多值旗标各次调用展开出的档位：顺序去重后作为该维度的全部档位。"""
    merged: List[StepValue] = []
    for group in step_groups:
        for step in group:
            if step not in merged:
                merged.append(step)
    return tuple(merged)


def _cartesian(
    dimensions: Sequence[VariantDimension],
) -> Tuple[Tuple[Optional[StepValue], ...], ...]:
    """各维度档位取笛卡尔积。无变化（空档位）的维度按恒等处理，不使整个积归零。"""
    per_dimension: List[Tuple[Optional[StepValue], ...]] = [
        cast("Tuple[Optional[StepValue], ...]", dim.steps or (None,))
        for dim in dimensions
    ]
    if not per_dimension:
        return ((),)
    return tuple(itertools.product(*per_dimension))


def build_matrix(request: RemixRequest, pair: WorkflowPromptPair) -> VariantMatrix:
    """展开重混请求的变体矩阵。档位只依赖基线工作流，因此各变体互不影响。"""
    weight_manager = WeightManager(pair)
    fragments = locate_fragments(pair, request)

    dimensions: List[VariantDimension] = []
    for edit in request.prompt_weights:
        dimensions.append(
            VariantDimension(
                kind=PROMPT_KIND,
                label=f"提示词「{edit.tag}」权重",
                edit=edit,
                steps=tuple(
                    variant_engine.prompt_steps(
                        fragments, edit.tag, edit.weight_expr, request.skip_add
                    )
                ),
            )
        )
    for edit in request.loras:
        dimensions.append(
            VariantDimension(
                kind=LORA_KIND,
                label=f"Lora「{edit.name}」权重",
                edit=edit,
                steps=tuple(
                    variant_engine.lora_steps(
                        weight_manager, edit.name, edit.weight_expr
                    )
                ),
            )
        )

    node_ids = list(request.nodes) or None
    if request.cfgs:
        dimensions.append(
            VariantDimension(
                kind=CFG_KIND,
                label="CFG",
                edit=None,
                steps=_merge_steps(
                    [
                        variant_engine.cfg_steps(weight_manager, expr, node_ids)
                        for expr in request.cfgs
                    ]
                ),
            )
        )
    if request.aspects:
        dimensions.append(
            VariantDimension(
                kind=ASPECT_KIND,
                label="长宽比",
                edit=None,
                steps=_merge_steps(
                    [
                        variant_engine.aspect_steps(weight_manager, expr, node_ids)
                        for expr in request.aspects
                    ]
                ),
            )
        )

    return VariantMatrix(tuple(dimensions), _cartesian(dimensions))


def locate_fragments(
    pair: WorkflowPromptPair, request: RemixRequest
) -> List[PromptFragment]:
    """按区域/节点/正负向定位提示词片段（新增与调权的目标）。"""
    if not request.targets_prompts:
        return []
    return list(
        pair.locate_prompts(
            nodes=list(request.nodes) or None,
            regions=list(request.regions) or None,
            is_neg=request.neg,
        )
    )


# #endregion
