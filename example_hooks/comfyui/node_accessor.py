#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
NodeAccessor 协议：定义对 ComfyUI 节点数据的读写接口。
切断 PromptFragment 和 LoraLoaderHandler 对 WorkflowPromptPair 内部数据布局的直接依赖。
"""

from typing import Any, Dict, Optional, Protocol

from .prompt_locator import NodeInfo


class NodeAccessor(Protocol):
    """节点数据读写接口，隐藏底层 prompt/workflow 字典布局"""

    def get_workflow_node_text(self, node_id: str) -> Optional[str]:
        """获取 workflow 节点文本 widget 值"""
        ...

    def update_workflow_node_text(
        self, node_id: str, text: str, input_key: str = "value"
    ) -> None:
        """更新 workflow 节点文本 widget 值"""
        ...

    def get_prompt_input(self, node_id: str, input_key: str) -> Any:
        """获取 prompt 节点指定 input 的值（只读）"""
        ...

    def set_prompt_input(self, node_id: str, input_key: str, value: Any) -> None:
        """设置 prompt 节点指定 input 的值"""
        ...

    def get_prompt_node(self, node_id: str) -> Optional[Dict[str, Any]]:
        """获取 prompt 节点的全部数据"""
        ...

    def setdefault_prompt_input(
        self, node_id: str, input_key: str, default: Any
    ) -> Any:
        """类似 dict.setdefault，确保 input_key 存在并返回值"""
        ...

    def get_node_by_id(self, node_id: str) -> Optional[NodeInfo]:
        """获取节点信息"""
        ...

    @property
    def prompt(self) -> Dict[str, Any]:
        """底层 prompt 字典（过渡期使用）"""
        ...

    @property
    def workflow(self) -> Dict[str, Any]:
        """底层 workflow 字典（过渡期使用）"""
        ...

    @property
    def nodes_cache(self) -> Dict[str, NodeInfo]:
        """节点分析缓存"""
        ...
