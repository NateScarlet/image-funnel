#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
SeedManager：管理 ComfyUI 工作流中的随机种子更新。
"""

import random
from typing import Dict, Any, List

from .node_accessor import NodeAccessor
from .prompt_locator import KNOWN_PRIMITIVE_TYPES, find_terminal_input, NodeInfo


class SeedManager:
    """
    种子管理器，负责检测和更新 ComfyUI 工作流中的随机种子。
    接收已分析好的种子节点列表，不负责节点分析。
    """

    def __init__(self, accessor: NodeAccessor, seed_nodes: List[NodeInfo]):
        self._accessor = accessor
        self._seed_nodes = seed_nodes

    def has_seeds(self) -> bool:
        """检查是否有种子需要更新。"""
        return len(self._seed_nodes) > 0

    def update_seeds(self) -> int:
        """
        修改 prompt (API 结构) 和 workflow (UI 结构) 中的随机种子值。
        使用缓存的种子节点信息，避免重复分析。
        返回成功修改的种子总数。
        """
        modified_count: int = 0

        for node_info in self._seed_nodes:
            if node_info.is_disabled:
                continue

            widgets_values = node_info.widgets_values
            if not widgets_values:
                continue

            for idx in range(len(widgets_values) - 1):
                val: Any = widgets_values[idx]
                val_next: Any = widgets_values[idx + 1]
                if isinstance(val, int) and val_next in [
                    "fixed",
                    "increment",
                    "decrement",
                    "randomize",
                ]:
                    old_seed: int = val
                    strategy: str = str(val_next)

                    new_seed: int
                    if strategy == "fixed":
                        new_seed = old_seed
                    elif strategy == "increment":
                        new_seed = (old_seed + 1) % 1125899906842624
                    elif strategy == "decrement":
                        new_seed = old_seed - 1
                        if new_seed < 0:
                            new_seed = 0
                    else:  # randomize
                        new_seed = random.randint(1, 1125899906842624)

                    # A. 更新 workflow 中的数值
                    widgets_values[idx] = new_seed
                    modified_count += 1

                    # B. 映射到 API (prompt) 端节点 ID 列表
                    api_node_ids: List[str] = []
                    if not node_info.is_subgraph:
                        api_node_ids = [node_info.node_id]
                    else:
                        for cached_info in self._accessor.nodes_cache.values():
                            if (
                                not cached_info.is_subgraph
                                and cached_info.node_type == node_info.subgraph_id
                            ):
                                if not cached_info.is_disabled:
                                    api_node_ids.append(
                                        f"{cached_info.node_id}:{node_info.node_id}"
                                    )

                    prompt = self._accessor.prompt
                    for api_node_id in api_node_ids:
                        if api_node_id not in prompt:
                            raise ValueError(
                                f"Workflow seed node {api_node_id} is missing in the prompt API structure."
                            )

                        inputs: Dict[str, Any] = prompt[api_node_id].get("inputs", {})
                        for ik in list(inputs.keys()):
                            src_node_id: str
                            src_key: str
                            src_node_id, src_key = find_terminal_input(
                                prompt, api_node_id, ik
                            )
                            if src_node_id not in prompt:
                                raise ValueError(
                                    f"Source node {src_node_id} for seed tracking is missing in the prompt API structure."
                                )

                            src_node: Dict[str, Any] = prompt[src_node_id]
                            src_inputs: Dict[str, Any] = src_node.get("inputs", {})
                            current_val: Any = src_inputs.get(src_key)

                            is_primitive = (
                                src_node.get("class_type") in KNOWN_PRIMITIVE_TYPES
                            )
                            if (
                                current_val == old_seed
                                or str(current_val) == str(old_seed)
                            ) and ("seed" in ik or "seed" in src_key or is_primitive):
                                src_inputs[src_key] = new_seed

        return modified_count
