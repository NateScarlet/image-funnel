#!/usr/bin/env python
# -*- coding: utf-8 -*-
# pyright: reportPrivateUsage=false
"""
prompt_locator 模块：封装 ComfyUI 提示词节点定位逻辑。

提供纯定位函数，用于在 prompt/workflow 结构中查找目标 CLIPTextEncode 节点，
不涉及文本操作的任何细节。
"""

import os
import re
from dataclasses import dataclass
from typing import Dict, List, Tuple, Any, Optional, Set, cast

# prompt_locator 是纯定位工具模块，不依赖其他 example_hooks 模块


@dataclass
class NodeInfo:
    """节点信息缓存"""

    node_id: str
    node_data: Dict[str, Any]  # workflow 中的节点对象引用
    prompt_data: Dict[str, Any]  # prompt 中的节点对象引用（可能为空）
    node_type: str  # workflow 中的 type
    class_type: str  # prompt 中的 class_type
    is_disabled: bool
    is_subgraph: bool
    subgraph_id: Optional[str]
    widgets_values: Optional[List[Any]]
    inputs: Dict[str, Any]


def is_node_disabled(node: Dict[str, Any]) -> bool:
    """
    检查节点是否被停用 (Mute/Bypass)。
    mode 值为 2 (Never/Mute) 或 4 (Bypass)。
    """
    return node.get("mode") in (2, 4)


# #region Region 标记匹配

# 标准 // #region / // #endregion 注释语法（ComfyUI 工作流需以 // 开头注释）
# 允许 // 前有缩进，允许 // 与 # 之间有任意空格，endregion 可附带名称作为描述
REGION_START_RE = re.compile(r"^\s*//\s*#region\s+(\S+)", re.MULTILINE)
REGION_END_RE = re.compile(r"^\s*//\s*#endregion\b", re.MULTILINE)

# #endregion

KNOWN_PRIMITIVE_TYPES: Set[str] = {
    "PrimitiveInt",
    "PrimitiveFloat",
    "PrimitiveString",
    "PrimitiveBoolean",
}
KNOWN_SWITCH_TYPES: Set[str] = {"Any Switch (rgthree)", "ComfySwitchNode"}


def find_terminal_input(
    prompt: Dict[str, Any], node_id: str, input_key: str
) -> Tuple[str, str]:
    """
    在 prompt 结构中顺着连接线递归向下追溯，直到找到直接存储具体数值的叶子节点输入。
    返回 (src_node_id, src_input_key) 元组。
    """
    node: Dict[str, Any] = prompt.get(node_id, {})
    inputs: Dict[str, Any] = node.get("inputs", {})
    val: Any = inputs.get(input_key)

    # 如果该输入是一个连接，格式形如 [target_node_id, slot_index]
    if isinstance(val, list):
        val_list = cast(List[Any], val)
        if len(val_list) == 2 and isinstance(val_list[0], str):
            target_node_id: str = val_list[0]
            target_node: Dict[str, Any] = prompt.get(target_node_id, {})
            target_class: str = target_node.get("class_type", "")

            # 1. 针对精确匹配的 Primitive 节点，其真实值存储在 inputs.value 中
            if target_class in KNOWN_PRIMITIVE_TYPES:
                return find_terminal_input(prompt, target_node_id, "value")

            # 2. 针对精确匹配的 Switch 路由中转节点，我们只追溯其有效的输入端口
            elif target_class in KNOWN_SWITCH_TYPES:
                if target_class == "ComfySwitchNode":
                    # ComfySwitchNode 的输入中转端口是 on_false 和 on_true
                    for ik in ["on_false", "on_true"]:
                        if ik in target_node.get("inputs", {}):
                            res: Tuple[str, str] = find_terminal_input(
                                prompt, target_node_id, ik
                            )
                            if res:
                                return res
                elif target_class == "Any Switch (rgthree)":
                    # Any Switch (rgthree) 的输入中转端口以 any_ 开头
                    for ik, iv in target_node.get("inputs", {}).items():
                        if ik.startswith("any_"):
                            res: Tuple[str, str] = find_terminal_input(
                                prompt, target_node_id, ik
                            )
                            if res:
                                return res

            # 3. 针对其他可能中转信号的自定义节点，我们继续追溯其列表型端口
            else:
                for ik, iv in target_node.get("inputs", {}).items():
                    iv_list = cast(List[Any], iv) if isinstance(iv, list) else []
                    if len(iv_list) == 2:
                        res: Tuple[str, str] = find_terminal_input(
                            prompt, target_node_id, ik
                        )
                        if res:
                            return res

    return (node_id, input_key)


