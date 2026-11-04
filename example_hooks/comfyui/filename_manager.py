#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
FilenameManager：管理 ComfyUI 工作流中的输出文件名更新和目录调整。

全局约束（改动这套逻辑时不要破坏）：

- **不得静默丢弃原有路径数据**：输出文件名里除目标目录外的层级只能拍平进文件名
  前缀，直接取 basename 等于丢掉用户的目录结构信息。
- **两侧必须一致**：workflow 侧保留 `%…%` 模板语法（只存在于 widgets_values 中），
  prompt 侧恒为按同一模板求值出的静态快照——提交侧不能持有工作流侧无法复现的值。
"""

import datetime
import os
import re
from typing import Dict, List, Any, Optional, Tuple

from .node_accessor import NodeAccessor
from .prompt_locator import find_terminal_input, NodeInfo

# 扁平文件名主体的长度上限：目标目录之外的层级全部拍平进该主体，
# 过长的前缀会让 ComfyUI 网页端无法处理（分支反复调整时尤其明显）
MAX_FLAT_NAME_LENGTH = 100

# 模板占位符：截断时不得从内部切断
_PLACEHOLDER_PATTERN = re.compile(r"%[^%]+%")


class FilenameManager:
    """
    文件名管理器，负责更新日期模板文件名和调整输出目录。
    接收已分析好的日期文件名节点列表和标题到节点 ID 映射。
    """

    def __init__(
        self,
        accessor: NodeAccessor,
        date_filename_nodes: List[NodeInfo],
        title_to_node: Dict[str, List[str]],
        now: datetime.datetime,
    ):
        self._accessor = accessor
        self._date_filename_nodes = date_filename_nodes
        self._title_to_node = title_to_node
        # 统一的时间取值：一次执行内的全部日期求值共用同一时刻，否则基线与提交侧
        # 可能跨秒求值出不同的 filename_prefix
        self._now = now

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
        now = self._now

        # 1. 替换非日期变量 %NodeTitle.input_key% 或 %NodeTitle%
        for m in re.finditer(r"%([a-zA-Z_][a-zA-Z0-9_.]*?)%", wf_template):
            var_name = m.group(1)
            placeholder = m.group(0)
            if var_name.startswith("date:"):
                continue

            node_title = var_name.rsplit(".", 1)[0]
            input_key = var_name.rsplit(".", 1)[1] if "." in var_name else "value"
            pid = self._first_prompt_key(node_title)
            if pid is not None:
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

    # #region 同名候选节点
    #
    # 变量 `%Title.key%` 的候选集合由 WorkflowPromptPair 提供（顶层 prompt 同名节点 +
    # 子图 definitions 内同名节点）。这里负责把候选 ID 翻译成 prompt 实际节点键、
    # 读取/校验当前值并同步写入两侧。
    #
    # 观测到的 ComfyUI 前端表达式解析行为（searchAndReplace.ts，序列化 Save* 节点的
    # filename_prefix 时执行）：匹配名优先取 properties["Node name for S&R"]、回退
    # title；多个同名匹配时取「收集顺序的第一个」（collectAllNodes 先递归子图再收集
    # 当前层）；替换值会被路径分隔符/控制字符过滤为 `_`；不区分节点 mute/bypass。
    #
    # 本实现不镜像这套顺序：它是前端内部实现，任何升级都可能变化，也没有文档承诺；
    # 依赖它只会把「静默写错节点」换成另一种静默错位。改为把候选集合全部同步成同一个
    # 值后，ComfyUI 无论按哪种顺序解析到的都是同一个结果。

    def _prompt_keys_for_candidate(self, candidate_id: str) -> List[str]:
        """把候选 ID 解析为 prompt 中的实际节点键。

        子图候选以 `subgraph:<sg>:<nid>` 表征，其值在 prompt 中分散于各子图实例
        展开出的 `<实例>:<nid>` 键；按 workflow 顶层实例节点的 type 找出同子图实例，
        顺序与 workflow nodes 顺序一致。
        """
        if not candidate_id.startswith("subgraph:"):
            return [candidate_id]
        _, subgraph_id, child_id = candidate_id.split(":", 2)
        keys: List[str] = []
        for info in self._accessor.nodes_cache.values():
            if info.is_subgraph or info.node_type != subgraph_id:
                continue
            key = f"{info.node_id}:{child_id}"
            if key in self._accessor.prompt:
                keys.append(key)
        return keys

    def _first_prompt_key(self, title: str) -> Optional[str]:
        """取同名候选集合中第一个能在 prompt 读取的节点键。

        同步阶段已把所有候选写成同一个值，展开模板时读哪一个都一致。
        """
        for candidate_id in self._title_to_node.get(title, []):
            for key in self._prompt_keys_for_candidate(candidate_id):
                if key in self._accessor.prompt:
                    return key
        return None

    def _candidate_values(self, candidate_id: str, input_key: str) -> List[Any]:
        """候选节点在各表征下的当前值。

        子图候选同时读取 definitions 节点的 widget 与各实例的 prompt 输入：任一
        不一致都说明无法安全确定唯一目标值。workflow 元数据缺失的候选没有可比较
        的值：一致性判定跳过它，实际写入时才由写入接口报错。
        """
        info = self._accessor.get_node_by_id(candidate_id)
        if info is None:
            return []
        values: List[Any] = []
        if info.is_subgraph:
            if info.widgets_values:
                values.append(info.widgets_values[0])
            for key in self._prompt_keys_for_candidate(candidate_id):
                values.append(
                    self._accessor.prompt.get(key, {})
                    .get("inputs", {})
                    .get(input_key, "")
                )
        elif candidate_id in self._accessor.prompt:
            values.append(
                self._accessor.prompt[candidate_id].get("inputs", {}).get(input_key, "")
            )
        return values

    @staticmethod
    def _candidate_label(candidate_id: str) -> str:
        """候选 ID 的可读标签；子图候选附带所在子图 ID 便于定位"""
        if candidate_id.startswith("subgraph:"):
            _, subgraph_id, _ = candidate_id.split(":", 2)
            return f"{candidate_id}（子图 {subgraph_id}）"
        return candidate_id

    def _assert_title_consistent(
        self, title: str, candidate_ids: List[str], input_key: str
    ) -> None:
        """同名候选对同一 input key 的值必须一致，否则直接报错中止该图片。

        报错信息包含 title、全部候选节点 ID 与子图 ID，便于用户删除重复节点。
        """
        parts: List[str] = []
        inconsistent = False
        reference_value: Any = None
        has_reference = False
        for candidate_id in candidate_ids:
            values = self._candidate_values(candidate_id, input_key)
            distinct: List[Any] = []
            for value in values:
                if not distinct or value != distinct[0]:
                    distinct.append(value)
                if not has_reference:
                    reference_value = value
                    has_reference = True
                elif value != reference_value:
                    inconsistent = True
            if len(distinct) > 1:
                inconsistent = True
            rendered = "、".join(repr(value) for value in distinct)
            parts.append(f"{FilenameManager._candidate_label(candidate_id)}={rendered}")

        if inconsistent:
            raise ValueError(
                f"同名节点 '{title}' 的当前值不一致"
                f"（{'；'.join(parts)}），无法安全确定输出目录，已跳过该图片"
            )

    def _write_candidate(self, candidate_id: str, input_key: str, value: str) -> None:
        """把候选节点在 prompt 与 workflow 两侧都写成同一个值"""
        for key in self._prompt_keys_for_candidate(candidate_id):
            self._accessor.set_prompt_input(key, input_key, value)
        self._accessor.update_workflow_node_text(candidate_id, value, input_key)

    # #endregion

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

        now = self._now
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

    def adjust_output_directory(
        self, rel_dir: str, source_rel_dir: str
    ) -> Optional[str]:
        """
        根据相对路径 rel_dir 调整所有输出节点的 filename_prefix，使输出文件总是
        直接落在 rel_dir 下，不创建任何子目录：rel_dir 之外的目录层级（字面目录、
        模板变量之间的分隔符）统一拍平为 __ 连接的文件名前缀。
        source_rel_dir 是源图当前所在目录（相对输出根目录），用于先剥离上一代目标
        目录，避免反复调整时把历史目录一代代累积进文件名。
        唯一的例外是模板中的非日期变量与 rel_dir 分段一一对应：此时变量值本身
        充当 rel_dir 路径（如 %Project.value%/%Title.value% 对应 NewProject/NewTitle），
        分隔符保留。
        在 workflow 中保留模版变量语法，在 prompt 中求值展开为静态快照字符串。

        返回给用户看的同名节点提示（存在多个同名候选并被同步时）；无重复时为 None。
        同名候选值不一致时抛出 ValueError，调用方应跳过该图片，绝不静默渲染到错误目录。
        """
        prompt = self._accessor.prompt
        duplicate_messages: List[str] = []
        notified_titles: List[str] = []
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

            non_date_vars: List[str] = []
            if wf_template:
                for m in re.finditer(r"%([a-zA-Z_][a-zA-Z0-9_.]*?)%", wf_template):
                    var_name = m.group(1)
                    if not var_name.startswith("date:"):
                        non_date_vars.append(var_name)

            # 模板含非日期变量时先校验同名候选：即使本次不写入（已落在目标目录或
            # 走拍平分支），交付的 workflow 仍会被 S&R 解析，任取一个候选就会静默错位
            for var_name in non_date_vars:
                node_title = var_name.rsplit(".", 1)[0]
                input_key = var_name.rsplit(".", 1)[1] if "." in var_name else "value"
                candidates = self._title_to_node.get(node_title)
                if candidates:
                    self._assert_title_consistent(node_title, candidates, input_key)

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
                if non_date_vars:
                    rel_parts = [p for p in rel_dir.split("/") if p]

                    if len(rel_parts) == len(non_date_vars):
                        updates: List[Tuple[str, List[str], str, str]] = []
                        for i, var_name in enumerate(non_date_vars):
                            node_title = var_name.rsplit(".", 1)[0]
                            candidates = self._title_to_node.get(node_title)
                            if not candidates:
                                updates.clear()
                                break
                            input_key = (
                                var_name.rsplit(".", 1)[1]
                                if "." in var_name
                                else "value"
                            )
                            updates.append(
                                (node_title, candidates, input_key, rel_parts[i])
                            )

                        if updates:
                            # 一致性已在写入前统一校验；这里再确认写入目标存在，
                            # 避免只写进 prompt 却写不到 workflow 的半成品
                            for node_title, candidates, input_key, _ in updates:
                                for candidate_id in candidates:
                                    if (
                                        self._accessor.get_node_by_id(candidate_id)
                                        is None
                                    ):
                                        raise ValueError(
                                            f"Workflow node '{candidate_id}' is missing in metadata"
                                        )
                            for node_title, candidates, input_key, val in updates:
                                if (
                                    len(candidates) > 1
                                    and node_title not in notified_titles
                                ):
                                    notified_titles.append(node_title)
                                    labels = "、".join(
                                        self._candidate_label(cid) for cid in candidates
                                    )
                                    duplicate_messages.append(
                                        f"存在同名节点 {node_title}"
                                        f"（节点 ID {labels}），已同步"
                                    )
                                for candidate_id in candidates:
                                    self._write_candidate(candidate_id, input_key, val)
                            # workflow 保留模板语法，prompt 中求值展开
                            eval_val = self._evaluate_template_for_prompt(
                                wf_template, prompt, original_val
                            )
                            src_inputs[src_key] = eval_val
                            continue

                # 无法通过变量映射 rel_dir：调整模板使其直接落在 rel_dir 下
                new_template = self._adjust_with_rel_dir(
                    rel_dir, source_rel_dir, wf_template
                )
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
            new_val = self._adjust_with_rel_dir(rel_dir, source_rel_dir, original_val)
            src_inputs[src_key] = new_val

            wf_src_info = self._accessor.get_node_by_id(src_node_id)
            if wf_src_info and wf_src_info.widgets_values:
                for idx, val in enumerate(wf_src_info.widgets_values):
                    if isinstance(val, str):
                        wf_src_info.widgets_values[idx] = new_val

        return "；".join(duplicate_messages) if duplicate_messages else None

    @staticmethod
    def _adjust_with_rel_dir(rel_dir: str, source_rel_dir: str, value: str) -> str:
        """
        调整 value 使其直接落在 rel_dir 下：rel_dir 前缀保留为真实目录，
        rel_dir 之外的所有目录层级拍平为 __ 连接，扁平主体受长度上限约束。
        value 开头若是源图当前目录（上一代目标目录）则先剥离，避免历史目录累积；
        只按源目录剥离：源图在输出根或目录不匹配时，原有层级全部保留。
        rel_dir 为 . 或空时只剥离、拍平，不拼前缀。
        """
        normalized = re.sub(r"[\\/]+", "/", value)
        rest = FilenameManager._strip_dir_prefix(normalized, source_rel_dir)
        if rest is None:
            rest = normalized

        flat = FilenameManager._limit_flat_name_length(
            FilenameManager._flatten_path_segments(rest)
        )
        if not rel_dir or rel_dir == ".":
            return flat
        return f"{rel_dir}/{flat}"

    @staticmethod
    def _strip_dir_prefix(value: str, dir_path: str) -> Optional[str]:
        """
        若 value 以 dir_path 加一个目录分隔符开头，返回剥离该前缀后的剩余部分。

        value 需已归一化为 / 分隔。Windows 下目录比较不区分大小写（os.path.normcase），
        剩余部分保留原始大小写。dir_path 为输出根目录（. 或空）时不剥离。
        """
        normalized_dir = dir_path.replace("\\", "/").strip("/")
        if not normalized_dir or normalized_dir == ".":
            return None
        prefix_length = len(normalized_dir) + 1
        candidate = value[:prefix_length]
        if os.path.normcase(candidate) == os.path.normcase(f"{normalized_dir}/"):
            return value[prefix_length:]
        return None

    @staticmethod
    def _limit_flat_name_length(value: str) -> str:
        """
        把扁平主体限制在 MAX_FLAT_NAME_LENGTH 内。

        超限时按 __ 段从头部整段丢弃直到不超限；若最后一段本身仍超限，
        再对该段按字符截头保尾。截断点落在 %...% 模板占位符内部时保留原样：
        无法在不破坏占位符的前提下满足上限，宁可超长。
        """
        if len(value) <= MAX_FLAT_NAME_LENGTH:
            return value

        # 从头部按 __ 段丢弃；位于占位符内部的 __ 不是段界，避免把占位符拆成残片
        segment_start = 0
        search_from = 0
        while len(value) - segment_start > MAX_FLAT_NAME_LENGTH:
            separator = value.find("__", search_from)
            if separator == -1:
                break
            if FilenameManager._is_inside_placeholder(value, separator):
                search_from = separator + 2
                continue
            segment_start = separator + 2
            search_from = segment_start
        body = value[segment_start:]
        if len(body) <= MAX_FLAT_NAME_LENGTH:
            return body

        cut = len(body) - MAX_FLAT_NAME_LENGTH
        for match in _PLACEHOLDER_PATTERN.finditer(body):
            if match.start() < cut < match.end():
                return body
        return body[cut:]

    @staticmethod
    def _is_inside_placeholder(value: str, index: int) -> bool:
        """判断位置是否落在某个 %...% 占位符内部"""
        return any(
            match.start() < index < match.end()
            for match in _PLACEHOLDER_PATTERN.finditer(value)
        )

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
