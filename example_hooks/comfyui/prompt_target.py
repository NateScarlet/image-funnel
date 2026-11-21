#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""目标提示词节点/区域定位：把「/remix 修饰参数 + 工作流结构 → 目标片段/目标节点」
的判定收敛在单一深 module 中。

提交路径（remix.locate_fragments）与补全路径（autocomplete）共用这里导出的判定：
显式 --node / --region 优先、多目标取第一个、默认区域按正/负向选择、区域标记提取、
移除作用于全部 CLIPTextEncode 节点等规则只在此处表达一次。
"""

import os
from typing import (
    Any,
    Dict,
    Iterable,
    Iterator,
    List,
    Optional,
    Sequence,
    Set,
    Tuple,
    TypeVar,
    cast,
)

from .node_accessor import NodeAccessor
from .prompt_locator import REGION_START_RE

# 补全时最常用的两个区域固定置顶，其余区域保持扫描顺序
_PREFERRED_REGION_NAMES = ("positive", "negative")


# #region 区域标记提取


def region_names_in_text(text: str) -> List[str]:
    """扫描单段文本中的区域标记名，按出现顺序去重返回。"""
    names: List[str] = []
    seen: Set[str] = set()
    for match in REGION_START_RE.finditer(text):
        name = match.group(1)
        if name and name not in seen:
            seen.add(name)
            names.append(name)
    return names


def extract_region_names(workflows: Iterable[Dict[str, Any]]) -> Iterator[str]:
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
            widget_values_list: List[Any] = cast("List[Any]", widgets_values)
            for raw_val in widget_values_list:
                if not isinstance(raw_val, str):
                    continue
                for name in region_names_in_text(raw_val):
                    if name not in seen:
                        seen.add(name)
                        scanned_names.append(name)

    # 置顶需要先看到全部区域，因此这里先收集再按偏好顺序产出
    for preferred_name in _PREFERRED_REGION_NAMES:
        if preferred_name in seen:
            yield preferred_name
    for name in scanned_names:
        if name not in _PREFERRED_REGION_NAMES:
            yield name


# #endregion

# #region 目标装配与默认选点


def target_specs(
    nodes: Optional[Sequence[str]] = None,
    regions: Optional[Sequence[str]] = None,
    is_neg: bool = False,
) -> List[Tuple[str, str]]:
    """把修饰参数展开为 (类型, 值) 目标序列：显式 --node、--region 依次在前，最后才是默认区域。"""
    specs: List[Tuple[str, str]] = [("node", n) for n in nodes or []]
    specs.extend(("region", r) for r in regions or [])
    if not specs:
        specs.append(("region", "negative" if is_neg else "positive"))
    return specs


def _find_clip_text_nodes(prompt: Dict[str, Any], start_val: Any) -> List[str]:
    """递归追溯 conditioning 链路，找到所有叶子 CLIPTextEncode 节点 ID。"""
    if not isinstance(start_val, list):
        return []

    val_list = cast(List[Any], start_val)
    if len(val_list) != 2:
        return []

    node_id = str(val_list[0])
    node = cast(Dict[str, Any], prompt.get(node_id, {}))
    if not node:
        return []

    class_type = str(node.get("class_type", ""))
    if class_type == "CLIPTextEncode":
        return [node_id]

    results: List[str] = []
    inputs = cast(Dict[str, Any], node.get("inputs", {}))

    if class_type == "ConditioningConcat":
        for key in ["conditioning_to", "conditioning_from"]:
            if key in inputs:
                results.extend(_find_clip_text_nodes(prompt, inputs[key]))
    elif class_type == "ConditioningCombine":
        for key in ["conditioning1", "conditioning2"]:
            if key in inputs:
                results.extend(_find_clip_text_nodes(prompt, inputs[key]))
    else:
        for key, value in inputs.items():
            if "conditioning" in key.lower() and isinstance(value, list):
                results.extend(_find_clip_text_nodes(prompt, value))

    return results


def _default_clip_node_id(prompt: Dict[str, Any], is_neg: bool) -> Optional[str]:
    """按极性选默认 CLIPTextEncode 节点：先沿 KSampler 的对应端口追溯，无线索时按关键词与文本长度降级。"""
    ksampler_ids = [
        nid
        for nid, node in prompt.items()
        if "KSampler" in cast(Dict[str, Any], node).get("class_type", "")
    ]
    clip_nodes: List[str] = []
    target_port = "negative" if is_neg else "positive"

    for kid in ksampler_ids:
        inputs = cast(
            Dict[str, Any], cast(Dict[str, Any], prompt[kid]).get("inputs", {})
        )
        if target_port in inputs:
            clip_nodes.extend(_find_clip_text_nodes(prompt, inputs[target_port]))

    clip_nodes = list(set(clip_nodes))

    if not clip_nodes:
        clip_nodes = [
            nid
            for nid, node in prompt.items()
            if cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
        ]

        if not clip_nodes:
            return None

        keywords_str = os.getenv(
            "HOOK_NEGATIVE_KEYWORDS" if is_neg else "HOOK_POSITIVE_KEYWORDS", ""
        )
        if not keywords_str:
            if is_neg:
                keywords_str = "worst quality, low quality, score_1, score_2, score_3"
            else:
                keywords_str = "masterpiece, best quality, score_7"

        keywords = [k.strip().lower() for k in keywords_str.split(",") if k.strip()]

        best_node_id: Optional[str] = None
        max_matches = -1
        max_len = -1
        for nid in clip_nodes:
            node = cast(Dict[str, Any], prompt.get(nid, {}))
            text_val = cast(Dict[str, Any], node.get("inputs", {})).get("text", "")
            if isinstance(text_val, str):
                text_normalized = text_val.lower().replace("_", " ")
                matches = sum(
                    1 for kw in keywords if kw.replace("_", " ") in text_normalized
                )

                if matches > max_matches:
                    max_matches = matches
                    max_len = len(text_val)
                    best_node_id = nid
                elif matches == max_matches:
                    if len(text_val) > max_len:
                        max_len = len(text_val)
                        best_node_id = nid

        return best_node_id

    best_node_id: Optional[str] = None
    max_len = -1
    for nid in clip_nodes:
        node = cast(Dict[str, Any], prompt.get(nid, {}))
        text_val = cast(Dict[str, Any], node.get("inputs", {})).get("text", "")
        if isinstance(text_val, str):
            if len(text_val) > max_len:
                max_len = len(text_val)
                best_node_id = nid

    return best_node_id


def clip_text_encode_node_ids(prompt: Dict[str, Any]) -> List[str]:
    """返回 prompt 中所有 CLIPTextEncode 节点 ID；移除是全局操作，不做范围限定。"""
    return [
        str(nid)
        for nid, node in prompt.items()
        if isinstance(node, dict)
        and cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
    ]


def locate_targets(
    accessor: NodeAccessor,
    nodes: Optional[Sequence[str]] = None,
    regions: Optional[Sequence[str]] = None,
    is_neg: bool = False,
) -> List[Tuple[str, str]]:
    """把修饰参数解析为 (node_id, region) 目标列表。

    显式节点/区域按输入顺序排列；区域找不到标记节点时回落到默认 CLIPTextEncode 选点。
    """
    targets: List[Tuple[str, str]] = []
    for spec_type, spec_value in target_specs(nodes, regions, is_neg):
        if spec_type == "node":
            if spec_value in accessor.prompt:
                targets.append((spec_value, ""))
            continue

        matching = [
            nid
            for nid in clip_text_encode_node_ids(accessor.prompt)
            if spec_value
            in region_names_in_text(accessor.get_workflow_node_text(nid) or "")
        ]
        if matching:
            targets.extend((nid, spec_value) for nid in matching)
            continue

        fallback = _default_clip_node_id(accessor.prompt, is_neg)
        if fallback:
            targets.append((fallback, spec_value))
    return targets


_T = TypeVar("_T")


def primary_target(targets: Sequence[_T]) -> Optional[_T]:
    """多目标时取第一个：提交与补全共用同一首目标选择。"""
    return targets[0] if targets else None


# #endregion