def find_region_boundaries(text: str, region_name: str) -> Tuple[int, int]:
    """
    在文本中查找指定 region 的边界。
    返回 (start_pos, endregion_pos)，其中：
      - start_pos:     // #region {name} 行的起始位置
      - endregion_pos: // #endregion 行的起始位置（不含该行内容）
    未找到时返回 (-1, -1)。
    不支持嵌套 region。
    """
    for match in REGION_START_RE.finditer(text):
        if match.group(1) == region_name:
            end_match = REGION_END_RE.search(text, match.end())
            if end_match:
                return (match.start(), end_match.start())
            return (-1, -1)
    return (-1, -1)


def get_region_content(text: str, region_name: str) -> Optional[str]:
    """
    获取 region 内部的内容（去除 // #region 和 // #endregion 标记行）。
    未找到 region 时返回 None。
    """
    start, endregion_start = find_region_boundaries(text, region_name)
    if start == -1:
        return None

    line_end = text.find("\n", start)
    if line_end == -1:
        return ""

    content = text[line_end + 1 : endregion_start]
    if content.endswith("\n"):
        content = content[:-1]
    return content


def get_region_markers(region_name: str) -> Tuple[str, str]:
    """
    返回标准 region 标记字符串。
    使用标准 // #region name / // #endregion 语法。
    """
    return (f"// #region {region_name}", f"// #endregion {region_name}")


def get_workflow_node_text(workflow: Dict[str, Any], node_id_str: str) -> Optional[str]:
    """
    在 UI 结构 workflow 中获取特定节点的文本 widget 数值。
    """
    for node in workflow.get("nodes", []):
        if str(node.get("id")) == node_id_str:
            widgets_values = node.get("widgets_values")
            if isinstance(widgets_values, list) and widgets_values:
                if isinstance(widgets_values[0], str):
                    return widgets_values[0]

    if ":" in node_id_str:
        _, child_id = node_id_str.split(":", 1)
        subgraphs = workflow.get("definitions", {}).get("subgraphs", [])
        for subgraph in subgraphs:
            for node in subgraph.get("nodes", []):
                if str(node.get("id")) == child_id:
                    widgets_values = node.get("widgets_values")
                    if isinstance(widgets_values, list) and widgets_values:
                        if isinstance(widgets_values[0], str):
                            return widgets_values[0]

    return None


def find_clip_text_nodes(prompt: Dict[str, Any], start_val: Any) -> List[str]:
    """
    递归追溯 conditioning 链路，找到所有叶子 CLIPTextEncode 节点 ID
    """
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
        for k in ["conditioning_to", "conditioning_from"]:
            if k in inputs:
                results.extend(find_clip_text_nodes(prompt, inputs[k]))
    elif class_type == "ConditioningCombine":
        for k in ["conditioning1", "conditioning2"]:
            if k in inputs:
                results.extend(find_clip_text_nodes(prompt, inputs[k]))
    else:
        for k, v in inputs.items():
            if "conditioning" in k.lower() and isinstance(v, list):
                results.extend(find_clip_text_nodes(prompt, v))

    return results


def get_target_clip_node(prompt: Dict[str, Any], is_neg: bool) -> Optional[str]:
    """
    定位目标 CLIPTextEncode 节点。
    优先顺着所有 KSampler 节点的 positive/negative 输入线索追溯；若无，降级为文本最长的 CLIPTextEncode 节点。
    """
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
            clip_nodes.extend(find_clip_text_nodes(prompt, inputs[target_port]))

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
