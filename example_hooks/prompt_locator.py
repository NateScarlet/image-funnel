#!/usr/bin/env python
# -*- coding: utf-8 -*-
# pyright: reportPrivateUsage=false
"""
prompt_locator 模块：封装 ComfyUI 提示词节点定位逻辑。

提供 PromptFragment OOP 实体封装，使用迭代器定位并操作目标节点/区域，
使调用者无需关心 marker 标记的具体处理。
"""

import os
import logging
import re
from typing import Dict, List, Tuple, Any, Optional, Set, Iterator, cast, TYPE_CHECKING

if TYPE_CHECKING:
    from workflow_prompt_pair import WorkflowPromptPair

_LOGGER = logging.getLogger(__name__)

# #region 常量定义与环境变量读取

KNOWN_PRIMITIVE_TYPES: Set[str] = {
    "PrimitiveInt",
    "PrimitiveFloat",
    "PrimitiveString",
    "PrimitiveBoolean",
}
KNOWN_SWITCH_TYPES: Set[str] = {"Any Switch (rgthree)", "ComfySwitchNode"}

_DEFAULT_START_REGION_PREFIX = "//#region hook-"
_DEFAULT_END_REGION_PREFIX = "//#endregion hook-"

START_REGION_PREFIX: str = os.getenv(
    "HOOK_START_REGION_PREFIX", _DEFAULT_START_REGION_PREFIX
)
END_REGION_PREFIX: str = os.getenv("HOOK_END_REGION_PREFIX", _DEFAULT_END_REGION_PREFIX)

# #endregion


class PromptFragment:
    """
    表示定位出来的一段提示词片段。
    它可能是整个 CLIPTextEncode 节点的文本，也可能是被特定区域标记包裹的局部文本段。
    """

    def __init__(
        self,
        pair: "WorkflowPromptPair",  # 关联的 WorkflowPromptPair 实例
        node_id: str,
        start_marker: str = "",
        end_marker: str = "",
        use_markers: bool = False,
    ):
        self.pair = pair
        self.node_id = node_id
        self.start_marker = start_marker
        self.end_marker = end_marker
        self.use_markers = use_markers

    @property
    def text(self) -> str:
        """
        获取该片段当前的文本内容（剥离区域 marker）。
        """
        workflow_text = self.pair._get_workflow_node_text(self.node_id)
        if workflow_text is None:
            return ""
        if self.use_markers:
            start_idx = workflow_text.find(self.start_marker)
            end_idx = workflow_text.find(self.end_marker)
            if start_idx != -1 and end_idx != -1 and start_idx < end_idx:
                return workflow_text[start_idx + len(self.start_marker) : end_idx]
        return workflow_text

    def add(self, prompt_str: str, raw: bool = False, no_skip: bool = False) -> bool:
        """
        往该片段追加提示词，支持双轨道同步更新。
        返回 True 表示执行了操作，False 表示跳过。
        """
        return self.pair.process_double_track(
            self.node_id,
            "add",
            prompt_str,
            self.start_marker,
            self.end_marker,
            raw,
            no_skip,
            hard=False,
            use_markers=self.use_markers,
        )

    def remove(
        self,
        prompt_str: str,
        raw: bool = False,
        hard: bool = False,
        no_skip: bool = False,
    ) -> bool:
        """
        从该片段中移除提示词，支持双轨道同步更新。
        返回 True 表示执行了操作，False 表示跳过。
        """
        return self.pair.process_double_track(
            self.node_id,
            "remove",
            prompt_str,
            self.start_marker,
            self.end_marker,
            raw,
            no_skip,
            hard,
            use_markers=self.use_markers,
        )

    def get_weight(self, target_prompt: str) -> Optional[float]:
        """
        在当前片段文本中查找目标提示词的权重。
        """
        text = self.text
        if not text:
            return None

        text = self.pair._strip_comments_for_prompt(text)

        escaped = re.escape(target_prompt)

        # 1. 匹配带权重的格式: (word:1.2)
        pattern_with_weight = re.compile(
            rf"\(\s*{escaped}\s*:\s*([0-9.-]+)\s*\)", re.IGNORECASE
        )
        m = pattern_with_weight.search(text)
        if m:
            return float(m.group(1))

        # 2. 匹配带括号无权重的格式: (word) → 默认权重 1.0
        pattern_brackets = re.compile(rf"\(\s*{escaped}\s*\)", re.IGNORECASE)
        if pattern_brackets.search(text):
            return 1.0

        # 3. 匹配裸词 → 默认权重 1.0
        pattern_bare = re.compile(rf"(?<!\w){escaped}(?!\w)", re.IGNORECASE)
        if pattern_bare.search(text):
            return 1.0

        return None

    def modify_weight(self, target_prompt: str, weight: float, skip_add: bool) -> bool:
        """
        在当前片段中调整目标提示词的权重。
        """
        workflow_text = self.pair._get_workflow_node_text(self.node_id)
        if workflow_text is None:
            return False

        prompt_text = (
            self.pair.prompt[self.node_id]
            .setdefault("inputs", {})
            .setdefault("text", "")
        )
        if not isinstance(prompt_text, str):
            prompt_text = ""

        # 调整权重
        new_workflow_text, mod_wf = self.pair._adjust_prompt_weight_in_text(
            workflow_text, target_prompt, weight
        )
        new_prompt_text, mod_pr = self.pair._adjust_prompt_weight_in_text(
            prompt_text, target_prompt, weight
        )

        if mod_wf or mod_pr:
            self.pair.prompt[self.node_id]["inputs"]["text"] = (
                self.pair._strip_comments_for_prompt(new_prompt_text)
            )
            self.pair._update_workflow_node_text(self.node_id, new_workflow_text)
            return True

        if not skip_add:
            added_text = f"({target_prompt}:{weight})"
            self.pair._add_prompt_to_node(
                self.node_id,
                added_text,
                self.start_marker,
                self.end_marker,
                self.use_markers,
            )
            return True

        return False


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


