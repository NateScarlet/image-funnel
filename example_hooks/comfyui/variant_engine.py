#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
变体档位展开。

把「权重/长宽比表达式 → 逐档取值」抽象为纯函数：档位只依赖基线工作流的当前值，
不产生任何副作用，因此 /remix 的编排层可以自由地对多个维度取笛卡尔积、让每个变体
都从同一份基线出发，而不会出现变体之间互相污染（相对表达式含义随顺序漂移）。

应用档位由调用方负责：CFG/长宽比档位是「节点 → 目标值」的映射，提示词/Lora 档位
是单个数值。
"""

import math
import re
from typing import (
    Any,
    Dict,
    List,
    Iterable,
    Optional,
    Tuple,
)

from .weight_parser import parse_weights, is_relative
from .weight_manager import WeightManager
from .prompt_fragment import PromptFragment

# CFG 档位：按节点列出的目标权重（用元组保存以便去重与哈希）
CfgStep = Tuple[Tuple[str, float], ...]
# 长宽比档位：按节点列出的目标宽高
AspectStep = Tuple[Tuple[str, Tuple[int, int]], ...]

COMMON_RATIOS = [
    "5:12",
    "4:7",
    "13:19",
    "7:9",
    "1:1",
    "9:7",
    "19:13",
    "7:4",
    "12:5",
]
COMMON_VALUES = [
    5 / 12,
    4 / 7,
    13 / 19,
    7 / 9,
    1.0,
    9 / 7,
    19 / 13,
    1.75,
    2.4,
]

# #region 档位展开


def cfg_steps(
    weight_manager: WeightManager,
    weight_expr: str,
    node_ids: Optional[List[str]] = None,
) -> List[CfgStep]:
    """把 CFG 权重表达式展开为逐档的目标值；所有目标节点取值都等于当前值的档为恒等，不产出。"""
    sources = weight_manager.collect_cfg_sources(node_ids)
    if not sources:
        return []

    node_weights_map: Dict[str, List[float]] = {}
    current_map: Dict[str, float] = {}
    for src in sources:
        node_weights_map[src.node_id] = parse_weights(weight_expr, src.current_value)
        current_map[src.node_id] = src.current_value

    version_lengths = {nid: len(w) for nid, w in node_weights_map.items()}
    unique_lengths = set(version_lengths.values())
    if len(unique_lengths) > 1:
        details = ", ".join(
            f"node {nid}: {l} versions" for nid, l in version_lengths.items()
        )
        raise ValueError(
            f"Inconsistent weights version counts generated for KSampler nodes ({details}) "
            f"under expression '{weight_expr}'. Please filter targets using --node to resolve ambiguity."
        )

    first_weights = list(node_weights_map.values())[0]
    steps: List[CfgStep] = []
    for vi in range(len(first_weights)):
        targets = {nid: weights[vi] for nid, weights in node_weights_map.items()}
        if all(targets[nid] == current_map[nid] for nid in targets):
            continue
        steps.append(tuple(sorted(targets.items())))
    return steps


def lora_steps(
    weight_manager: WeightManager,
    lora_name_query: str,
    weight_expr: str,
) -> List[float]:
    """把 Lora 权重表达式展开为逐档取值；相对表达式解析不出当前权重时不产出档位。"""
    relative = is_relative(weight_expr)
    current = (
        weight_manager.get_current_lora_weight(lora_name_query) if relative else None
    )
    if relative and current is None:
        return []
    return parse_weights(weight_expr, current)


def prompt_steps(
    fragments: Iterable[PromptFragment],
    target_prompt: str,
    weight_expr: str,
    skip_add: bool,
) -> List[float]:
    """把提示词权重表达式展开为逐档取值；相对表达式解析不出当前权重时不产出档位。"""
    actual_fragments = list(fragments)
    relative = is_relative(weight_expr)

    current: Optional[float] = None
    if relative:
        for fragment in actual_fragments:
            val = fragment.get_weight(target_prompt)
            if val is not None:
                current = val
                break
        if current is None:
            return []

    if not relative and skip_add:
        if not any(f.get_weight(target_prompt) is not None for f in actual_fragments):
            return []

    return parse_weights(weight_expr, current)


def aspect_steps(
    weight_manager: WeightManager,
    ratio_expr: str,
    node_ids: Optional[List[str]] = None,
) -> List[AspectStep]:
    """把长宽比表达式展开为逐档的目标宽高；所有目标节点取值都等于当前值的档为恒等，不产出。"""
    sources = weight_manager.collect_aspect_sources(node_ids)
    if not sources:
        return []

    node_variants_map: Dict[str, List[Tuple[int, int]]] = {}
    current_map: Dict[str, Tuple[int, int]] = {}
    swap = ratio_expr.strip().lower() in ("swap", "exchange")

    for src in sources:
        W = src.current_width
        H = src.current_height
        # 当前尺寸统一到整数再比较：档位是按 8 对齐取整后的值
        current_map[src.node_id] = (int(round(W)), int(round(H)))

        # swap/exchange 直接交换宽高，与目标比例序列无关
        if swap:
            node_variants_map[src.node_id] = [(int(round(H)), int(round(W)))]
            continue

        curr_idx = _find_closest_ratio_index(W / H)
        if curr_idx is None:
            continue

        S = W * H
        variants: List[Tuple[int, int]] = []
        for R in _compute_target_ratios(ratio_expr, curr_idx):
            W_new = max(8, int(round(math.sqrt(S * R) / 8) * 8))
            H_new = max(8, int(round(math.sqrt(S / R) / 8) * 8))
            variants.append((W_new, H_new))
        node_variants_map[src.node_id] = variants

    if not node_variants_map:
        return []

    lengths = {nid: len(vts) for nid, vts in node_variants_map.items()}
    unique_lengths = set(lengths.values())
    if len(unique_lengths) > 1:
        details = ", ".join(f"node {nid}: {l} versions" for nid, l in lengths.items())
        raise ValueError(
            f"Inconsistent aspect ratio version counts generated for latent nodes ({details}) "
            f"under expression '{ratio_expr}'."
        )

    first_variants = list(node_variants_map.values())[0]
    steps: List[AspectStep] = []
    for vi in range(len(first_variants)):
        targets = {nid: variants[vi] for nid, variants in node_variants_map.items()}
        if all(targets[nid] == current_map[nid] for nid in targets):
            continue
        steps.append(tuple(sorted(targets.items())))
    return steps


# #endregion

# #region 长宽比表达式解析


def _compute_target_ratios(ratio_expr: str, curr_idx: int) -> List[float]:
    """从比例表达式中解析出目标比率列表。"""
    ratio_expr_clean = ratio_expr.strip()

    m_sym = _parse_symmetric_expression(ratio_expr_clean)
    if m_sym:
        delta = int(m_sym.group(2))
        step = int(m_sym.group(3)) if m_sym.group(3) else 1

        start_idx = max(0, curr_idx - delta)
        end_idx = min(len(COMMON_RATIOS) - 1, curr_idx + delta)

        indices: List[int] = []
        for offset in range(0, delta + 1, step):
            l_idx = curr_idx - offset
            if l_idx >= start_idx:
                indices.append(l_idx)
            r_idx = curr_idx + offset
            if r_idx <= end_idx:
                indices.append(r_idx)

        indices = sorted(list(set(indices)))
        return [COMMON_VALUES[idx] for idx in indices]

    m_shift = re.match(r"^([wh]?)([+-]?\d+)$", ratio_expr_clean)
    if m_shift:
        prefix = m_shift.group(1) or "w"
        shift = int(m_shift.group(2))
        if shift == 0:
            return [COMMON_VALUES[curr_idx]]

        direction = 1 if shift > 0 else -1
        abs_n = abs(shift)
        effective_direction = -direction if prefix == "h" else direction

        indices = []
        for i in range(1, abs_n + 1):
            offset = effective_direction * i
            target_idx = max(0, min(len(COMMON_RATIOS) - 1, curr_idx + offset))
            indices.append(target_idx)

        return [COMMON_VALUES[idx] for idx in dict.fromkeys(indices)]

    if ":" in ratio_expr_clean:
        try:
            w_part, h_part = ratio_expr_clean.split(":", 1)
            rw = float(w_part)
            rh = float(h_part)
            if rw <= 0 or rh <= 0:
                raise ValueError()
            return [rw / rh]
        except ValueError:
            raise ValueError(f"Invalid aspect ratio format: '{ratio_expr_clean}'")

    raise ValueError(f"Invalid aspect ratio expression: '{ratio_expr_clean}'")


def _find_closest_ratio_index(ratio: float) -> Optional[int]:
    """从 COMMON_VALUES 中查找最接近的比率索引"""
    diffs = [abs(v - ratio) for v in COMMON_VALUES]
    return diffs.index(min(diffs))


def _parse_symmetric_expression(expr: str) -> Optional[Any]:
    """解析对称表达式如 w+-2:2 或 +-3"""
    return re.match(r"^([wh]?)\+-(\d+)(?::(\d+))?$", expr)


# #endregion
