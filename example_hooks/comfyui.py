#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import random
import uuid
import datetime
import re
import urllib.request
from typing import Dict, List, Tuple, Any, Optional, cast
import logging
import argparse

from graphql_utils import update_image_label, fetch_images

_LOGGER = logging.getLogger(__name__)

# 捕获 PIL 导入错误并给出清晰提示
try:
    from PIL import Image
except ImportError:
    print(
        "Error: Missing Pillow library. Please install it in your Python environment to handle image metadata:",
        file=sys.stderr,
    )
    print("      pip install Pillow", file=sys.stderr)
    sys.exit(1)

KNOWN_PRIMITIVE_TYPES = {
    "PrimitiveInt",
    "PrimitiveFloat",
    "PrimitiveString",
    "PrimitiveBoolean",
}
KNOWN_SWITCH_TYPES = {"Any Switch (rgthree)", "ComfySwitchNode"}


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


def update_output_filenames(prompt: Dict[str, Any], workflow: Dict[str, Any]) -> None:
    """
    扫描 workflow 和 prompt 中的输出节点，如果发现使用了 %date:...% 占位符且在 prompt 中被写死为旧日期，
    将其更新为当前系统时间的日期静态值。
    """
    if not workflow:
        return

    # 汇总所有待处理的节点（包含顶层节点和子图内部节点）
    candidate_nodes: List[Tuple[Dict[str, Any], str, bool, Optional[str], str]] = []

    # 1. 收集工作流顶层节点
    for node in workflow.get("nodes", []):
        candidate_nodes.append(
            (node, str(node.get("id")), False, None, node.get("type"))
        )

    # 2. 收集各子图定义内部的节点
    subgraphs: List[Dict[str, Any]] = workflow.get("definitions", {}).get(
        "subgraphs", []
    )
    for subgraph in subgraphs:
        subgraph_id = subgraph.get("id")
        for node in subgraph.get("nodes", []):
            candidate_nodes.append(
                (
                    node,
                    str(node.get("id")),
                    True,
                    subgraph_id,
                    f"{subgraph.get('name', 'Subgraph')}:{node.get('type')}",
                )
            )

    date_patterns: Dict[str, Tuple[str, str, bool, Optional[str]]] = (
        {}
    )  # node_id_str -> (py_fmt, regex_pattern, is_subgraph, subgraph_id)

    for (
        node,
        node_id_str,
        is_subgraph,
        subgraph_id,
        node_type_for_log,
    ) in candidate_nodes:
        widgets_values_raw: Any = node.get("widgets_values")
        if not isinstance(widgets_values_raw, list):
            continue
        widgets_values = cast(List[Any], widgets_values_raw)

        # 寻找包含 "%date:" 的字符串 widget
        for val in widgets_values:
            if isinstance(val, str) and "%date:" in val:
                match = re.search(r"%date:([^%]+)%", val)
                if match:
                    comfy_fmt: str = match.group(1)
                    py_fmt, regex_pattern = convert_comfy_date_format_to_python(
                        comfy_fmt
                    )
                    date_patterns[node_id_str] = (
                        py_fmt,
                        regex_pattern,
                        is_subgraph,
                        subgraph_id,
                    )
                    _LOGGER.info(
                        f"Workflow node {node_id_str} ({node_type_for_log}) uses date template: {comfy_fmt}"
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
        # 映射到 API (prompt) 端节点 ID 列表。对于子图节点，需根据其所有顶层实例生成前缀 ID 列表。
        api_node_ids: List[str] = []
        if not is_subgraph:
            api_node_ids = [node_id_str]
        else:
            for parent_node in workflow.get("nodes", []):
                if parent_node.get("type") == subgraph_id:
                    api_node_ids.append(f"{parent_node.get('id')}:{node_id_str}")

        for api_node_id in api_node_ids:
            if api_node_id in prompt:
                inputs: Dict[str, Any] = prompt[api_node_id].get("inputs", {})
                filename_prefix: Any = inputs.get("filename_prefix")

                if filename_prefix:
                    # 追溯 filename_prefix 端口的值源头
                    src_node_id: str
                    src_key: str
                    src_node_id, src_key = find_terminal_input(
                        prompt, api_node_id, "filename_prefix"
                    )
                    if src_node_id in prompt:
                        src_inputs = prompt[src_node_id].setdefault("inputs", {})
                        prefix_val: Any = src_inputs.get(src_key)
                        if isinstance(prefix_val, str):
                            new_date_str: str = now.strftime(py_fmt)
                            if re.search(regex_pattern, prefix_val):
                                new_prefix: str = re.sub(
                                    regex_pattern, new_date_str, prefix_val
                                )
                                src_inputs[src_key] = new_prefix
                                _LOGGER.info(
                                    f"Prompt node {src_node_id} (key {src_key}) filename_prefix updated: {prefix_val} -> {new_prefix}"
                                )


def update_seeds(prompt: Dict[str, Any], workflow: Dict[str, Any]) -> int:
    """
    修改 prompt (API 结构) 和 workflow (UI 结构) 中的随机种子值。
    支持一个节点存在多个种子，并在 prompt 和 workflow 中同步更新。
    通过识别 workflow.nodes 中 widgets_values 的数组临接特征（[seed数值, 变化策略]）精准替换种子值。
    返回成功修改的种子总数。
    """
    if not workflow:
        return 0

    modified_count: int = 0

    # 汇总所有待处理的节点（包含顶层节点和子图内部节点）
    candidate_nodes: List[Tuple[Dict[str, Any], str, bool, Optional[str], str]] = []

    # 1. 收集工作流顶层节点
    for node in workflow.get("nodes", []):
        candidate_nodes.append(
            (node, str(node.get("id")), False, None, node.get("type"))
        )

    # 2. 收集各子图定义内部的节点
    subgraphs: List[Dict[str, Any]] = workflow.get("definitions", {}).get(
        "subgraphs", []
    )
    for subgraph in subgraphs:
        subgraph_id = subgraph.get("id")
        for node in subgraph.get("nodes", []):
            candidate_nodes.append(
                (
                    node,
                    str(node.get("id")),
                    True,
                    subgraph_id,
                    f"{subgraph.get('name', 'Subgraph')}:{node.get('type')}",
                )
            )

    # 遍历所有候选节点，定位包含种子的 Widget 并更新
    for (
        node,
        node_id_str,
        is_subgraph,
        subgraph_id,
        node_type_for_log,
    ) in candidate_nodes:
        widgets_values_raw: Any = node.get("widgets_values")
        if not isinstance(widgets_values_raw, list):
            continue
        widgets_values = cast(List[Any], widgets_values_raw)

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
                    new_seed = (old_seed - 1) % 1125899906842624
                    if new_seed < 0:
                        new_seed = 0
                else:  # randomize
                    new_seed = random.randint(1, 1125899906842624)

                # A. 尝试更新 workflow 中的数值，使用户用 Web UI 打开 workflow 即可重现生成数值
                widgets_values[idx] = new_seed
                modified_count += 1
                _LOGGER.info(
                    f"Workflow node {node_id_str} ({node_type_for_log}) seed updated: {old_seed} -> {new_seed} (strategy: {strategy})"
                )

                # B. 映射到 API (prompt) 端节点 ID 列表。对于子图节点，需根据其所有顶层实例生成前缀 ID 列表。
                api_node_ids: List[str] = []
                if not is_subgraph:
                    api_node_ids = [node_id_str]
                else:
                    for parent_node in workflow.get("nodes", []):
                        if parent_node.get("type") == subgraph_id:
                            api_node_ids.append(
                                f"{parent_node.get('id')}:{node_id_str}"
                            )

                # 同步修改 API 端 (prompt) 结构中对应的连接源头。
                for api_node_id in api_node_ids:
                    if api_node_id in prompt:
                        inputs: Dict[str, Any] = prompt[api_node_id].get("inputs", {})
                        # 遍历此节点在 prompt 中的所有 inputs，寻找和当前种子关联的端口并追溯修改其源头值
                        for ik in list(inputs.keys()):
                            src_node_id: str
                            src_key: str
                            src_node_id, src_key = find_terminal_input(
                                prompt, api_node_id, ik
                            )
                            if src_node_id in prompt:
                                src_node: Dict[str, Any] = prompt[src_node_id]
                                src_inputs: Dict[str, Any] = src_node.get("inputs", {})
                                current_val: Any = src_inputs.get(src_key)

                                # 校验当前值是否等于 old_seed 且满足种子标识或 Primitive 属性
                                is_primitive = (
                                    src_node.get("class_type") in KNOWN_PRIMITIVE_TYPES
                                )
                                if (
                                    current_val == old_seed
                                    or str(current_val) == str(old_seed)
                                ) and (
                                    "seed" in ik or "seed" in src_key or is_primitive
                                ):
                                    src_inputs[src_key] = new_seed
                                    _LOGGER.info(
                                        f"  -> Prompt structure sync: updated source node {src_node_id} key {src_key} = {new_seed}"
                                    )

    return modified_count


def get_workflow_node_text(workflow: Dict[str, Any], node_id_str: str) -> Optional[str]:
    """
    在 UI 结构 workflow 中获取特定节点的文本 widget 数值。
    """
    if not workflow:
        return None

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


def strip_comments_for_prompt(text: str) -> str:
    """
    为 prompt 剥离注释行。如果一行去除首尾空白后以 '//' 开头，该整行将被完全过滤掉。
    """
    lines: List[str] = []
    for line in text.splitlines():
        if line.strip().startswith("//"):
            continue
        lines.append(line)
    return "\n".join(lines)


def process_double_track(
    workflow_text: str,
    prompt_text: str,
    command: str,
    prompt_str_arg: str,
    start_marker: str,
    end_marker: str,
    raw: bool,
    no_skip: bool,
    hard: bool,
) -> Tuple[Optional[str], Optional[str]]:
    """
    处理双轨道逻辑，输入 workflow_text 并执行 add/remove 动作，分别生成：
    new_workflow_text (包含 marker)
    new_prompt_text (不包含 marker)
    如果需要跳过操作，返回 (None, None)。
    """
    # 经过我们的清除逻辑得到的 prompt 文本，用于做等价性比对
    stripped_workflow = strip_comments_for_prompt(workflow_text)
    workflow_cleaned = "\n".join(
        [line.strip() for line in stripped_workflow.splitlines() if line.strip()]
    )
    prompt_cleaned = "\n".join(
        [line.strip() for line in prompt_text.splitlines() if line.strip()]
    )
    is_equivalent = workflow_cleaned == prompt_cleaned

    # 在 workflow 文本中定位 marker 区域
    start_idx = workflow_text.find(start_marker)
    end_idx = workflow_text.find(end_marker)
    has_marker = start_idx != -1 and end_idx != -1 and start_idx < end_idx

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
                _LOGGER.info(
                    f"Prompt '{prompt_str_arg}' already exists in marker region, skipping."
                )
                return None, None

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
                new_prompt_text = strip_comments_for_prompt(new_prompt_text_raw)
            else:
                # 回退：基于区域内容在 prompt 中做精确匹配
                target_match_content = strip_comments_for_prompt(marker_content).strip()
                if target_match_content and target_match_content in prompt_text:
                    new_prompt_text = prompt_text.replace(
                        target_match_content, new_content_prompt.strip(), 1
                    )
                else:
                    # 匹配不到加在尾部
                    if raw:
                        new_prompt_text = prompt_text.rstrip() + "\n" + prompt_str_arg
                    else:
                        new_prompt_text = prompt_text.rstrip() + f"\n{prompt_str_arg},"
        else:
            if contains_prompt(workflow_text) and not no_skip:
                _LOGGER.info(
                    f"Prompt '{prompt_str_arg}' already exists in text, skipping."
                )
                return None, None

            if raw:
                new_content_prompt = prompt_str_arg
            else:
                new_content_prompt = f"{prompt_str_arg},"

            # 第一次添加，双轨道组装：workflow 附加带 marker 区域，prompt 附加不带 marker 区域
            new_workflow_text = (
                workflow_text.rstrip()
                + f"\n{start_marker}\n"
                + new_content_prompt
                + f"\n{end_marker}\n"
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
                if not no_skip:
                    _LOGGER.info(
                        f"Prompt '{prompt_str_arg}' not found in marker region, skipping."
                    )
                    return None, None
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
                new_prompt_text = strip_comments_for_prompt(new_prompt_text_raw)
            else:
                # 回退：基于区域内容在 prompt 中做精确匹配
                target_match_content = strip_comments_for_prompt(marker_content).strip()
                if target_match_content and target_match_content in prompt_text:
                    new_prompt_text = prompt_text.replace(
                        target_match_content, new_content_prompt.strip(), 1
                    )
                else:
                    # 匹配不到则保持原样
                    new_prompt_text = prompt_text
        else:
            if not contains_prompt(workflow_text):
                if not no_skip:
                    _LOGGER.info(f"Prompt '{prompt_str_arg}' not found, skipping.")
                    return None, None
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

            # 第一次移除（原本无 marker），因为已移除，无需追加 marker 结构
            new_workflow_text = new_content_prompt
            new_prompt_text = new_content_prompt

    new_prompt_text = strip_comments_for_prompt(new_prompt_text)

    return new_workflow_text, new_prompt_text


def send_to_comfyui(
    comfyui_url: str, prompt: Dict[str, Any], workflow: Dict[str, Any]
) -> bool:
    """
    提交工作流到 ComfyUI 的 /prompt 接口。
    """
    client_id: str = str(uuid.uuid4())
    payload: Dict[str, Any] = {
        "prompt": prompt,
        "client_id": client_id,
        "extra_data": {"extra_pnginfo": {"workflow": workflow}},
    }
    data: bytes = json.dumps(payload).encode("utf-8")
    req: urllib.request.Request = urllib.request.Request(
        f"{comfyui_url}/prompt", data=data, headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req) as f:
            res: Dict[str, Any] = json.loads(f.read().decode("utf-8"))
            prompt_id: Any = res.get("prompt_id")
            _LOGGER.info(
                f"Workflow successfully queued to ComfyUI, prompt_id: {prompt_id}"
            )
            return True
    except Exception as e:
        _LOGGER.error(f"Failed to submit to ComfyUI: {e}")
        return False


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

        # 读取环境变量中配置的常见正反向关键词
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

                # 优先匹配关键词数量最多的输入，相同时取最长文本
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


def process_add_prompt(
    text: str,
    prompt_to_add: str,
    start_marker: str,
    end_marker: str,
    raw: bool,
    no_skip: bool,
) -> Optional[str]:
    """
    添加提示词行。如果跳过，返回 None。
    """
    start_idx = text.find(start_marker)
    end_idx = text.find(end_marker)

    def contains_prompt(area: str) -> bool:
        if raw:
            return prompt_to_add in area
        parts = [p.strip().lower() for p in area.split(",")]
        return prompt_to_add.strip().lower() in parts

    if start_idx != -1 and end_idx != -1 and start_idx < end_idx:
        before_marker = text[:start_idx]
        marker_content = text[start_idx + len(start_marker) : end_idx]
        after_marker = text[end_idx + len(end_marker) :]

        if contains_prompt(marker_content):
            if not no_skip:
                _LOGGER.info(
                    f"Prompt '{prompt_to_add}' already exists in marker region, skipping."
                )
                return None

        if raw:
            new_content = marker_content + prompt_to_add
        else:
            stripped = marker_content.strip()
            if stripped:
                if not stripped.endswith(","):
                    stripped += ","
                new_content = f" {stripped} {prompt_to_add},"
            else:
                new_content = f" {prompt_to_add},"

        return before_marker + start_marker + new_content + end_marker + after_marker
    else:
        if contains_prompt(text):
            if not no_skip:
                _LOGGER.info(
                    f"Prompt '{prompt_to_add}' already exists in text, skipping."
                )
                return None

        if raw:
            appended = f"\n{start_marker}\n{prompt_to_add}\n{end_marker}"
        else:
            appended = f"\n{start_marker}\n{prompt_to_add},\n{end_marker}"

        if text.strip():
            return text.rstrip() + appended
        else:
            return appended

def update_workflow_node_text(
    workflow: Dict[str, Any], node_id_str: str, new_text: str
) -> None:
    """
    在 UI 结构 workflow 中同步更新特定节点的文本 widget 数值。
    """
    if not workflow:
        return

    for node in workflow.get("nodes", []):
        if str(node.get("id")) == node_id_str:
            widgets_values = node.get("widgets_values")
            if isinstance(widgets_values, list):
                val_list = cast(List[Any], widgets_values)
                if len(val_list) > 0:
                    val_list[0] = new_text
                    _LOGGER.info(f"Updated workflow node {node_id_str} text widget.")
                    return

    if ":" in node_id_str:
        _, child_id = node_id_str.split(":", 1)
        subgraphs = workflow.get("definitions", {}).get("subgraphs", [])
        for subgraph in subgraphs:
            for node in subgraph.get("nodes", []):
                if str(node.get("id")) == child_id:
                    widgets_values = node.get("widgets_values")
                    if isinstance(widgets_values, list):
                        val_list = cast(List[Any], widgets_values)
                        if len(val_list) > 0:
                            val_list[0] = new_text
                            _LOGGER.info(
                                f"Updated workflow subgraph node {child_id} text widget."
                            )
                            return


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    args = parse_args()

    image_paths_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "")
    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    comfyui_url: str = os.getenv("COMFYUI_URL", "http://127.0.0.1:8188")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    # 过滤器星级检查（针对 add/remove 指令的选做项）
    required_rating_str = os.getenv("HOOK_IMAGE_RATING")
    required_rating: Optional[int] = None
    if required_rating_str:
        try:
            required_rating = int(required_rating_str)
        except ValueError:
            pass

    queue_count_env = os.getenv("HOOK_QUEUE_COUNT", "1")
    try:
        queue_count = int(queue_count_env)
        if queue_count < 1:
            queue_count = 1
    except ValueError:
        queue_count = 1

    try:
        image_paths: List[str] = json.loads(image_paths_str) if image_paths_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_PATHS: {e}")
        sys.exit(1)

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS: {e}")
        sys.exit(1)

    # 汇总最终要处理 of (image_id, path) 列表
    targets: List[Tuple[str, str]] = []
    if args.command in ["add", "remove"]:
        _LOGGER.info(f"Command is {args.command}, fetching images via GraphQL...")
        targets = fetch_images(required_rating)
    else:
        # queue 场景
        if image_paths:
            for idx, path in enumerate(image_paths):
                img_id = image_ids[idx] if idx < len(image_ids) else ""
                targets.append((img_id, path))
        else:
            _LOGGER.error(
                "No images provided in IMAGE_FUNNEL_IMAGE_PATHS for queue command."
            )
            sys.exit(1)

    if not targets:
        _LOGGER.error("No images found to process.")
        sys.exit(1)

    _LOGGER.info(f"Found {len(targets)} image(s) to process, command: {args.command}")

    has_errors = False
    success_count = 0

    for idx, (img_id, path) in enumerate(targets):
        if not os.path.exists(path):
            _LOGGER.error(f"File does not exist: {path}")
            has_errors = True
            continue

        _LOGGER.info(f"[{idx+1}/{len(targets)}] Processing image: {path}")

        prompt: Dict[str, Any]
        workflow: Dict[str, Any]

        try:
            with Image.open(path) as img:
                info: Any = img.info
                prompt_str: Optional[str] = info.get("prompt")
                workflow_str: Optional[str] = info.get("workflow")

                if not prompt_str:
                    _LOGGER.error(
                        f"This PNG image does not contain prompt metadata from ComfyUI: {path}"
                    )
                    has_errors = True
                    continue

                prompt = json.loads(prompt_str)
                workflow = json.loads(workflow_str) if workflow_str else {}
        except Exception as e:
            _LOGGER.error(f"Failed to read PNG properties: {e}")
            has_errors = True
            continue

        if args.command == "queue":
            any_success = False
            for q_idx in range(queue_count):
                if queue_count > 1:
                    _LOGGER.info(f"  -> Queueing run {q_idx+1}/{queue_count}")

                update_seeds(prompt, workflow)
                update_output_filenames(prompt, workflow)
                success: bool = send_to_comfyui(comfyui_url, prompt, workflow)
                if success:
                    any_success = True
                else:
                    has_errors = True

            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

        elif args.command in ["add", "remove"]:
            prompt_str_arg = " ".join(args.prompt)
            is_neg = args.neg

            target_node_id = get_target_clip_node(prompt, is_neg)
            if not target_node_id:
                _LOGGER.error("Failed to locate target CLIPTextEncode node in prompt.")
                has_errors = True
                continue

            node = prompt[target_node_id]

            # 必须从 workflow 获取含有 marker 的当前 text，作为定位和操作的基准。如果 workflow 中不存在该节点，则认为元数据损坏或不可读，报错并快速失败
            workflow_text = get_workflow_node_text(workflow, target_node_id)
            if workflow_text is None:
                _LOGGER.error(
                    f"Failed to locate target node {target_node_id} in workflow metadata."
                )
                has_errors = True
                continue

            prompt_text = node.setdefault("inputs", {}).setdefault("text", "")
            if not isinstance(prompt_text, str):
                prompt_text = ""

            start_marker = (
                os.getenv("HOOK_NEGATIVE_START_MARKER", "//#region hook-negative")
                if is_neg
                else os.getenv("HOOK_POSITIVE_START_MARKER", "//#region hook-positive")
            )
            end_marker = (
                os.getenv("HOOK_NEGATIVE_END_MARKER", "//#endregion hook-negative")
                if is_neg
                else os.getenv("HOOK_POSITIVE_END_MARKER", "//#endregion hook-positive")
            )

            hard = getattr(args, "hard", False)
            new_workflow_text, new_prompt_text = process_double_track(
                workflow_text,
                prompt_text,
                args.command,
                prompt_str_arg,
                start_marker,
                end_marker,
                args.raw,
                args.no_skip,
                hard,
            )

            if new_workflow_text is None or new_prompt_text is None:
                success_count += 1
                continue

            # 更新 API 端 (prompt) 结构数据，不包含 marker
            node["inputs"]["text"] = new_prompt_text

            # 同步更新 UI 端 (workflow) 结构数据，包含 marker
            update_workflow_node_text(workflow, target_node_id, new_workflow_text)

            any_success = False
            for q_idx in range(queue_count):
                update_seeds(prompt, workflow)
                update_output_filenames(prompt, workflow)
                success: bool = send_to_comfyui(comfyui_url, prompt, workflow)
                if success:
                    any_success = True
                else:
                    has_errors = True

            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

    print(f"processed {success_count}/{len(targets)} image(s) successfully.")

    if has_errors or success_count == 0:
        sys.exit(1)
    else:
        sys.exit(0)


def parse_args():
    parser = argparse.ArgumentParser(description="ComfyUI Funnel Hook Script")
    subparsers = parser.add_subparsers(
        dest="command", required=True, help="Sub-commands"
    )

    subparsers.add_parser("queue", help="Queue the image back to ComfyUI")

    add_parser = subparsers.add_parser(
        "add", help="Add prompt to image metadata and queue"
    )
    add_parser.add_argument(
        "--no-skip",
        "-S",
        action="store_true",
        help="Do not skip if prompt already exists",
    )
    add_parser.add_argument(
        "--neg", action="store_true", help="Add to negative prompt instead of positive"
    )
    add_parser.add_argument(
        "--raw", action="store_true", help="Add raw text without trailing comma"
    )
    add_parser.add_argument("prompt", nargs="+", help="The prompt text to add")

    remove_parser = subparsers.add_parser(
        "remove", help="Remove prompt from image metadata and queue"
    )
    remove_parser.add_argument(
        "--no-skip", "-S", action="store_true", help="Do not skip if prompt not found"
    )
    remove_parser.add_argument(
        "--neg",
        action="store_true",
        help="Remove from negative prompt instead of positive",
    )
    remove_parser.add_argument(
        "--raw",
        action="store_true",
        help="Remove raw text substring without line matching",
    )
    remove_parser.add_argument(
        "--hard",
        action="store_true",
        help="Directly delete the text instead of commenting it out",
    )
    remove_parser.add_argument("prompt", nargs="+", help="The prompt text to remove")

    return parser.parse_args()


if __name__ == "__main__":
    main()
