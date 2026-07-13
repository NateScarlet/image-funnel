#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
WeightManager：管理 ComfyUI 工作流中的权重调整操作（CFG / LoRA / Prompt / Aspect）。
"""

import logging
from dataclasses import dataclass
from typing import Dict, List, Any, Optional, Set, cast

from .node_accessor import NodeAccessor
from .prompt_locator import (
    KNOWN_PRIMITIVE_TYPES,
    find_terminal_input,
)
from .lora_handler import LORA_HANDLERS
from .prompt_fragment import PromptFragment

_LOGGER = logging.getLogger(__name__)


# #region CFG / Aspect 源节点数据


@dataclass
class CfgSource:
    node_id: str
    src_node_id: str
    src_key: str
    current_value: float


@dataclass
class AspectSource:
    node_id: str
    width_src_node_id: str
    width_src_key: str
    height_src_node_id: str
    height_src_key: str
    current_width: float
    current_height: float


# #endregion


class WeightManager:
    """
    权重管理器，负责 CFG、LoRA、提示词权重和长宽比的读取与修改。
    """

    def __init__(self, accessor: NodeAccessor):
        self._accessor = accessor

    # #region LoRA 权重

    @staticmethod
    def collect_lora_names(prompt: Dict[str, Any]) -> List[str]:
        """
        从 prompt 元数据中提取所有 lora 文件名。
        """
        names: List[str] = []
        seen: Set[str] = set()
        for node in prompt.values():
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            for handler in LORA_HANDLERS:
                if class_type in handler.node_types:
                    for name in handler.collect_names(node_dict):
                        if name and name not in seen:
                            seen.add(name)
                            names.append(name)
                    break
        return names

    def get_current_lora_weight(self, lora_name_query: str) -> Optional[float]:
        """在 prompt 和 workflow 中查找匹配的 Lora 节点，返回其当前权重。"""
        query_lower = lora_name_query.lower()
        for node_info in self._accessor.nodes_cache.values():
            if node_info.is_disabled:
                continue
            for handler in LORA_HANDLERS:
                if (
                    node_info.node_type in handler.node_types
                    or node_info.class_type in handler.node_types
                ):
                    val = handler.get_weight(node_info, self._accessor, query_lower)
                    if val is not None:
                        return val
                    break
        return None

    def modify_lora_weights(self, lora_name_query: str, weight: float) -> None:
        """修改 Lora 权重，直接修改原对象。"""
        query_lower = lora_name_query.lower()
        for node_info in self._accessor.nodes_cache.values():
            if node_info.is_disabled:
                continue
            for handler in LORA_HANDLERS:
                if (
                    node_info.node_type in handler.node_types
                    or node_info.class_type in handler.node_types
                ):
                    handler.modify_weight(
                        node_info, self._accessor, query_lower, weight
                    )
                    break

    # #endregion

    # #region CFG 权重

    def collect_cfg_sources(
        self, node_ids: Optional[List[str]] = None
    ) -> List[CfgSource]:
        """在 prompt 中查找 KSampler 节点的 CFG 源节点及当前值。
        返回所有匹配的 CfgSource 列表，直接值或通过追溯终端节点获取。"""
        prompt = self._accessor.prompt
        sources: List[CfgSource] = []
        for nid, node in prompt.items():
            if node_ids is not None and nid not in node_ids:
                continue
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            if "KSampler" in class_type:
                inputs = node_dict.get("inputs", {})
                if "cfg" in inputs:
                    src_nid, src_key = find_terminal_input(prompt, nid, "cfg")
                    val = prompt[src_nid]["inputs"].get(src_key)
                    if isinstance(val, (int, float)):
                        sources.append(
                            CfgSource(
                                node_id=nid,
                                src_node_id=src_nid,
                                src_key=src_key,
                                current_value=float(val),
                            )
                        )
        return sources

    def get_current_cfg_weight(
        self, node_ids: Optional[List[str]] = None
    ) -> Optional[float]:
        """在 prompt 中查找 KSampler 节点的当前 CFG 值。"""
        sources = self.collect_cfg_sources(node_ids)
        if sources:
            return sources[0].current_value
        return None

    def modify_cfg_weights(
        self, weight: float, node_ids: Optional[List[str]] = None
    ) -> int:
        """修改 KSampler 的 CFG 权重，返回实际修改的 source 节点数。"""
        prompt = self._accessor.prompt
        sources = self.collect_cfg_sources(node_ids)

        modified = 0
        for src in sources:
            if src.src_node_id in prompt:
                prompt[src.src_node_id]["inputs"][src.src_key] = weight
                modified += 1

        # Update workflow widgets
        for node_info in self._accessor.nodes_cache.values():
            if node_ids is not None and node_info.node_id not in node_ids:
                continue
            if node_info.is_disabled:
                continue

            if "KSampler" in node_info.node_type:
                wv = node_info.widgets_values
                if wv:
                    if node_info.node_type == "KSampler" and len(wv) >= 4:
                        if isinstance(wv[3], (int, float)):
                            wv[3] = weight
                    elif node_info.node_type == "KSamplerAdvanced" and len(wv) >= 5:
                        if isinstance(wv[4], (int, float)):
                            wv[4] = weight

            elif node_info.class_type in KNOWN_PRIMITIVE_TYPES:
                for src in sources:
                    if src.src_node_id == node_info.node_id:
                        wv = node_info.widgets_values
                        if wv and isinstance(wv[0], (int, float)):
                            wv[0] = weight

        return modified

    # #endregion

    # #region 提示词权重

    def get_current_prompt_weight(
        self, fragments: List[PromptFragment], target_prompt: str
    ) -> Optional[float]:
        """在目标节点的文本中查找匹配的提示词，返回其当前权重。"""
        for f in fragments:
            val = f.get_weight(target_prompt)
            if val is not None:
                return val
        return None

    def modify_prompt_weights(
        self,
        fragments: List[PromptFragment],
        target_prompt: str,
        weight: float,
        skip_add: bool,
    ) -> None:
        """在 CLIPTextEncode 节点中调整提示词的权重（原地修改）。"""
        any_existing_modified = False
        for f in fragments:
            if f.modify_weight(target_prompt, weight, skip_add=True):
                any_existing_modified = True

        if not any_existing_modified and not skip_add:
            if fragments:
                fragments[0].modify_weight(target_prompt, weight, skip_add=False)

    # #endregion

    # #region 长宽比

    def collect_aspect_sources(
        self, node_ids: Optional[List[str]] = None
    ) -> List[AspectSource]:
        """在 prompt 中查找具有 width/height 输入的节点的源节点及当前值。
        返回所有匹配的 AspectSource 列表，通过追溯终端节点获取。"""
        prompt = self._accessor.prompt
        sources: List[AspectSource] = []
        for nid, node_info in self._accessor.nodes_cache.items():
            if node_ids is not None and nid not in node_ids:
                continue
            if "width" in node_info.inputs and "height" in node_info.inputs:
                w_nid, w_key = find_terminal_input(prompt, nid, "width")
                h_nid, h_key = find_terminal_input(prompt, nid, "height")
                w_val = prompt[w_nid]["inputs"].get(w_key)
                h_val = prompt[h_nid]["inputs"].get(h_key)
                if isinstance(w_val, (int, float)) and isinstance(h_val, (int, float)):
                    sources.append(
                        AspectSource(
                            node_id=nid,
                            width_src_node_id=w_nid,
                            width_src_key=w_key,
                            height_src_node_id=h_nid,
                            height_src_key=h_key,
                            current_width=float(w_val),
                            current_height=float(h_val),
                        )
                    )
        return sources

    def modify_aspect_ratio(
        self,
        target_width: int,
        target_height: int,
        node_ids: Optional[List[str]] = None,
    ) -> int:
        """修改指定包含 width 和 height 的节点的长宽比，返回实际修改的 source 节点数。"""
        prompt = self._accessor.prompt
        sources = self.collect_aspect_sources(node_ids)

        modified = 0
        for src in sources:
            if src.width_src_node_id in prompt:
                prompt[src.width_src_node_id]["inputs"][
                    src.width_src_key
                ] = target_width
                modified += 1
            if src.height_src_node_id in prompt:
                prompt[src.height_src_node_id]["inputs"][
                    src.height_src_key
                ] = target_height
                modified += 1

        # Update workflow widgets
        for nid, node_info in self._accessor.nodes_cache.items():
            if node_ids is not None and nid not in node_ids:
                continue
            if not node_info.is_disabled:
                wv = node_info.widgets_values
                if wv and len(wv) >= 2:
                    is_source_node = any(src.node_id == nid for src in sources)
                    if is_source_node and isinstance(wv[0], (int, float)):
                        wv[0] = target_width
                    if is_source_node and isinstance(wv[1], (int, float)):
                        wv[1] = target_height

            for cached_info in self._accessor.nodes_cache.values():
                if cached_info.is_disabled:
                    continue
                if cached_info.class_type in KNOWN_PRIMITIVE_TYPES:
                    wv = cached_info.widgets_values
                    if wv and isinstance(wv[0], (int, float)):
                        for src in sources:
                            if cached_info.node_id == src.width_src_node_id:
                                wv[0] = target_width
                            elif cached_info.node_id == src.height_src_node_id:
                                wv[0] = target_height

        return modified

    # #endregion
