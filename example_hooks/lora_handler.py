#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
Lora 节点处理器模块。

定义 LoraLoaderHandler 抽象接口及其实现，用于处理 ComfyUI 中不同类型 Lora 节点的权重读写。
"""

from abc import ABC, abstractmethod
from typing import Dict, List, Set, Any, Optional, cast, TYPE_CHECKING

from prompt_locator import find_terminal_input, NodeInfo

if TYPE_CHECKING:
    from workflow_prompt_pair import WorkflowPromptPair


class LoraLoaderHandler(ABC):
    """Lora 节点处理抽象接口"""

    @property
    @abstractmethod
    def node_types(self) -> Set[str]:
        """该处理器能处理的 ComfyUI 节点类型集合"""
        pass

    @abstractmethod
    def collect_names(self, node_dict: Dict[str, Any]) -> List[str]:
        """从 prompt 节点中提取所有的 Lora 文件名"""
        pass

    @abstractmethod
    def get_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
    ) -> Optional[float]:
        """从节点信息中获取匹配的 Lora 权重"""
        pass

    @abstractmethod
    def modify_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
        weight: float,
    ) -> bool:
        """同时修改 prompt 和 workflow 中的 Lora 权重，修改成功返回 True"""
        pass


class NativeLoraLoaderHandler(LoraLoaderHandler):
    """原生 LoraLoader 节点处理器"""

    @property
    def node_types(self) -> Set[str]:
        return {"LoraLoader"}

    def collect_names(self, node_dict: Dict[str, Any]) -> List[str]:
        lora_name = node_dict.get("inputs", {}).get("lora_name", "")
        if isinstance(lora_name, str) and lora_name:
            return [lora_name]
        return []

    def get_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
    ) -> Optional[float]:
        # 1. 只从参与运行的 prompt 提取数据
        if node_info.prompt_data:
            inputs = node_info.prompt_data.get("inputs", {})
            lora_name = inputs.get("lora_name", "")
            if isinstance(lora_name, str) and query_lower in lora_name.lower():
                for ik in ["strength_model", "strength_clip"]:
                    if ik in inputs:
                        src_nid, src_key = find_terminal_input(
                            pair.prompt, node_info.node_id, ik
                        )
                        val = pair.prompt[src_nid]["inputs"].get(src_key)
                        if isinstance(val, (int, float)):
                            return float(val)
        return None

    def modify_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
        weight: float,
    ) -> bool:
        # 如果 prompt_data 缺失（说明节点不参与实际执行图），直接跳过不予修改
        if not node_info.prompt_data:
            return False

        modified = False
        inputs = node_info.prompt_data.get("inputs", {})
        lora_name = inputs.get("lora_name", "")
        if isinstance(lora_name, str) and query_lower in lora_name.lower():
            for ik in ["strength_model", "strength_clip"]:
                if ik in inputs:
                    src_nid, src_key = find_terminal_input(
                        pair.prompt, node_info.node_id, ik
                    )
                    pair.prompt[src_nid]["inputs"][src_key] = weight
                    if src_nid != node_info.node_id:
                        wf_node = pair.get_node_by_id(src_nid)
                        if wf_node and wf_node.widgets_values:
                            wf_node.widgets_values[0] = weight
                    modified = True

            # 只有在修改 prompt 成功后，才同步修改该节点在 workflow 中的 widgets_values
            if modified:
                if (
                    not node_info.widgets_values
                    or not isinstance(node_info.widgets_values[0], str)
                    or query_lower not in node_info.widgets_values[0].lower()
                ):
                    raise ValueError(
                        f"Workflow data is out of sync with prompt for LoraLoader node {node_info.node_id}"
                    )
                if len(node_info.widgets_values) > 1 and isinstance(
                    node_info.widgets_values[1], (int, float)
                ):
                    node_info.widgets_values[1] = weight
                if len(node_info.widgets_values) > 2 and isinstance(
                    node_info.widgets_values[2], (int, float)
                ):
                    node_info.widgets_values[2] = weight

        return modified


class PowerLoraLoaderHandler(LoraLoaderHandler):
    """Power Lora Loader (rgthree) 节点处理器"""

    @property
    def node_types(self) -> Set[str]:
        return {"Power Lora Loader (rgthree)"}

    def collect_names(self, node_dict: Dict[str, Any]) -> List[str]:
        names: List[str] = []
        for k, v in node_dict.get("inputs", {}).items():
            if k.startswith("lora_") and isinstance(v, dict):
                v_dict = cast(Dict[str, Any], v)
                lora_path = v_dict.get("lora", "")
                if isinstance(lora_path, str) and lora_path:
                    names.append(lora_path)
        return names

    def get_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
    ) -> Optional[float]:
        # 1. 只从参与运行的 prompt 提取数据
        if node_info.prompt_data:
            inputs = node_info.prompt_data.get("inputs", {})
            for k, v in inputs.items():
                if k.startswith("lora_") and isinstance(v, dict):
                    v_dict = cast(Dict[str, Any], v)
                    lora_path = v_dict.get("lora", "")
                    if isinstance(lora_path, str) and query_lower in lora_path.lower():
                        strength = v_dict.get("strength")
                        if isinstance(strength, (int, float)):
                            return float(strength)
        return None

    def modify_weight(
        self,
        node_info: NodeInfo,
        pair: "WorkflowPromptPair",
        query_lower: str,
        weight: float,
    ) -> bool:
        # 如果 prompt_data 缺失（说明节点不参与实际执行图），直接跳过不予修改
        if not node_info.prompt_data:
            return False

        modified = False
        inputs = node_info.prompt_data.get("inputs", {})
        modified_loras: List[str] = []
        for k, v in list(inputs.items()):
            if k.startswith("lora_") and isinstance(v, dict):
                v_dict = cast(Dict[str, Any], v)
                lora_path = v_dict.get("lora", "")
                if isinstance(lora_path, str) and query_lower in lora_path.lower():
                    v_dict["strength"] = weight
                    modified_loras.append(lora_path)
                    modified = True

        # 只有在修改 prompt 成功后，才同步修改该节点在 workflow 中的 widgets_values
        if modified:
            if not node_info.widgets_values:
                raise ValueError(
                    f"Workflow widgets values are missing for rgthree node {node_info.node_id}"
                )
            for ml in modified_loras:
                slot_found = False
                for val in node_info.widgets_values:
                    if isinstance(val, dict) and "lora" in val:
                        val_dict = cast(Dict[str, Any], val)
                        lora_path = val_dict.get("lora", "")
                        if (
                            isinstance(lora_path, str)
                            and ml.lower() in lora_path.lower()
                        ):
                            val_dict["strength"] = weight
                            slot_found = True
                if not slot_found:
                    raise ValueError(
                        f"Workflow Lora slot for '{ml}' is out of sync with prompt in rgthree node {node_info.node_id}"
                    )

        return modified


LORA_HANDLERS: List[LoraLoaderHandler] = [
    NativeLoraLoaderHandler(),
    PowerLoraLoaderHandler(),
]
