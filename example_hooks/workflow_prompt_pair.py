#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
WorkflowPromptPair 类：封装 ComfyUI workflow 和 prompt 配对操作。

通过一次性分析所有节点并缓存信息，避免重复遍历节点树，
所有修改操作直接在原对象上进行，不需要深拷贝。
"""

import os
import datetime
import json
import logging
import random
import re
import uuid
import urllib.request
from dataclasses import dataclass
from typing import Dict, List, Any, Optional, cast, Tuple, Generator

from weight_parser import parse_weights, is_relative

_LOGGER = logging.getLogger(__name__)

# #region 常量定义
KNOWN_PRIMITIVE_TYPES = {
    "PrimitiveInt",
    "PrimitiveFloat",
    "PrimitiveString",
    "PrimitiveBoolean",
}
KNOWN_SWITCH_TYPES = {"Any Switch (rgthree)", "ComfySwitchNode"}
# #endregion


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


class WorkflowPromptPair:
    """封装 ComfyUI workflow 和 prompt 配对操作"""

    def __init__(self, workflow: Dict[str, Any], prompt: Dict[str, Any]):
        self.workflow = workflow
        self.prompt = prompt
        # 缓存节点信息，避免重复遍历
        self._nodes_cache: Dict[str, NodeInfo] = {}
        # _meta.title → node_id 映射，供按标题查找用
        self._title_to_node: Dict[str, str] = {}
        # 缓存种子节点信息
        self._seed_nodes: List[NodeInfo] = []
        # 缓存文件名节点信息（带日期模板）
        self._date_filename_nodes: List[NodeInfo] = []
        self._analyze_nodes()

    def _analyze_nodes(self):
        """一次性分析所有节点，缓存节点信息、种子节点和文件名节点"""
        # 1. 收集工作流顶层节点
        for node in self.workflow.get("nodes", []):
            node_id_str = str(node.get("id"))
            if is_node_disabled(node):
                continue

            node_type = node.get("type", "")
            widgets_values_raw: Any = node.get("widgets_values")
            widgets_values: Optional[List[Any]] = None
            if isinstance(widgets_values_raw, list):
                widgets_values = cast(List[Any], widgets_values_raw)

            # 尝试从 prompt 中获取对应节点的数据
            prompt_data: Dict[str, Any] = self.prompt.get(node_id_str, {})
            class_type = prompt_data.get("class_type", "")
            inputs: Dict[str, Any] = prompt_data.get("inputs", {})

            node_info = NodeInfo(
                node_id=node_id_str,
                node_data=node,
                prompt_data=prompt_data,
                node_type=node_type,
                class_type=class_type,
                is_disabled=False,  # 已过滤 disabled 节点
                is_subgraph=False,
                subgraph_id=None,
                widgets_values=widgets_values,
                inputs=inputs,
            )

            self._nodes_cache[node_id_str] = node_info

            # 检查是否是种子节点
            if widgets_values:
                for idx in range(len(widgets_values) - 1):
                    val = widgets_values[idx]
                    val_next = widgets_values[idx + 1]
                    if isinstance(val, int) and val_next in [
                        "fixed",
                        "increment",
                        "decrement",
                        "randomize",
                    ]:
                        self._seed_nodes.append(node_info)
                        break

            # 检查是否是文件名节点（带日期模板）
            if widgets_values:
                for val in widgets_values:
                    if isinstance(val, str) and "%date:" in val:
                        self._date_filename_nodes.append(node_info)
                        break

        # 2. 收集各子图定义内部的节点
        subgraphs: List[Dict[str, Any]] = self.workflow.get("definitions", {}).get(
            "subgraphs", []
        )
        for subgraph in subgraphs:
            subgraph_id = subgraph.get("id")
            for node in subgraph.get("nodes", []):
                child_node_id_str = str(node.get("id"))
                if is_node_disabled(node):
                    continue

                node_type = node.get("type", "")
                widgets_values_raw: Any = node.get("widgets_values")
                widgets_values: Optional[List[Any]] = None
                if isinstance(widgets_values_raw, list):
                    widgets_values = cast(List[Any], widgets_values_raw)

                # 子图节点在 prompt 中可能不存在顶层 ID，而是通过父节点实例引用
                # 稍后在需要时通过父节点动态解析
                prompt_data: Dict[str, Any] = {}
                class_type = ""
                inputs: Dict[str, Any] = {}

                node_info = NodeInfo(
                    node_id=child_node_id_str,
                    node_data=node,
                    prompt_data=prompt_data,
                    node_type=node_type,
                    class_type=class_type,
                    is_disabled=False,
                    is_subgraph=True,
                    subgraph_id=subgraph_id,
                    widgets_values=widgets_values,
                    inputs=inputs,
                )

                self._nodes_cache[f"subgraph:{subgraph_id}:{child_node_id_str}"] = (
                    node_info
                )

                # 检查是否是种子节点
                if widgets_values:
                    for idx in range(len(widgets_values) - 1):
                        val = widgets_values[idx]
                        val_next = widgets_values[idx + 1]
                        if isinstance(val, int) and val_next in [
                            "fixed",
                            "increment",
                            "decrement",
                            "randomize",
                        ]:
                            self._seed_nodes.append(node_info)
                            break

                # 检查是否是文件名节点（带日期模板）
                if widgets_values:
                    for val in widgets_values:
                        if isinstance(val, str) and "%date:" in val:
                            self._date_filename_nodes.append(node_info)
                            break

        # 3. 构建 _meta.title → node_id 映射（遍历所有 prompt 节点，包括非 workflow 节点）
        for pid, pnode in self.prompt.items():
            title = pnode.get("_meta", {}).get("title")
            if title:
                self._title_to_node[title] = pid

    def find_nodes(self, **criteria: Any) -> List[NodeInfo]:
        """根据条件筛选节点"""
        result: List[NodeInfo] = []

        for node_info in self._nodes_cache.values():
            # 筛选条件
            if "node_type" in criteria and node_info.node_type != criteria["node_type"]:
                continue
            if (
                "class_type" in criteria
                and node_info.class_type != criteria["class_type"]
            ):
                continue
            if (
                "is_subgraph" in criteria
                and node_info.is_subgraph != criteria["is_subgraph"]
            ):
                continue
            if "node_id" in criteria and node_info.node_id != criteria["node_id"]:
                continue

            result.append(node_info)

        return result

    def find_nodes_by_class_type(self, class_type: str) -> List[NodeInfo]:
        """根据 class_type 筛选节点"""
        return self.find_nodes(class_type=class_type)

    def find_nodes_by_node_type(self, node_type: str) -> List[NodeInfo]:
        """根据 node_type 筛选节点"""
        return self.find_nodes(node_type=node_type)

    def get_node_by_id(self, node_id: str) -> Optional[NodeInfo]:
        """根据节点 ID 获取节点信息"""
        # 先尝试顶层节点
        if node_id in self._nodes_cache:
            return self._nodes_cache[node_id]

        # 尝试子图节点（格式: parent_id:child_id）
        if ":" in node_id:
            parent_id, child_id = node_id.split(":", 1)
            subgraph_cache_key = None
            for key in self._nodes_cache:
                if key.startswith(f"subgraph:") and key.endswith(f":{child_id}"):
                    # 检查父节点是否匹配
                    subgraph_info = self._nodes_cache[key]
                    if subgraph_info.subgraph_id:
                        # 查找父节点是否使用该子图
                        parent_info = self._nodes_cache.get(parent_id)
                        if (
                            parent_info
                            and parent_info.node_type == subgraph_info.subgraph_id
                        ):
                            subgraph_cache_key = key
                            break

            if subgraph_cache_key:
                return self._nodes_cache[subgraph_cache_key]

        return None

    # #region 种子和文件名更新方法
    def has_seeds_to_update(self) -> bool:
        """
        检查是否有种子需要更新。
        返回 True 如果至少有一个种子节点存在。
        """
        return len(self._seed_nodes) > 0

    def update_seeds(self) -> int:
        """
        修改 prompt (API 结构) 和 workflow (UI 结构) 中的随机种子值。
        使用缓存的种子节点信息，避免重复分析。
        返回成功修改的种子总数。
        """
        modified_count: int = 0

        # 遍历缓存的种子节点
        for node_info in self._seed_nodes:
            if node_info.is_disabled:
                continue

            widgets_values = node_info.widgets_values
            if not widgets_values:
                continue

            # 遍历 widgets_values 数组，寻找满足 [整数值, 'fixed'/'increment'/'decrement'/'randomize'] 邻接特征的项
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

                    # 计算新生成的种子值
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
                        # 对于子图节点，需根据其所有顶层实例生成前缀 ID 列表
                        for parent_info in self._nodes_cache.values():
                            if (
                                not parent_info.is_subgraph
                                and parent_info.node_type == node_info.subgraph_id
                            ):
                                if not parent_info.is_disabled:
                                    api_node_ids.append(
                                        f"{parent_info.node_id}:{node_info.node_id}"
                                    )

                    for api_node_id in api_node_ids:
                        # api_node_id 必须存在于 prompt 结构中
                        if api_node_id not in self.prompt:
                            raise ValueError(
                                f"Workflow seed node {api_node_id} is missing in the prompt API structure."
                            )

                        inputs: Dict[str, Any] = self.prompt[api_node_id].get(
                            "inputs", {}
                        )
                        # 遍历此节点在 prompt 中的所有 inputs，寻找和当前种子关联的端口并追溯修改其源头值
                        for ik in list(inputs.keys()):
                            src_node_id: str
                            src_key: str
                            src_node_id, src_key = find_terminal_input(
                                self.prompt, api_node_id, ik
                            )
                            # 追溯到的源头节点必须存在于 prompt 结构中
                            if src_node_id not in self.prompt:
                                raise ValueError(
                                    f"Source node {src_node_id} for seed tracking is missing in the prompt API structure."
                                )

                            src_node: Dict[str, Any] = self.prompt[src_node_id]
                            src_inputs: Dict[str, Any] = src_node.get("inputs", {})
                            current_val: Any = src_inputs.get(src_key)

                            # 校验当前值是否等于 old_seed 且满足种子标识或 Primitive 属性
                            is_primitive = (
                                src_node.get("class_type") in KNOWN_PRIMITIVE_TYPES
                            )
                            if (
                                current_val == old_seed
                                or str(current_val) == str(old_seed)
                            ) and ("seed" in ik or "seed" in src_key or is_primitive):
                                src_inputs[src_key] = new_seed

        return modified_count

    def update_output_filenames(self) -> None:
        """
        扫描 workflow 和 prompt 中的输出节点，如果发现使用了 %date:...% 占位符且在 prompt 中被写死为旧日期，
        将其更新为当前系统时间的日期静态值。
        使用缓存的文件名节点信息，避免重复分析。
        """
        if not self._date_filename_nodes:
            return

        # 汇总所有待处理的节点
        date_patterns: Dict[str, Tuple[str, str, bool, Optional[str]]] = {}

        for node_info in self._date_filename_nodes:
            if node_info.is_disabled:
                continue

            widgets_values = node_info.widgets_values
            if not widgets_values:
                continue

            # 寻找包含 "%date:" 的字符串 widget
            for val in widgets_values:
                if isinstance(val, str) and "%date:" in val:
                    match = re.search(r"%date:([^%]+)%", val)
                    if match:
                        comfy_fmt: str = match.group(1)
                        py_fmt, regex_pattern = (
                            self._convert_comfy_date_format_to_python(comfy_fmt)
                        )
                        date_patterns[node_info.node_id] = (
                            py_fmt,
                            regex_pattern,
                            node_info.is_subgraph,
                            node_info.subgraph_id,
                        )
                        break

        if not date_patterns:
            return

        now = datetime.datetime.now()

        for node_id_str, (
            py_fmt,
            regex_pattern,
            is_subgraph,
            subgraph_id,
        ) in date_patterns.items():
            # 映射到 API (prompt) 端节点 ID 列表
            api_node_ids: List[str] = []
            if not is_subgraph:
                api_node_ids = [node_id_str]
            else:
                for parent_info in self._nodes_cache.values():
                    if (
                        not parent_info.is_subgraph
                        and parent_info.node_type == subgraph_id
                    ):
                        if not parent_info.is_disabled:
                            api_node_ids.append(f"{parent_info.node_id}:{node_id_str}")

            for api_node_id in api_node_ids:
                # api_node_id 必须存在于 prompt 结构中
                if api_node_id not in self.prompt:
                    raise ValueError(
                        f"Workflow output node {api_node_id} is missing in the prompt API structure."
                    )

                inputs: Dict[str, Any] = self.prompt[api_node_id].get("inputs", {})
                filename_prefix: Any = inputs.get("filename_prefix")

                if filename_prefix:
                    # 追溯 filename_prefix 端口的值源头
                    src_node_id: str
                    src_key: str
                    src_node_id, src_key = find_terminal_input(
                        self.prompt, api_node_id, "filename_prefix"
                    )
                    # 追溯到的源头节点必须存在于 prompt 结构中
                    if src_node_id not in self.prompt:
                        raise ValueError(
                            f"Source node {src_node_id} for filename_prefix is missing in the prompt API structure."
                        )

                    src_inputs = self.prompt[src_node_id].setdefault("inputs", {})
                    prefix_val: Any = src_inputs.get(src_key)
                    if isinstance(prefix_val, str):
                        new_date_str: str = now.strftime(py_fmt)
                        if re.search(regex_pattern, prefix_val):
                            new_prefix: str = re.sub(
                                regex_pattern, new_date_str, prefix_val
                            )
                            src_inputs[src_key] = new_prefix

    def adjust_output_directory(self, rel_dir: str) -> None:
        """
        根据相对路径 rel_dir 调整所有输出节点的 filename_prefix。
        模板始终从终端节点的 workflow widget 读取（终端节点即 find_terminal_input 返回的节点），
        含 %...% 语法的按非日期变量和 rel_dir 分段一一对应更新源节点值并重建前缀，
        日期部分保留占位符（由节点在队列时自行求值）。
        模板变量数与分段数不匹配或变量源节点未找到时展平路径分隔符为 __ 并拼入 rel_dir。
        无模板的节点回退为旧的前缀追加行为。
        """
        for node_id, node in self.prompt.items():
            inputs = node.get("inputs", {})
            if "filename_prefix" not in inputs:
                continue

            src_node_id, src_key = find_terminal_input(
                self.prompt, node_id, "filename_prefix"
            )
            src_node = self.prompt.get(src_node_id)
            if not src_node:
                continue

            src_inputs = src_node.setdefault("inputs", {})
            original_val = src_inputs.get(src_key)
            if not isinstance(original_val, str):
                continue

            # 模板始终从终端节点的 workflow widget 读取
            terminal_info = self._nodes_cache.get(src_node_id)
            wf_template: Optional[str] = None
            if terminal_info and terminal_info.widgets_values:
                for val in terminal_info.widgets_values:
                    if isinstance(val, str) and "%" in val:
                        wf_template = val
                        break

            if wf_template:
                # 解析非日期模板变量
                non_date_vars: List[str] = []
                var_placeholders: List[str] = []
                for m in re.finditer(r"%([a-zA-Z_][a-zA-Z0-9_.]*?)%", wf_template):
                    var_name = m.group(1)
                    if not var_name.startswith("date:"):
                        non_date_vars.append(var_name)
                        var_placeholders.append(m.group(0))

                if non_date_vars:
                    rel_parts = [p for p in rel_dir.split("/") if p]

                    if len(rel_parts) == len(non_date_vars):
                        # 通过标题缓存一次查找全部，全部找到才统一更新
                        updates: List[Tuple[str, str, str]] = []
                        for i, var_name in enumerate(non_date_vars):
                            node_title = var_name.rsplit(".", 1)[0]
                            pid = self._title_to_node.get(node_title)
                            if pid is None:
                                updates.clear()
                                break
                            input_key = (
                                var_name.rsplit(".", 1)[1]
                                if "." in var_name
                                else "value"
                            )
                            updates.append((pid, input_key, rel_parts[i]))

                        if updates:
                            for pid, input_key, val in updates:
                                self.prompt[pid].setdefault("inputs", {})[
                                    input_key
                                ] = val
                            # 重建 prompt filename_prefix
                            # 保留已解析的日期部分，替换非日期变量为 rel_dir 对应段
                            new_prefix = wf_template
                            for m in re.finditer(r"%date:([^%]+)%", new_prefix):
                                comfy_fmt = m.group(1)
                                _, date_regex = (
                                    self._convert_comfy_date_format_to_python(
                                        comfy_fmt
                                    )
                                )
                                date_m = re.search(date_regex, original_val)
                                if date_m:
                                    new_prefix = new_prefix.replace(
                                        m.group(0), date_m.group(0), 1
                                    )
                            for i, placeholder in enumerate(var_placeholders):
                                if i < len(rel_parts):
                                    new_prefix = new_prefix.replace(
                                        placeholder, rel_parts[i], 1
                                    )

                            src_inputs[src_key] = new_prefix
                            continue

                    # 非日期变量数与 rel_dir 分段数不匹配或变量源节点未找到
                    # 展平路径分隔符并拼入 rel_dir，使输出在目标目录而不是其子目录
                    flat_val = original_val.replace("/", "__").replace("\\", "__")
                    new_val = (
                        f"{rel_dir}/{flat_val}"
                        if rel_dir and rel_dir != "."
                        else flat_val
                    )
                    src_inputs[src_key] = new_val

                    # 同步修改终端节点的 workflow widget
                    if terminal_info and terminal_info.widgets_values:
                        for idx, val in enumerate(terminal_info.widgets_values):
                            if isinstance(val, str):
                                flat_wf = val.replace("/", "__").replace("\\", "__")
                                terminal_info.widgets_values[idx] = (
                                    f"{rel_dir}/{flat_wf}"
                                    if rel_dir and rel_dir != "."
                                    else flat_wf
                                )
                    continue

            # 回退：旧的目录前缀追加行为
            original_basename = os.path.basename(original_val)
            new_val = (
                f"{rel_dir}/{original_basename}"
                if rel_dir and rel_dir != "."
                else original_basename
            )
            src_inputs[src_key] = new_val

            # 同步修改 workflow 结构
            wf_src_info = self._nodes_cache.get(src_node_id)
            if wf_src_info and wf_src_info.widgets_values:
                for idx, val in enumerate(wf_src_info.widgets_values):
                    if isinstance(val, str):
                        parts = re.split(r"[\\/]", val)
                        basename = parts[-1] if parts else val
                        new_wf_val = (
                            f"{rel_dir}/{basename}"
                            if rel_dir and rel_dir != "."
                            else basename
                        )
                        wf_src_info.widgets_values[idx] = new_wf_val

    def _convert_comfy_date_format_to_python(self, comfy_fmt: str) -> Tuple[str, str]:
        """
        将 ComfyUI 的日期占位符格式 (如 yyyyMMdd_hhmmss) 转换为 Python datetime 格式和对应的正则表达式。
        """
        replacements = [
            ("yyyy", "%Y", r"\d{4}"),
            ("yy", "%y", r"\d{2}"),
            ("MM", "%m", r"\d{2}"),
            ("dd", "%d", r"\d{2}"),
            ("hh", "%H", r"\d{2}"),
            ("mm", "%M", r"\d{2}"),
            ("ss", "%S", r"\d{2}"),
        ]
        py_fmt = comfy_fmt
        regex_pattern = comfy_fmt
        for comfy_key, py_val, regex_val in replacements:
            py_fmt = py_fmt.replace(comfy_key, py_val)
            regex_pattern = regex_pattern.replace(comfy_key, regex_val)
        return py_fmt, regex_pattern

    # #endregion

    # #region 权重调整方法
    def get_current_lora_weight(self, lora_name_query: str) -> Optional[float]:
        """
        在 prompt 和 workflow 中查找匹配的 Lora 节点，返回其当前权重。
        支持原生 LoraLoader 和 Power Lora Loader (rgthree)。
        """
        query_lower = lora_name_query.lower()

        # 1. 从 prompt (API 结构) 中查找
        for nid, node in self.prompt.items():
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            if class_type == "LoraLoader":
                inputs = node_dict.get("inputs", {})
                lora_name = inputs.get("lora_name", "")
                if isinstance(lora_name, str) and query_lower in lora_name.lower():
                    for ik in ["strength_model", "strength_clip"]:
                        if ik in inputs:
                            src_nid, src_key = find_terminal_input(self.prompt, nid, ik)
                            val = self.prompt[src_nid]["inputs"].get(src_key)
                            if isinstance(val, (int, float)):
                                return float(val)
            elif class_type == "Power Lora Loader (rgthree)":
                inputs = node_dict.get("inputs", {})
                for k, v in inputs.items():
                    if k.startswith("lora_") and isinstance(v, dict):
                        v_dict = cast(Dict[str, Any], v)
                        lora_path = v_dict.get("lora", "")
                        if (
                            isinstance(lora_path, str)
                            and query_lower in lora_path.lower()
                        ):
                            strength = v_dict.get("strength")
                            if isinstance(strength, (int, float)):
                                return float(strength)

        # 2. 从 workflow (UI 结构) 中查找
        for node in self.workflow.get("nodes", []):
            if is_node_disabled(node):
                continue
            node_type = node.get("type", "")
            widgets_values = node.get("widgets_values")
            if not isinstance(widgets_values, list):
                continue
            wv = cast(List[Any], widgets_values)

            if node_type == "LoraLoader":
                if wv and isinstance(wv[0], str) and query_lower in wv[0].lower():
                    if len(wv) > 1 and isinstance(wv[1], (int, float)):
                        return float(wv[1])
            elif node_type == "Power Lora Loader (rgthree)":
                for val in wv:
                    if isinstance(val, dict) and "lora" in val:
                        val_dict = cast(Dict[str, Any], val)
                        if query_lower in str(val_dict.get("lora", "")).lower():
                            strength = val_dict.get("strength")
                            if isinstance(strength, (int, float)):
                                return float(strength)

        return None

    def modify_lora_weights(self, lora_name_query: str, weight: float) -> None:
        """
        修改 Lora 权重，直接修改原对象。
        """
        query_lower = lora_name_query.lower()

        # 修改 prompt 中的 LoraLoader
        for nid, node in self.prompt.items():
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            if class_type == "LoraLoader":
                inputs = node_dict.get("inputs", {})
                lora_name = inputs.get("lora_name", "")
                if isinstance(lora_name, str) and query_lower in lora_name.lower():
                    for ik in ["strength_model", "strength_clip"]:
                        if ik in inputs:
                            src_nid, src_key = find_terminal_input(self.prompt, nid, ik)
                            self.prompt[src_nid]["inputs"][src_key] = weight
                            # 同步更新 workflow 中对应 Primitive 节点的 widgets_values
                            if src_nid != nid:
                                wf_node = self.get_node_by_id(src_nid)
                                if wf_node and wf_node.widgets_values:
                                    wf_node.widgets_values[0] = weight
            elif class_type == "Power Lora Loader (rgthree)":
                inputs = node_dict.get("inputs", {})
                for k, v in list(inputs.items()):
                    if k.startswith("lora_") and isinstance(v, dict):
                        v_dict = cast(Dict[str, Any], v)
                        lora_path = v_dict.get("lora", "")
                        if (
                            isinstance(lora_path, str)
                            and query_lower in lora_path.lower()
                        ):
                            v_dict["strength"] = weight

        # 修改 workflow 中的节点
        for node_info in self._nodes_cache.values():
            if node_info.is_disabled:
                continue

            if node_info.node_type == "LoraLoader":
                wv = node_info.widgets_values
                if wv and isinstance(wv[0], str) and query_lower in wv[0].lower():
                    # strength_model 和 strength_clip 通常在 index 1 和 2
                    if len(wv) > 1 and isinstance(wv[1], (int, float)):
                        wv[1] = weight
                    if len(wv) > 2 and isinstance(wv[2], (int, float)):
                        wv[2] = weight

            elif node_info.node_type == "Power Lora Loader (rgthree)":
                wv = node_info.widgets_values
                if wv:
                    for val in wv:
                        if isinstance(val, dict) and "lora" in val:
                            val_dict = cast(Dict[str, Any], val)
                            lora_path = val_dict.get("lora", "")
                            if (
                                isinstance(lora_path, str)
                                and query_lower in lora_path.lower()
                            ):
                                val_dict["strength"] = weight

    def get_current_cfg_weight(
        self, node_ids: Optional[List[str]] = None
    ) -> Optional[float]:
        """
        在 prompt 中查找 KSampler 节点的当前 CFG 值。
        """
        for nid, node in self.prompt.items():
            if node_ids is not None and nid not in node_ids:
                continue
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            if "KSampler" in class_type:
                inputs = node_dict.get("inputs", {})
                if "cfg" in inputs:
                    src_nid, src_key = find_terminal_input(self.prompt, nid, "cfg")
                    val = self.prompt[src_nid]["inputs"].get(src_key)
                    if isinstance(val, (int, float)):
                        return float(val)

        return None

    def modify_cfg_weights(
        self, weight: float, node_ids: Optional[List[str]] = None
    ) -> None:
        """
        修改 KSampler 的 CFG 权重，直接修改原对象。
        """
        # 修改 prompt 中的 KSampler CFG
        cfg_sources: List[Tuple[str, str]] = []

        for nid, node in self.prompt.items():
            if node_ids is not None and nid not in node_ids:
                continue
            node_dict = cast(Dict[str, Any], node)
            class_type = node_dict.get("class_type", "")
            if "KSampler" in class_type:
                inputs = node_dict.get("inputs", {})
                if "cfg" in inputs:
                    src_nid, src_key = find_terminal_input(self.prompt, nid, "cfg")
                    cfg_sources.append((src_nid, src_key))

        for src_nid, src_key in cfg_sources:
            if src_nid in self.prompt:
                self.prompt[src_nid]["inputs"][src_key] = weight

        # 修改 workflow 中的 KSampler widget 和 Primitive 节点
        for node_info in self._nodes_cache.values():
            if node_ids is not None and node_info.node_id not in node_ids:
                continue
            if node_info.is_disabled:
                continue

            if "KSampler" in node_info.node_type:
                wv = node_info.widgets_values
                if wv:
                    # KSampler 的 CFG 通常在 index 3，KSamplerAdvanced 在 index 4
                    if node_info.node_type == "KSampler" and len(wv) >= 4:
                        if isinstance(wv[3], (int, float)):
                            wv[3] = weight
                    elif node_info.node_type == "KSamplerAdvanced" and len(wv) >= 5:
                        if isinstance(wv[4], (int, float)):
                            wv[4] = weight

            # 更新 Primitive 节点（CFG 的源头）
            elif node_info.class_type in KNOWN_PRIMITIVE_TYPES:
                # 检查这个 Primitive 是否是某个 KSampler CFG 的源头
                for src_nid, src_key in cfg_sources:
                    if src_nid == node_info.node_id:
                        wv = node_info.widgets_values
                        if wv and isinstance(wv[0], (int, float)):
                            wv[0] = weight

    def get_current_prompt_weight(
        self, target_nodes: List[Tuple[str, str, str, bool]], target_prompt: str
    ) -> Optional[float]:
        """
        在目标节点的文本中查找匹配的提示词，返回其当前权重。
        支持 (word:weight)、(word) 和裸词格式。未带权重视为 1.0。
        """
        escaped = re.escape(target_prompt)

        for node_id, _, _, _ in target_nodes:
            node_info = self.get_node_by_id(node_id)
            if node_info is None:
                continue

            # 从 workflow 文本中查找（需要获取节点的文本 widget）
            workflow_text = self._get_workflow_node_text(node_id)
            if workflow_text is None:
                continue

            # 匹配带权重的格式: (word:1.2)
            pattern_with_weight = re.compile(
                rf"\(\s*{escaped}\s*:\s*([0-9.-]+)\s*\)", re.IGNORECASE
            )
            m = pattern_with_weight.search(workflow_text)
            if m:
                return float(m.group(1))

            # 匹配带括号无权重的格式: (word) → 默认权重 1.0
            pattern_brackets = re.compile(rf"\(\s*{escaped}\s*\)", re.IGNORECASE)
            if pattern_brackets.search(workflow_text):
                return 1.0

            # 匹配裸词 → 默认权重 1.0
            # 不能用 \b 因为目标文本可能以非单词字符结尾（如 `\(text\)`），会导致边界断言失败
            pattern_bare = re.compile(rf"(?<!\w){escaped}(?!\w)", re.IGNORECASE)
            if pattern_bare.search(workflow_text):
                return 1.0

        return None

    def _get_workflow_node_text(self, node_id: str) -> Optional[str]:
        """
        在 UI 结构 workflow 中获取特定节点的文本 widget 数值。
        """
        node_info = self.get_node_by_id(node_id)
        if node_info is None:
            return None

        widgets_values = node_info.widgets_values
        if widgets_values and isinstance(widgets_values[0], str):
            return widgets_values[0]

        return None

    def modify_prompt_weights(
        self,
        target_nodes: List[Tuple[str, str, str, bool]],
        target_prompt: str,
        weight: float,
        skip_add: bool,
    ) -> None:
        """
        在 CLIPTextEncode 节点中调整提示词的权重（原地修改）。
        如果不存在，且未指定 skip_add 选项，则将其添加到第一个有效节点上。
        """
        any_existing_modified = False

        # 1. 首先尝试在所有候选目标节点中修改已存在的提示词
        for node_id, start_marker, end_marker, use_markers in target_nodes:
            workflow_text = self._get_workflow_node_text(node_id)
            if workflow_text is None:
                continue

            node_info = self.get_node_by_id(node_id)
            if node_info is None:
                continue

            # 从 prompt 中获取文本
            prompt_text = (
                self.prompt[node_id].setdefault("inputs", {}).setdefault("text", "")
            )
            if not isinstance(prompt_text, str):
                prompt_text = ""

            # 调整权重
            new_workflow_text, mod_wf = self._adjust_prompt_weight_in_text(
                workflow_text, target_prompt, weight
            )
            new_prompt_text, mod_pr = self._adjust_prompt_weight_in_text(
                prompt_text, target_prompt, weight
            )

            if mod_wf or mod_pr:
                self.prompt[node_id]["inputs"]["text"] = (
                    self._strip_comments_for_prompt(new_prompt_text)
                )
                self._update_workflow_node_text(node_id, new_workflow_text)
                any_existing_modified = True

        # 2. 如果没有任何一个节点含有该提示词，且允许添加，则在第一个节点上添加
        if not any_existing_modified and not skip_add:
            if target_nodes:
                node_id, start_marker, end_marker, use_markers = target_nodes[0]
                workflow_text = self._get_workflow_node_text(node_id)
                if workflow_text is not None:
                    added_text = f"({target_prompt}:{weight})"
                    # 添加到 workflow 文本和 prompt 文本
                    self._add_prompt_to_node(
                        node_id, added_text, start_marker, end_marker, use_markers
                    )

    def _adjust_prompt_weight_in_text(
        self, text: str, target_prompt: str, new_weight: float
    ) -> Tuple[str, bool]:
        """
        在文本中找到并更新特定提示词的权重为 new_weight。
        支持匹配格式：(word:weight)、(word) 以及裸词 word。
        返回 (new_text, is_modified)。
        """
        escaped_target = re.escape(target_prompt)

        # 1. 优先匹配已带权重的格式, 如 (beautiful scenery:1.2) 或 (beautiful scenery:-0.5)
        pattern_with_weight = re.compile(
            rf"\(\s*{escaped_target}\s*:\s*[0-9.-]+\s*\)", re.IGNORECASE
        )
        if pattern_with_weight.search(text):
            new_text = pattern_with_weight.sub(f"({target_prompt}:{new_weight})", text)
            return new_text, True

        # 2. 匹配带括号但无权重的格式, 如 (beautiful scenery)
        pattern_with_brackets = re.compile(
            rf"\(\s*{escaped_target}\s*\)", re.IGNORECASE
        )
        if pattern_with_brackets.search(text):
            new_text = pattern_with_brackets.sub(
                f"({target_prompt}:{new_weight})", text
            )
            return new_text, True

        # 3. 匹配裸词，两边使用负向环视以防匹配子串 (caterpillar vs cat)
        # 不能用 \b 因为目标文本可能以非单词字符结尾（如 `\(text\)`），会导致边界断言失败
        pattern_bare = re.compile(rf"(?<!\w){escaped_target}(?!\w)", re.IGNORECASE)
        if pattern_bare.search(text):
            new_text = pattern_bare.sub(f"({target_prompt}:{new_weight})", text)
            return new_text, True

        return text, False

    def _update_workflow_node_text(self, node_id: str, new_text: str) -> None:
        """
        在 UI 结构 workflow 中同步更新特定节点的文本 widget 数值。
        """
        node_info = self.get_node_by_id(node_id)
        if node_info is None:
            return

        widgets_values = node_info.widgets_values
        if widgets_values and len(widgets_values) > 0:
            widgets_values[0] = new_text

    def _strip_comments_for_prompt(self, text: str) -> str:
        """
        为 prompt 剥离注释行。如果一行去除首尾空白后以 '//' 开头，该整行将被完全过滤掉。
        """
        lines: List[str] = []
        for line in text.splitlines():
            if line.strip().startswith("//"):
                continue
            lines.append(line)
        return "\n".join(lines)

    def _add_prompt_to_node(
        self,
        node_id: str,
        added_text: str,
        start_marker: str,
        end_marker: str,
        use_markers: bool,
    ) -> None:
        """
        将提示词添加到节点。
        """
        workflow_text = self._get_workflow_node_text(node_id)
        if workflow_text is None:
            return

        prompt_text = (
            self.prompt[node_id].setdefault("inputs", {}).setdefault("text", "")
        )
        if not isinstance(prompt_text, str):
            prompt_text = ""

        start_idx = -1
        end_idx = -1
        if use_markers:
            start_idx = workflow_text.find(start_marker)
            end_idx = workflow_text.find(end_marker)

        has_marker = start_idx != -1 and end_idx != -1 and start_idx < end_idx

        if has_marker:
            before_marker = workflow_text[:start_idx]
            marker_content = workflow_text[start_idx + len(start_marker) : end_idx]
            after_marker = workflow_text[end_idx + len(end_marker) :]

            stripped = marker_content.strip()
            if stripped:
                if not stripped.endswith(","):
                    stripped += ","
                new_content_prompt = f"{stripped}\n{added_text},"
            else:
                new_content_prompt = f"{added_text},"

            new_workflow_text = (
                before_marker.rstrip()
                + f"\n{start_marker}\n"
                + new_content_prompt
                + f"\n{end_marker}\n"
                + after_marker.lstrip()
            )
            new_prompt_text = prompt_text.rstrip()
            if new_prompt_text and not new_prompt_text.endswith(","):
                new_prompt_text += ","
            new_prompt_text += f"\n{added_text},"
        else:
            new_workflow_text = workflow_text.rstrip()
            if new_workflow_text and not new_workflow_text.endswith(","):
                new_workflow_text += ","
            new_workflow_text += f"\n{added_text},"

            new_prompt_text = prompt_text.rstrip()
            if new_prompt_text and not new_prompt_text.endswith(","):
                new_prompt_text += ","
            new_prompt_text += f"\n{added_text},"

        self.prompt[node_id]["inputs"]["text"] = self._strip_comments_for_prompt(
            new_prompt_text
        )
        self._update_workflow_node_text(node_id, new_workflow_text)

    # #endregion

    # #region 双轨道文本处理（add/remove 提示词）
    def process_double_track(
        self,
        node_id: str,
        command: str,
        prompt_str_arg: str,
        start_marker: str,
        end_marker: str,
        raw: bool,
        no_skip: bool,
        hard: bool,
        use_markers: bool = True,
    ) -> bool:
        """
        对指定节点执行 add/remove 操作，原地修改 prompt 和 workflow 文本。
        返回 True 表示执行了操作，False 表示跳过。
        """
        workflow_text = self._get_workflow_node_text(node_id)
        if workflow_text is None:
            return False
        prompt_text = self.prompt[node_id].get("inputs", {}).get("text", "")
        if not isinstance(prompt_text, str):
            prompt_text = ""

        # 经过清除逻辑得到的 prompt 文本，用于做等价性比对
        stripped_workflow = self._strip_comments_for_prompt(workflow_text)
        workflow_cleaned = "\n".join(
            [line.strip() for line in stripped_workflow.splitlines() if line.strip()]
        )
        prompt_cleaned = "\n".join(
            [line.strip() for line in prompt_text.splitlines() if line.strip()]
        )
        is_equivalent = workflow_cleaned == prompt_cleaned

        # 在 workflow 文本中定位 marker 区域
        start_idx = -1
        end_idx = -1
        if use_markers:
            start_idx = workflow_text.find(start_marker)
            end_idx = workflow_text.find(end_marker)
            has_marker = start_idx != -1 and end_idx != -1 and start_idx < end_idx
        else:
            has_marker = False

        # 校验提示词是否已存在
        def contains_prompt(area: str) -> bool:
            target_lower = prompt_str_arg.strip().lower()
            if raw:
                return target_lower in area.lower()
            for line in area.splitlines():
                if line.strip().startswith("//"):
                    continue
                if target_lower in line.lower():
                    return True
            return False

        new_workflow_text = None
        new_prompt_text = None

        if command == "add":
            if has_marker:
                before_marker = workflow_text[:start_idx]
                marker_content = workflow_text[start_idx + len(start_marker) : end_idx]
                after_marker = workflow_text[end_idx + len(end_marker) :]

                if contains_prompt(marker_content) and not no_skip:
                    _LOGGER.debug(
                        "Prompt '%s' already exists in marker region, skipping.",
                        prompt_str_arg,
                    )
                    return False

                # 计算不带 marker 的内部拼接新文本，按行追加
                stripped = marker_content.strip()
                if stripped:
                    if not stripped.endswith(","):
                        stripped += ","
                    new_content_prompt = f"{stripped}\n{prompt_str_arg},"
                else:
                    new_content_prompt = f"{prompt_str_arg},"

                # 双轨道组装：分别组装带 marker 的 workflow 文本，和不带 marker 的 prompt 文本
                new_workflow_text = (
                    before_marker.rstrip()
                    + f"\n{start_marker}\n"
                    + new_content_prompt
                    + f"\n{end_marker}\n"
                    + after_marker.lstrip()
                )

                # 判断等价性以决定是否采用回退防御机制
                if is_equivalent:
                    new_prompt_text_raw = (
                        before_marker.rstrip()
                        + "\n"
                        + new_content_prompt
                        + "\n"
                        + after_marker.lstrip()
                    )
                    new_prompt_text = self._strip_comments_for_prompt(
                        new_prompt_text_raw
                    )
                else:
                    # 回退：基于区域内容在 prompt 中做精确匹配
                    target_match_content = self._strip_comments_for_prompt(
                        marker_content
                    ).strip()
                    if target_match_content and target_match_content in prompt_text:
                        new_prompt_text = prompt_text.replace(
                            target_match_content, new_content_prompt.strip(), 1
                        )
                    else:
                        # 匹配不到加在尾部
                        if raw:
                            new_prompt_text = (
                                prompt_text.rstrip() + "\n" + prompt_str_arg
                            )
                        else:
                            new_prompt_text = (
                                prompt_text.rstrip() + f"\n{prompt_str_arg},"
                            )
            else:
                if contains_prompt(workflow_text) and not no_skip:
                    _LOGGER.debug(
                        "Prompt '%s' already exists in text, skipping.",
                        prompt_str_arg,
                    )
                    return False

                if raw:
                    new_content_prompt = prompt_str_arg
                else:
                    new_content_prompt = f"{prompt_str_arg},"

                # 第一次添加，双轨道组装：workflow 附加带 marker 区域（如果 use_markers），prompt 附加不带 marker 区域
                if use_markers:
                    new_workflow_text = (
                        workflow_text.rstrip()
                        + f"\n{start_marker}\n"
                        + new_content_prompt
                        + f"\n{end_marker}\n"
                    )
                else:
                    new_workflow_text = (
                        workflow_text.rstrip() + "\n" + new_content_prompt
                    )
                # 对于无 marker，直接拼在尾部即可
                new_prompt_text = prompt_text.rstrip() + "\n" + new_content_prompt

        else:  # remove Command
            effective_hard = hard or raw
            if has_marker:
                before_marker = workflow_text[:start_idx]
                marker_content = workflow_text[start_idx + len(start_marker) : end_idx]
                after_marker = workflow_text[end_idx + len(end_marker) :]

                if not contains_prompt(marker_content):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in marker region (has_marker=True)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found in marker region, skipping.",
                            prompt_str_arg,
                        )
                        return False
                    new_content_prompt = marker_content
                else:
                    if raw:
                        new_content_prompt = marker_content.replace(prompt_str_arg, "")
                    else:
                        target_lower = prompt_str_arg.strip().lower()
                        lines = marker_content.split("\n")
                        new_lines: List[str] = []
                        for line in lines:
                            if target_lower in line.lower():
                                if effective_hard:
                                    pass
                                else:
                                    stripped = line.strip()
                                    if stripped.startswith("//"):
                                        new_lines.append(line)
                                    else:
                                        indent = line[: len(line) - len(line.lstrip())]
                                        new_lines.append(f"{indent}// {line.lstrip()}")
                            else:
                                new_lines.append(line)
                        new_content_prompt = "\n".join(new_lines)

                # 双轨道组装
                new_workflow_text = (
                    before_marker.rstrip()
                    + f"\n{start_marker}\n"
                    + new_content_prompt
                    + f"\n{end_marker}\n"
                    + after_marker.lstrip()
                )

                if is_equivalent:
                    new_prompt_text_raw = (
                        before_marker.rstrip()
                        + "\n"
                        + new_content_prompt
                        + "\n"
                        + after_marker.lstrip()
                    )
                    new_prompt_text = self._strip_comments_for_prompt(
                        new_prompt_text_raw
                    )
                else:
                    target_match_content = self._strip_comments_for_prompt(
                        marker_content
                    ).strip()
                    if target_match_content and target_match_content in prompt_text:
                        new_prompt_text = prompt_text.replace(
                            target_match_content, new_content_prompt.strip(), 1
                        )
                    else:
                        new_prompt_text = prompt_text
            else:
                if not contains_prompt(workflow_text):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in full text (has_marker=False)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found, skipping.", prompt_str_arg
                        )
                        return False
                    new_content_prompt = workflow_text
                else:
                    if raw:
                        new_content_prompt = workflow_text.replace(prompt_str_arg, "")
                    else:
                        target_lower = prompt_str_arg.strip().lower()
                        lines = workflow_text.split("\n")
                        new_lines = []
                        for line in lines:
                            if target_lower in line.lower():
                                if effective_hard:
                                    pass
                                else:
                                    stripped = line.strip()
                                    if stripped.startswith("//"):
                                        new_lines.append(line)
                                    else:
                                        indent = line[: len(line) - len(line.lstrip())]
                                        new_lines.append(f"{indent}// {line.lstrip()}")
                            else:
                                new_lines.append(line)
                        new_content_prompt = "\n".join(new_lines)

                new_workflow_text = new_content_prompt
                new_prompt_text = new_content_prompt

        new_prompt_text = self._strip_comments_for_prompt(new_prompt_text)

        self.prompt[node_id]["inputs"]["text"] = new_prompt_text
        self._update_workflow_node_text(node_id, new_workflow_text)
        return True

    # #endregion

    # #region 生成器方法（原地修改，每次 yield 自身）
    def generate_cfg_variants(
        self, weight_expr: str, node_ids: Optional[List[str]] = None
    ) -> Generator[None, None, None]:
        """
        为每个 CFG 权重变体原地修改并 yield。
        如果未找到 KSampler 节点或相对权重无法解析，不 yield 任何内容。
        """
        # 找到所有 KSampler 节点及其当前 CFG 值
        ksampler_cfgs: Dict[str, float] = {}
        for nid, node in self.prompt.items():
            if node_ids is not None and nid not in node_ids:
                continue
            class_type = node.get("class_type", "")
            if "KSampler" in class_type:
                inputs = node.get("inputs", {})
                if "cfg" in inputs:
                    src_nid, src_key = find_terminal_input(self.prompt, nid, "cfg")
                    val = self.prompt[src_nid]["inputs"].get(src_key)
                    if isinstance(val, (int, float)):
                        ksampler_cfgs[nid] = float(val)

        if not ksampler_cfgs:
            return

        # 为每个节点独立解析权重表达式
        node_weights_map: Dict[str, List[float]] = {}
        for nid, cfg_val in ksampler_cfgs.items():
            node_weights_map[nid] = parse_weights(weight_expr, cfg_val)

        # 检查版本数一致性
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
                self.modify_cfg_weights(node_weights_map[nid][vi], [nid])
            yield

    def generate_lora_variants(
        self, lora_name_query: str, weight_expr: str
    ) -> Generator[None, None, None]:
        """
        为每个 Lora 权重变体原地修改并 yield。
        如果相对权重无法解析当前值，不 yield 任何内容。
        """
        current = (
            self.get_current_lora_weight(lora_name_query)
            if is_relative(weight_expr)
            else None
        )
        if is_relative(weight_expr) and current is None:
            return
        weights = parse_weights(weight_expr, current)

        for w in weights:
            self.modify_lora_weights(lora_name_query, w)
            yield

    def generate_prompt_variants(
        self,
        target_nodes: List[Tuple[str, str, str, bool]],
        target_prompt: str,
        weight_expr: str,
        skip_add: bool,
    ) -> Generator[None, None, None]:
        """
        为每个提示词权重变体原地修改并 yield。
        如果相对权重无法解析当前值，或 skip_add 时提示词不存在，不 yield 任何内容。
        """
        current = (
            self.get_current_prompt_weight(target_nodes, target_prompt)
            if is_relative(weight_expr)
            else None
        )
        if is_relative(weight_expr) and current is None:
            return
        if (
            not is_relative(weight_expr)
            and skip_add
            and self.get_current_prompt_weight(target_nodes, target_prompt) is None
        ):
            return
        weights = parse_weights(weight_expr, current)

        for w in weights:
            self.modify_prompt_weights(target_nodes, target_prompt, w, skip_add)
            yield

    def generate_aspect_variants(
        self, ratio_expr: str, node_ids: Optional[List[str]] = None
    ) -> Generator[None, None, None]:
        """
        为每个长宽比变体原地修改并 yield。
        """
        # 1. 寻找所有含有 width 和 height inputs 的非 disabled 节点
        latent_nodes: List[str] = []
        for nid, node_info in self._nodes_cache.items():
            if node_ids is not None and nid not in node_ids:
                continue
            if "width" in node_info.inputs and "height" in node_info.inputs:
                latent_nodes.append(nid)

        if not latent_nodes:
            return

        # 2. 为每个节点计算目标宽高变体列表
        node_variants_map: Dict[str, List[Tuple[int, int]]] = {}
        for nid in latent_nodes:
            w_nid, w_key = find_terminal_input(self.prompt, nid, "width")
            h_nid, h_key = find_terminal_input(self.prompt, nid, "height")
            w_val = self.prompt[w_nid]["inputs"].get(w_key)
            h_val = self.prompt[h_nid]["inputs"].get(h_key)

            if not isinstance(w_val, (int, float)) or not isinstance(
                h_val, (int, float)
            ):
                continue

            W = float(w_val)
            H = float(h_val)
            S = W * H
            R_curr = W / H

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

            # 寻找与当前长宽比最接近的预设比例索引
            diffs = [abs(v - R_curr) for v in COMMON_VALUES]
            curr_idx = diffs.index(min(diffs))

            target_ratios: List[float] = []
            ratio_expr_clean = ratio_expr.strip()

            # 交换宽高模式
            if ratio_expr_clean.lower() in ("swap", "exchange"):
                node_variants_map[nid] = [(int(round(H)), int(round(W)))]
                continue

            # 解析对称浮动模式，支持 "w+-2:2" 这种形式以及默认 w 前缀，支持步长指定
            # 对称语法正则：^(?P<prefix>[wh]?)\+-(?P<delta>\d+)(?::(?P<step>\d+))?$
            import re

            m_sym = re.match(r"^([wh]?)\+-(\d+)(?::(\d+))?$", ratio_expr_clean)
            if m_sym:
                prefix = m_sym.group(1) or "w"
                delta = int(m_sym.group(2))
                step = int(m_sym.group(3)) if m_sym.group(3) else 1

                # 对称浮动在比例索引上计算变体
                start_idx = max(0, curr_idx - delta)
                end_idx = min(len(COMMON_RATIOS) - 1, curr_idx + delta)

                # 以 step 为步长生成索引序列，并确保 curr_idx 被包含或基于其对称分布
                indices: List[int] = []
                for offset in range(0, delta + 1, step):
                    l_idx = curr_idx - offset
                    if l_idx >= start_idx:
                        indices.append(l_idx)
                    r_idx = curr_idx + offset
                    if r_idx <= end_idx:
                        indices.append(r_idx)

                # 排序并去重
                indices = sorted(list(set(indices)))
                target_ratios = [COMMON_VALUES[idx] for idx in indices]

            # 解析升降档语法，如 "+1", "w-2", "h+1" 等
            # 正则：^(?P<prefix>[wh]?)(?P<shift>[+-]\d+)$
            else:
                m_shift = re.match(r"^([wh]?)([+-]\d+)$", ratio_expr_clean)
                if m_shift:
                    prefix = m_shift.group(1) or "w"
                    shift = int(m_shift.group(2))

                    # 如果前缀为 h，由于高度增加代表长宽比减小，因此索引变化方向取反
                    effective_shift = -shift if prefix == "h" else shift
                    target_idx = max(
                        0, min(len(COMMON_RATIOS) - 1, curr_idx + effective_shift)
                    )
                    target_ratios = [COMMON_VALUES[target_idx]]

                # 解析直接指定比例模式，如 "16:9"
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

            # 根据目标比例计算宽高，四舍五入到最接近的 8 的倍数以符合 SD 神经网络要求
            variants: List[Tuple[int, int]] = []
            for R in target_ratios:
                import math

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

        # 验证所有节点的变体数量相同，以确保可以同步修改
        lengths = {nid: len(vts) for nid, vts in node_variants_map.items()}
        unique_lengths = set(lengths.values())
        if len(unique_lengths) > 1:
            details = ", ".join(
                f"node {nid}: {l} versions" for nid, l in lengths.items()
            )
            raise ValueError(
                f"Inconsistent aspect ratio version counts generated for latent nodes ({details}) "
                f"under expression '{ratio_expr}'."
            )

        first_variants = list(node_variants_map.values())[0]
        for vi in range(len(first_variants)):
            for nid, variants in node_variants_map.items():
                w_target, h_target = variants[vi]
                self.modify_aspect_ratio(w_target, h_target, [nid])
            yield

    def modify_aspect_ratio(
        self,
        target_width: int,
        target_height: int,
        node_ids: Optional[List[str]] = None,
    ) -> None:
        """
        修改指定包含 width 和 height 的节点的长宽比，直接修改原对象。
        """
        sources: List[Tuple[str, Tuple[str, str], Tuple[str, str]]] = []
        for nid, node_info in self._nodes_cache.items():
            if node_ids is not None and nid not in node_ids:
                continue
            if "width" in node_info.inputs and "height" in node_info.inputs:
                w_nid, w_key = find_terminal_input(self.prompt, nid, "width")
                h_nid, h_key = find_terminal_input(self.prompt, nid, "height")
                sources.append((nid, (w_nid, w_key), (h_nid, h_key)))

        # 更新 prompt 中的输入源头值
        for nid, (w_nid, w_key), (h_nid, h_key) in sources:
            if w_nid in self.prompt:
                self.prompt[w_nid]["inputs"][w_key] = target_width
            if h_nid in self.prompt:
                self.prompt[h_nid]["inputs"][h_key] = target_height

        # 更新 workflow 结构中的对应 widgets_values
        for nid, (w_nid, w_key), (h_nid, h_key) in sources:
            node_info = self._nodes_cache.get(nid)
            if node_info and not node_info.is_disabled:
                wv = node_info.widgets_values
                if wv and len(wv) >= 2:
                    # 如果 width / height 没有外接 Primitive 节点，则直接修改 widget 数组
                    if w_nid == nid and isinstance(wv[0], (int, float)):
                        wv[0] = target_width
                    if h_nid == nid and isinstance(wv[1], (int, float)):
                        wv[1] = target_height

            # 如果接了 Primitive 节点，需要更新 Primitive 节点的 widget 值
            for node_info in self._nodes_cache.values():
                if node_info.is_disabled:
                    continue
                if node_info.class_type in KNOWN_PRIMITIVE_TYPES:
                    wv = node_info.widgets_values
                    if wv and isinstance(wv[0], (int, float)):
                        if node_info.node_id == w_nid:
                            wv[0] = target_width
                        elif node_info.node_id == h_nid:
                            wv[0] = target_height

    # #endregion

    # #region 提交方法
    def submit(self, comfyui_url: str) -> bool:
        """提交工作流到 ComfyUI 的 /prompt 接口"""
        client_id: str = str(uuid.uuid4())
        payload: Dict[str, Any] = {
            "prompt": self.prompt,
            "client_id": client_id,
            "extra_data": {"extra_pnginfo": {"workflow": self.workflow}},
        }
        data: bytes = json.dumps(payload).encode("utf-8")
        req: urllib.request.Request = urllib.request.Request(
            f"{comfyui_url}/prompt",
            data=data,
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req) as f:
                json.loads(f.read().decode("utf-8"))
                return True
        except Exception:
            return False

    # #endregion
