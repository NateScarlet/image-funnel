#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
FilenameManager：管理 ComfyUI 工作流中的输出文件名更新和目录调整。
"""

import datetime
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

    def _evaluate_template_for_prompt(
        self,
        wf_template: str,
        prompt: Dict[str, Any],
        original_val: Optional[str] = None,
    ) -> str:
        """
        根据 workflow 中的模板 wf_template 和 prompt 中各源变量节点的当前输入值，
        将模板展开求值为发给 API 的静态 filename_prefix 字符串（prompt 不保留 % 模板语法）。
        """
        result = wf_template
        now = datetime.datetime.now()

        # 1. 替换非日期变量 %NodeTitle.input_key% 或 %NodeTitle%
        for m in re.finditer(r"%([a-zA-Z_][a-zA-Z0-9_.]*?)%", wf_template):
            var_name = m.group(1)
            placeholder = m.group(0)
            if var_name.startswith("date:"):
                continue

            node_title = var_name.rsplit(".", 1)[0]
            pid = self._title_to_node.get(node_title)
            if pid and pid in prompt:
                input_key = var_name.rsplit(".", 1)[1] if "." in var_name else "value"
                val = prompt[pid].get("inputs", {}).get(input_key)
                if isinstance(val, str):
                    result = result.replace(placeholder, val)

        # 2. 替换日期变量 %date:format%
        for m in re.finditer(r"%date:([^%]+)%", result):
            comfy_fmt = m.group(1)
            py_fmt, _ = self.convert_comfy_date_format_to_python(comfy_fmt)
            placeholder = m.group(0)
            new_date_str = now.strftime(py_fmt)
            result = result.replace(placeholder, new_date_str, 1)

        return result

    def update_output_filenames(self) -> None:
        """
        扫描 workflow 和 prompt 中的输出节点：
        - 在 workflow 中保留模版变量语法（如 %Project.value%）。
        - 在 prompt API 结构中，根据 workflow 模板和 prompt 里源变量的当前值展开为求值后的快照，使修改源变量生效。
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

        for node_info in self._date_filename_nodes:
            if node_info.is_disabled:
                continue

            widgets_values = node_info.widgets_values
            if not widgets_values:
                continue

            wf_template: Optional[str] = None
            for val in widgets_values:
                if isinstance(val, str) and "%" in val:
                    wf_template = val
                    break

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

                    if wf_template:
                        # workflow 中保留模板变量，prompt 中展开求值
                        eval_val = self._evaluate_template_for_prompt(
                            wf_template,
                            prompt,
                            original_val=(
                                prefix_val if isinstance(prefix_val, str) else None
                            ),
                        )
                        src_inputs[src_key] = eval_val
                    elif (
                        isinstance(prefix_val, str)
                        and node_info.node_id in date_patterns
                    ):
                        py_fmt, regex_pattern, _, _ = date_patterns[node_info.node_id]
                        new_date_str: str = now.strftime(py_fmt)
                        if re.search(regex_pattern, prefix_val):
                            new_prefix: str = re.sub(
                                regex_pattern, new_date_str, prefix_val
                            )
                            src_inputs[src_key] = new_prefix

    def adjust_output_directory(self, rel_dir: str) -> None:
        """
        根据相对路径 rel_dir 调整所有输出节点的 filename_prefix，使输出文件总是
        直接落在 rel_dir 下，不创建任何子目录：rel_dir 之外的目录层级（字面目录、
        模板变量之间的分隔符）统一拍平为 __ 连接的文件名前缀。
        在 workflow 中保留模版变量语法，在 prompt 中求值展开为静态快照字符串。
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

            terminal_info = self._accessor.get_node_by_id(src_node_id)
            wf_template: Optional[str] = None
            if terminal_info and terminal_info.widgets_values:
                for val in terminal_info.widgets_values:
                    if isinstance(val, str) and "%" in val:
                        wf_template = val
                        break

            # 已直接落在目标目录时不修改：prompt 侧是已求值的静态快照，
            # 空值占位符会残留连续分隔符（如 Title 为空使快照为 TODO//日期），
            # 直接用原始字符串会误判出剩余层级；先归一化分隔符得到实际落点，
            # 再与 rel_dir 比较
            normalized = re.sub(r"[\\/]+", "/", original_val).strip("/")
            if rel_dir and rel_dir != ".":
                if normalized == rel_dir:
                    continue
                if normalized.startswith(f"{rel_dir}/"):
                    rest = normalized[len(rel_dir) + 1 :]
                    if "/" not in rest:
                        continue
            elif "/" not in normalized:
                # 目标为输出根目录且表达式已无任何目录层级时同样无需调整
                continue

            if wf_template:
                non_date_vars: List[str] = []
                for m in re.finditer(r"%([a-zA-Z_][a-zA-Z0-9_.]*?)%", wf_template):
                    var_name = m.group(1)
                    if not var_name.startswith("date:"):
                        non_date_vars.append(var_name)

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
                                if not self._accessor.get_node_by_id(pid):
                                    raise ValueError(
                                        f"Workflow node '{pid}' is missing in metadata"
                                    )
                            for pid, input_key, val in updates:
                                self._accessor.set_prompt_input(pid, input_key, val)
                                self._accessor.update_workflow_node_text(
                                    pid, val, input_key
                                )
                            # workflow 保留模板语法，prompt 中求值展开
                            eval_val = self._evaluate_template_for_prompt(
                                wf_template, prompt, original_val
                            )
                            src_inputs[src_key] = eval_val
                            continue

                # 无法通过变量映射 rel_dir：调整模板使其直接落在 rel_dir 下
                new_template = self._adjust_with_rel_dir(rel_dir, wf_template)
                if terminal_info and terminal_info.widgets_values:
                    for idx, val in enumerate(terminal_info.widgets_values):
                        if isinstance(val, str) and "%" in val:
                            terminal_info.widgets_values[idx] = new_template

                eval_val = self._evaluate_template_for_prompt(
                    new_template, prompt, original_val
                )
                src_inputs[src_key] = eval_val
                continue

            # 无模板：整个前缀视为静态路径，调整使其直接落在 rel_dir 下
            new_val = self._adjust_with_rel_dir(rel_dir, original_val)
            src_inputs[src_key] = new_val

            wf_src_info = self._accessor.get_node_by_id(src_node_id)
            if wf_src_info and wf_src_info.widgets_values:
                for idx, val in enumerate(wf_src_info.widgets_values):
                    if isinstance(val, str):
                        wf_src_info.widgets_values[idx] = new_val

    @staticmethod
    def _adjust_with_rel_dir(rel_dir: str, value: str) -> str:
        """
        调整 value 使其直接落在 rel_dir 下：rel_dir 前缀保留为真实目录，
        rel_dir 之外的所有目录层级拍平为 __ 连接。
        rel_dir 为 . 或空时只拍平不拼前缀。
        """
        if not rel_dir or rel_dir == ".":
            return FilenameManager._flatten_path_segments(value)
        prefix = f"{rel_dir}/"
        if value.startswith(prefix):
            return prefix + FilenameManager._flatten_path_segments(value[len(prefix) :])
        return prefix + FilenameManager._flatten_path_segments(value)

    @staticmethod
    def _flatten_path_segments(value: str) -> str:
        """
        将路径字符串中的目录分隔符（/ 或 \\）拍平为 __ 连接。
        先按标准路径清理合并连续分隔符（ComfyUI 对连续分隔符本就是合并处理的），
        再逐分隔符替换为 __；段名中字面的 __ 不会被改动。
        %...% 模板变量占位符内部不含分隔符，原样保留。
        """
        cleaned = re.sub(r"[\\/]+", "/", value)
        return cleaned.replace("/", "__")

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
