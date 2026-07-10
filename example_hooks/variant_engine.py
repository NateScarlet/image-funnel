#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
VariantEngine：变体生成引擎。

从 WorkflowPromptPair 提取变体生成模式，将 "parse weight string → iterate → mutate → yield"
抽象为一个统一接口。每个具体的变异策略实现 MutationStrategy 协议。
"""

import math
import re
from typing import (
    Dict,
    List,
    Any,
    Optional,
    Generator,
    Iterable,
    Tuple,
)

from weight_parser import parse_weights, is_relative
from prompt_locator import find_terminal_input
from weight_manager import WeightManager
from prompt_fragment import PromptFragment

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


def generate_cfg_variants(
    weight_manager: WeightManager,
    prompt: Dict[str, Any],
    weight_expr: str,
    node_ids: Optional[List[str]] = None,
) -> Generator[None, None, None]:
    """
    为每个 CFG 权重变体原地修改并 yield。
    """
    ksampler_cfgs: Dict[str, float] = {}
    for nid, node in prompt.items():
        if node_ids is not None and nid not in node_ids:
            continue
        class_type = node.get("class_type", "")
        if "KSampler" in class_type:
            inputs = node.get("inputs", {})
            if "cfg" in inputs:
                src_nid, src_key = find_terminal_input(prompt, nid, "cfg")
                val = prompt[src_nid]["inputs"].get(src_key)
                if isinstance(val, (int, float)):
                    ksampler_cfgs[nid] = float(val)

    if not ksampler_cfgs:
        return

    node_weights_map: Dict[str, List[float]] = {}
    for nid, cfg_val in ksampler_cfgs.items():
        node_weights_map[nid] = parse_weights(weight_expr, cfg_val)

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
    for vi in range(len(first_weights)):
        for nid in node_weights_map:
            weight_manager.modify_cfg_weights(node_weights_map[nid][vi], [nid])
        yield


def generate_lora_variants(
    weight_manager: WeightManager,
    lora_name_query: str,
    weight_expr: str,
) -> Generator[None, None, None]:
    """
    为每个 Lora 权重变体原地修改并 yield。
    """
    current = (
        weight_manager.get_current_lora_weight(lora_name_query)
        if is_relative(weight_expr)
        else None
    )
    if is_relative(weight_expr) and current is None:
        return
    weights = parse_weights(weight_expr, current)

    for w in weights:
        weight_manager.modify_lora_weights(lora_name_query, w)
        yield


def generate_prompt_variants(
    weight_manager: WeightManager,
    fragments: Iterable[PromptFragment],
    target_prompt: str,
    weight_expr: str,
    skip_add: bool,
) -> Generator[None, None, None]:
    """
    为每个提示词权重变体原地修改并 yield。
    """
    actual_fragments = list(fragments)

    current = None
    if is_relative(weight_expr):
        for fragment in actual_fragments:
            val = fragment.get_weight(target_prompt)
            if val is not None:
                current = val
                break

    if is_relative(weight_expr) and current is None:
        return

    if not is_relative(weight_expr) and skip_add:
        has_existing = False
        for fragment in actual_fragments:
            if fragment.get_weight(target_prompt) is not None:
                has_existing = True
                break
        if not has_existing:
            return

    weights = parse_weights(weight_expr, current)

    for w in weights:
        any_modified = False
        for fragment in actual_fragments:
            if fragment.modify_weight(target_prompt, w, skip_add=True):
                any_modified = True
        if not any_modified and not skip_add:
            if actual_fragments:
                actual_fragments[0].modify_weight(target_prompt, w, skip_add=False)
        yield


def generate_aspect_variants(
    weight_manager: WeightManager,
    prompt: Dict[str, Any],
    nodes_cache: Dict[str, Any],
    ratio_expr: str,
    node_ids: Optional[List[str]] = None,
) -> Generator[None, None, None]:
    """
    为每个长宽比变体原地修改并 yield。
    """
    latent_nodes: List[str] = []
    for nid, node_info in nodes_cache.items():
        if node_ids is not None and nid not in node_ids:
            continue
        if "width" in node_info.inputs and "height" in node_info.inputs:
            latent_nodes.append(nid)

    if not latent_nodes:
        return

    node_variants_map: Dict[str, List[Tuple[int, int]]] = {}
    for nid in latent_nodes:
        w_nid, w_key = find_terminal_input(prompt, nid, "width")
        h_nid, h_key = find_terminal_input(prompt, nid, "height")
        w_val = prompt[w_nid]["inputs"].get(w_key)
        h_val = prompt[h_nid]["inputs"].get(h_key)

        if not isinstance(w_val, (int, float)) or not isinstance(h_val, (int, float)):
            continue

        W = float(w_val)
        H = float(h_val)
        S = W * H
        R_curr = W / H

        curr_idx = _find_closest_ratio_index(R_curr)
        if curr_idx is None:
            continue

        target_ratios: List[float] = []
        ratio_expr_clean = ratio_expr.strip()

        if ratio_expr_clean.lower() in ("swap", "exchange"):
            node_variants_map[nid] = [(int(round(H)), int(round(W)))]
            continue

        m_sym = _parse_symmetric_expression(ratio_expr_clean)
        if m_sym:
            prefix = m_sym.group(1) or "w"
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
            target_ratios = [COMMON_VALUES[idx] for idx in indices]

        else:
            m_shift = re.match(r"^([wh]?)([+-]\d+)$", ratio_expr_clean)
            if m_shift:
                prefix = m_shift.group(1) or "w"
                shift = int(m_shift.group(2))

                effective_shift = -shift if prefix == "h" else shift
                target_idx = max(
                    0, min(len(COMMON_RATIOS) - 1, curr_idx + effective_shift)
                )
                target_ratios = [COMMON_VALUES[target_idx]]

            elif ":" in ratio_expr_clean:
                try:
                    w_part, h_part = ratio_expr_clean.split(":", 1)
                    rw = float(w_part)
                    rh = float(h_part)
                    if rw <= 0 or rh <= 0:
                        raise ValueError()
                    target_ratios = [rw / rh]
                except ValueError:
                    raise ValueError(
                        f"Invalid aspect ratio format: '{ratio_expr_clean}'"
                    )
            else:
                raise ValueError(
                    f"Invalid aspect ratio expression: '{ratio_expr_clean}'"
                )

        variants: List[Tuple[int, int]] = []
        for R in target_ratios:
            W_raw = math.sqrt(S * R)
            H_raw = math.sqrt(S / R)
            W_new = int(round(W_raw / 8) * 8)
            H_new = int(round(H_raw / 8) * 8)
            W_new = max(8, W_new)
            H_new = max(8, H_new)
            variants.append((W_new, H_new))

        node_variants_map[nid] = variants

    if not node_variants_map:
        return

    lengths = {nid: len(vts) for nid, vts in node_variants_map.items()}
    unique_lengths = set(lengths.values())
    if len(unique_lengths) > 1:
        details = ", ".join(f"node {nid}: {l} versions" for nid, l in lengths.items())
        raise ValueError(
            f"Inconsistent aspect ratio version counts generated for latent nodes ({details}) "
            f"under expression '{ratio_expr}'."
        )

    first_variants = list(node_variants_map.values())[0]
    for vi in range(len(first_variants)):
        for nid, variants in node_variants_map.items():
            w_target, h_target = variants[vi]
            weight_manager.modify_aspect_ratio(w_target, h_target, [nid])
        yield


def _find_closest_ratio_index(ratio: float) -> Optional[int]:
    """从 COMMON_VALUES 中查找最接近的比率索引"""
    diffs = [abs(v - ratio) for v in COMMON_VALUES]
    return diffs.index(min(diffs))


def _parse_symmetric_expression(expr: str) -> Optional[Any]:
    """解析对称表达式如 w+-2:2 或 +-3"""
    return re.match(r"^([wh]?)\+-(\d+)(?::(\d+))?$", expr)