def get_region_markers(region_name: str) -> Tuple[str, str]:
    """
    根据区域名称拼装 marker 字符串。
    使用 HOOK_START_REGION_PREFIX / HOOK_END_REGION_PREFIX 环境变量作为前缀，追加区域名。
    """
    return START_REGION_PREFIX + region_name, END_REGION_PREFIX + region_name


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


def find_nodes_with_region(
    prompt: Dict[str, Any], workflow: Dict[str, Any], region_name: str
) -> List[str]:
    """
    查找所有包含指定区域 marker 的 CLIPTextEncode 节点 ID。
    """
    start_marker, end_marker = get_region_markers(region_name)
    result: List[str] = []
    for nid, node in prompt.items():
        node_dict = cast(Dict[str, Any], node)
        if node_dict.get("class_type") == "CLIPTextEncode":
            wf_text = get_workflow_node_text(workflow, nid)
            if wf_text and start_marker in wf_text and end_marker in wf_text:
                result.append(nid)
    return result


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


def resolve_target_to_nodes(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    target_type: str,
    target_value: str,
    is_neg: bool,
) -> List[Tuple[str, str, str, bool]]:
    """
    将单个目标（node 或 region）解析为具体的节点列表。
    返回 [(node_id, start_marker, end_marker, use_markers), ...]。
    """
    if target_type == "node":
        if target_value not in prompt:
            _LOGGER.warning(f"Node {target_value} not found in prompt, skipping.")
            return []
        return [(target_value, "", "", False)]
    else:  # region
        start_marker, end_marker = get_region_markers(target_value)
        matching_nodes = find_nodes_with_region(prompt, workflow, target_value)
        if matching_nodes:
            return [(nid, start_marker, end_marker, True) for nid in matching_nodes]
        else:
            fallback_nid = get_target_clip_node(prompt, is_neg)
            if fallback_nid:
                return [(fallback_nid, start_marker, end_marker, True)]
            else:
                _LOGGER.warning(
                    f"Failed to locate target node for region '{target_value}'"
                )
                return []


def locate_prompt_fragments(
    pair: "WorkflowPromptPair",  # WorkflowPromptPair 实例
    nodes: Optional[List[str]] = None,
    regions: Optional[List[str]] = None,
    is_neg: bool = False,
) -> Iterator[PromptFragment]:
    """
    定位提示词目标节点并使用迭代器 yield PromptFragment。
    """
    prompt = pair.prompt
    workflow = pair.workflow

    raw_targets: List[Tuple[str, str]] = []
    if nodes:
        for nid in nodes:
            raw_targets.append(("node", nid))
    if regions:
        for rname in regions:
            raw_targets.append(("region", rname))

    if not raw_targets:
        default_region = "negative" if is_neg else "positive"
        raw_targets.append(("region", default_region))

    for target_type, target_value in raw_targets:
        resolved = resolve_target_to_nodes(
            prompt, workflow, target_type, target_value, is_neg
        )
        for nid, start_marker, end_marker, use_markers in resolved:
            yield PromptFragment(pair, nid, start_marker, end_marker, use_markers)
