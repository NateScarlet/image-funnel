#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
FilenameManager：管理 ComfyUI 工作流中的输出文件名更新和目录调整。
"""

import datetime
import os
import re
from typing import Dict, List, Any, Optional, Tuple

from .node_accessor import NodeAccessor
from .prompt_locator import find_terminal_input, NodeInfo


class FilenameManager:
    """
    文件名管理器，负责更新日期模板文件名和调整输出目录。
    接收已分析好的日期文件名节点列表和标题到节点 ID 映射。
    """

    def __init__(
        self,
        accessor: NodeAccessor,
        date_filename_nodes: List[NodeInfo],
        title_to_node: Dict[str, str],
    ):
        self._accessor = accessor
        self._date_filename_nodes = date_filename_nodes
        self._title_to_node = title_to_node

    def update_output_filenames(self) -> None:
        """
        扫描 workflow 和 prompt 中的输出节点，如果发现使用了 %date:...% 占位符且在 prompt 中被写死为旧日期，
        将其更新为当前系统时间的日期静态值。
        """
        if not self._date_filename_nodes:
            return

        date_patterns: Dict[str, Tuple[str, str, bool, Optional[str]]] = {}

        for node_info in self._date_filename_nodes:
            if node_info.is_disabled:
                continue

            widgets_values = node_info.widgets_values
            if not widgets_values:
                continue

            for val in widgets_values:
                if isinstance(val, str) and "%date:" in val:
                    match = re.search(r"%date:([^%]+)%", val)
                    if match:
                        comfy_fmt: str = match.group(1)
                        py_fmt, regex_pattern = (
                            self.convert_comfy_date_format_to_python(comfy_fmt)
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

        prompt = self._accessor.prompt
        for node_id_str, (
            py_fmt,
            regex_pattern,
            is_subgraph,
            subgraph_id,
        ) in date_patterns.items():
            api_node_ids: List[str] = []
            if not is_subgraph:
                api_node_ids = [node_id_str]
            else:
                for cached_info in self._accessor.nodes_cache.values():
                    if (
                        not cached_info.is_subgraph
                        and cached_info.node_type == subgraph_id
                    ):
                        if not cached_info.is_disabled:
                            api_node_ids.append(f"{cached_info.node_id}:{node_id_str}")

            for api_node_id in api_node_ids:
                if api_node_id not in prompt:
                    raise ValueError(
                        f"Workflow output node {api_node_id} is missing in the prompt API structure."
                    )

                inputs: Dict[str, Any] = prompt[api_node_id].get("inputs", {})
                filename_prefix: Any = inputs.get("filename_prefix")

                if filename_prefix:
                    src_node_id: str
                    src_key: str
                    src_node_id, src_key = find_terminal_input(
                        prompt, api_node_id, "filename_prefix"
                    )
                    if src_node_id not in prompt:
                        raise ValueError(
                            f"Source node {src_node_id} for filename_prefix is missing in the prompt API structure."
                        )

                    src_inputs = prompt[src_node_id].setdefault("inputs", {})
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
        """
        prompt = self._accessor.prompt
        for node_id, node in prompt.items():
            inputs = node.get("inputs", {})
            if "filename_prefix" not in inputs:
                continue

            src_node_id, src_key = find_terminal_input(
                prompt, node_id, "filename_prefix"
            )
            src_node = prompt.get(src_node_id)
            if not src_node:
                continue

            src_inputs = src_node.setdefault("inputs", {})
            original_val = src_inputs.get(src_key)
            if not isinstance(original_val, str):
                continue

            terminal_info = self._accessor.nodes_cache.get(src_node_id)
            wf_template: Optional[str] = None
            if terminal_info and terminal_info.widgets_values:
                for val in terminal_info.widgets_values:
                    if isinstance(val, str) and "%" in val:
                        wf_template = val
                        break

            if wf_template:
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
                                prompt[pid].setdefault("inputs", {})[input_key] = val
                            new_prefix = wf_template
                            for m in re.finditer(r"%date:([^%]+)%", new_prefix):
                                comfy_fmt = m.group(1)
                                _, date_regex = (
                                    self.convert_comfy_date_format_to_python(comfy_fmt)
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

                    flat_val = original_val.replace("/", "__").replace("\\", "__")
                    new_val = (
                        f"{rel_dir}/{flat_val}"
                        if rel_dir and rel_dir != "."
                        else flat_val
                    )
                    src_inputs[src_key] = new_val

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

            original_basename = os.path.basename(original_val)
            new_val = (
                f"{rel_dir}/{original_basename}"
                if rel_dir and rel_dir != "."
                else original_basename
            )
            src_inputs[src_key] = new_val

            wf_src_info = self._accessor.nodes_cache.get(src_node_id)
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

    @staticmethod
    def convert_comfy_date_format_to_python(comfy_fmt: str) -> Tuple[str, str]:
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
