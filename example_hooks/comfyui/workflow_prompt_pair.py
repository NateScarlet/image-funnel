#!/usr/bin/env python
# -*- coding: utf-8 -*-
# pyright: reportRedeclaration=false
"""
WorkflowPromptPair：ComfyUI workflow 和 prompt 配对容器。

负责节点分析和缓存，实现 NodeAccessor 协议供领域模块使用。
不包含种子、文件名、权重、提交等业务逻辑——这些在对应领域模块中。
"""

import logging
from typing import Dict, List, Any, Optional, Iterator, Tuple, cast

from .prompt_locator import (
    REGION_START_RE,
    get_target_clip_node,
    NodeInfo,
    is_node_disabled,
)
from .prompt_fragment import PromptFragment

_LOGGER = logging.getLogger(__name__)


class WorkflowPromptPair:
    """ComfyUI workflow/prompt 容器 + 节点分析缓存"""

    def __init__(self, workflow: Dict[str, Any], prompt: Dict[str, Any]):
        self._workflow = workflow
        self._prompt = prompt
        self._nodes_cache: Dict[str, NodeInfo] = {}
        self._title_to_node: Dict[str, str] = {}
        self._seed_nodes: List[NodeInfo] = []
        self._date_filename_nodes: List[NodeInfo] = []
        self._analyze_nodes()

    @property
    def workflow(self) -> Dict[str, Any]:
        return self._workflow

    @property
    def prompt(self) -> Dict[str, Any]:
        return self._prompt

    @property
    def nodes_cache(self) -> Dict[str, NodeInfo]:
        return self._nodes_cache

    @property
    def seed_nodes(self) -> List[NodeInfo]:
        return self._seed_nodes

    @property
    def date_filename_nodes(self) -> List[NodeInfo]:
        return self._date_filename_nodes

    @property
    def title_to_node(self) -> Dict[str, str]:
        return self._title_to_node

    # #region NodeAccessor 接口实现

    def get_workflow_node_text(self, node_id: str) -> Optional[str]:
        node_info = self.get_node_by_id(node_id)
        if node_info is None:
            return None
        wv = node_info.widgets_values
        if wv and isinstance(wv[0], str):
            return wv[0]
        return None

    def update_workflow_node_text(self, node_id: str, text: str) -> None:
        node_info = self.get_node_by_id(node_id)
        if node_info is None:
            return
        wv = node_info.widgets_values
        if wv and len(wv) > 0:
            wv[0] = text

    def get_prompt_input(self, node_id: str, input_key: str) -> Any:
        return self._prompt.get(node_id, {}).get("inputs", {}).get(input_key, "")

    def set_prompt_input(self, node_id: str, input_key: str, value: Any) -> None:
        self._prompt.setdefault(node_id, {}).setdefault("inputs", {})[input_key] = value

    def get_prompt_node(self, node_id: str) -> Optional[Dict[str, Any]]:
        return self._prompt.get(node_id)

    def setdefault_prompt_input(
        self, node_id: str, input_key: str, default: Any
    ) -> Any:
        return (
            self._prompt.setdefault(node_id, {})
            .setdefault("inputs", {})
            .setdefault(input_key, default)
        )

    def get_node_by_id(self, node_id: str) -> Optional[NodeInfo]:
        if node_id in self._nodes_cache:
            return self._nodes_cache[node_id]
        if ":" in node_id:
            parent_id, child_id = node_id.split(":", 1)
            for key in self._nodes_cache:
                if key.startswith("subgraph:") and key.endswith(f":{child_id}"):
                    info = self._nodes_cache[key]
                    if info.subgraph_id:
                        parent_info = self._nodes_cache.get(parent_id)
                        if parent_info and parent_info.node_type == info.subgraph_id:
                            return info
        return None

    # #endregion

    # #region 节点分析

    def _analyze_nodes(self):
        for node in self._workflow.get("nodes", []):
            nid = str(node.get("id"))
            disabled = is_node_disabled(node)
            node_type = node.get("type", "")
            wv_raw: Any = node.get("widgets_values")
            wv: Optional[List[Any]] = None
            if isinstance(wv_raw, list):
                wv = cast(List[Any], wv_raw)
            pd = self._prompt.get(nid, {})
            cls = pd.get("class_type", "")
            inp = pd.get("inputs", {})

            info = NodeInfo(
                node_id=nid,
                node_data=node,
                prompt_data=pd,
                node_type=node_type,
                class_type=cls,
                is_disabled=disabled,
                is_subgraph=False,
                subgraph_id=None,
                widgets_values=wv,
                inputs=inp,
            )
            self._nodes_cache[nid] = info
            self._check_seed_and_date(info, wv, disabled)

        for sub in self._workflow.get("definitions", {}).get("subgraphs", []):
            sg_id = sub.get("id")
            for node in sub.get("nodes", []):
                child_nid = str(node.get("id"))
                disabled = is_node_disabled(node)
                node_type = node.get("type", "")
                wv_raw = node.get("widgets_values")
                wv = None
                if isinstance(wv_raw, list):
                    wv = cast(List[Any], wv_raw)
                info = NodeInfo(
                    node_id=child_nid,
                    node_data=node,
                    prompt_data={},
                    node_type=node_type,
                    class_type="",
                    is_disabled=disabled,
                    is_subgraph=True,
                    subgraph_id=sg_id,
                    widgets_values=wv,
                    inputs={},
                )
                self._nodes_cache[f"subgraph:{sg_id}:{child_nid}"] = info
                self._check_seed_and_date(info, wv, disabled)

        for pid, pnode in self._prompt.items():
            title = pnode.get("_meta", {}).get("title")
            if title:
                self._title_to_node[title] = pid

    def _check_seed_and_date(
        self, info: NodeInfo, wv: Optional[List[Any]], disabled: bool
    ):
        if disabled or not wv:
            return
        for idx in range(len(wv) - 1):
            if isinstance(wv[idx], int) and wv[idx + 1] in [
                "fixed",
                "increment",
                "decrement",
                "randomize",
            ]:
                self._seed_nodes.append(info)
                break
        for val in wv:
            if isinstance(val, str) and "%date:" in val:
                self._date_filename_nodes.append(info)
                break

    # #endregion

    # #region 节点查询

    def find_nodes(self, **criteria: Any) -> List[NodeInfo]:
        result: List[NodeInfo] = []
        for info in self._nodes_cache.values():
            if "node_type" in criteria and info.node_type != criteria["node_type"]:
                continue
            if "class_type" in criteria and info.class_type != criteria["class_type"]:
                continue
            if (
                "is_subgraph" in criteria
                and info.is_subgraph != criteria["is_subgraph"]
            ):
                continue
            if "node_id" in criteria and info.node_id != criteria["node_id"]:
                continue
            result.append(info)
        return result

    def find_nodes_by_class_type(self, class_type: str) -> List[NodeInfo]:
        return self.find_nodes(class_type=class_type)

    def find_nodes_by_node_type(self, node_type: str) -> List[NodeInfo]:
        return self.find_nodes(node_type=node_type)

    def locate_prompts(
        self,
        nodes: Optional[List[str]] = None,
        regions: Optional[List[str]] = None,
        is_neg: bool = False,
    ) -> Iterator[PromptFragment]:
        raw_targets: List[Tuple[str, str]] = []
        if nodes:
            for n in nodes:
                raw_targets.append(("node", n))
        if regions:
            for r in regions:
                raw_targets.append(("region", r))
        if not raw_targets:
            raw_targets.append(("region", "negative" if is_neg else "positive"))
        for tt, tv in raw_targets:
            yield from self._resolve_target_to_nodes(tt, tv, is_neg)

    def _find_nodes_with_region(self, region_name: str) -> List[str]:
        result: List[str] = []
        for nid, raw_node in self._prompt.items():
            node = cast(Dict[str, Any], raw_node)
            if node.get("class_type") == "CLIPTextEncode":
                wf_text = self.get_workflow_node_text(nid)
                if wf_text:
                    for m in REGION_START_RE.finditer(wf_text):
                        if m.group(1) == region_name:
                            result.append(nid)
                            break
        return result

    def _resolve_target_to_nodes(
        self,
        target_type: str,
        target_value: str,
        is_neg: bool,
    ) -> List[PromptFragment]:
        if target_type == "node":
            if target_value not in self._prompt:
                return []
            return [PromptFragment(self, target_value)]
        matching = self._find_nodes_with_region(target_value)
        if matching:
            return [PromptFragment(self, n, region=target_value) for n in matching]
        fallback = get_target_clip_node(self._prompt, is_neg)
        if fallback:
            return [PromptFragment(self, fallback, region=target_value)]
        return []

    # #endregion
