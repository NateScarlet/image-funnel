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

_LOGGER = logging.getLogger(__name__)


# 捕获 PIL 导入错误并给出清晰提示
try:
    from PIL import Image
except ImportError:
    print("Error: Missing Pillow library. Please install it in your Python environment to handle image metadata:", file=sys.stderr)
    print("      pip install Pillow", file=sys.stderr)
    sys.exit(1)

KNOWN_PRIMITIVE_TYPES = {"PrimitiveInt", "PrimitiveFloat", "PrimitiveString", "PrimitiveBoolean"}
KNOWN_SWITCH_TYPES = {"Any Switch (rgthree)", "ComfySwitchNode"}

def find_terminal_input(prompt: Dict[str, Any], node_id: str, input_key: str) -> Tuple[str, str]:
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
                            res: Tuple[str, str] = find_terminal_input(prompt, target_node_id, ik)
                            if res:
                                return res
                elif target_class == "Any Switch (rgthree)":
                    # Any Switch (rgthree) 的输入中转端口以 any_ 开头
                    for ik, iv in target_node.get("inputs", {}).items():
                        if ik.startswith("any_"):
                            res: Tuple[str, str] = find_terminal_input(prompt, target_node_id, ik)
                            if res:
                                return res
                                
            # 3. 针对其他可能中转信号的自定义节点，我们继续追溯其列表型端口
            else:
                for ik, iv in target_node.get("inputs", {}).items():
                    iv_list = cast(List[Any], iv) if isinstance(iv, list) else []
                    if len(iv_list) == 2:
                        res: Tuple[str, str] = find_terminal_input(prompt, target_node_id, ik)
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
        candidate_nodes.append((
            node,
            str(node.get("id")),
            False,
            None,
            node.get("type")
        ))
        
    # 2. 收集各子图定义内部的节点
    subgraphs: List[Dict[str, Any]] = workflow.get("definitions", {}).get("subgraphs", [])
    for subgraph in subgraphs:
        subgraph_id = subgraph.get("id")
        for node in subgraph.get("nodes", []):
            candidate_nodes.append((
                node,
                str(node.get("id")),
                True,
                subgraph_id,
                f"{subgraph.get('name', 'Subgraph')}:{node.get('type')}"
            ))

    date_patterns: Dict[str, Tuple[str, str, bool, Optional[str]]] = {}  # node_id_str -> (py_fmt, regex_pattern, is_subgraph, subgraph_id)
    
    for node, node_id_str, is_subgraph, subgraph_id, node_type_for_log in candidate_nodes:
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
                    py_fmt, regex_pattern = convert_comfy_date_format_to_python(comfy_fmt)
                    date_patterns[node_id_str] = (py_fmt, regex_pattern, is_subgraph, subgraph_id)
                    _LOGGER.info(f"Workflow node {node_id_str} ({node_type_for_log}) uses date template: {comfy_fmt}")
                    break
                    
    if not date_patterns:
        return
        
    now = datetime.datetime.now()
    
    for node_id_str, (py_fmt, regex_pattern, is_subgraph, subgraph_id) in date_patterns.items():
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
                    src_node_id, src_key = find_terminal_input(prompt, api_node_id, "filename_prefix")
                    if src_node_id in prompt:
                        src_inputs = prompt[src_node_id].setdefault("inputs", {})
                        prefix_val: Any = src_inputs.get(src_key)
                        if isinstance(prefix_val, str):
                            new_date_str: str = now.strftime(py_fmt)
                            if re.search(regex_pattern, prefix_val):
                                new_prefix: str = re.sub(regex_pattern, new_date_str, prefix_val)
                                src_inputs[src_key] = new_prefix
                                _LOGGER.info(f"Prompt node {src_node_id} (key {src_key}) filename_prefix updated: {prefix_val} -> {new_prefix}")

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
        candidate_nodes.append((
            node,
            str(node.get("id")),
            False,
            None,
            node.get("type")
        ))
        
    # 2. 收集各子图定义内部的节点
    subgraphs: List[Dict[str, Any]] = workflow.get("definitions", {}).get("subgraphs", [])
    for subgraph in subgraphs:
        subgraph_id = subgraph.get("id")
        for node in subgraph.get("nodes", []):
            candidate_nodes.append((
                node,
                str(node.get("id")),
                True,
                subgraph_id,
                f"{subgraph.get('name', 'Subgraph')}:{node.get('type')}"
            ))

    # 遍历所有候选节点，定位包含种子的 Widget 并更新
    for node, node_id_str, is_subgraph, subgraph_id, node_type_for_log in candidate_nodes:
        widgets_values_raw: Any = node.get("widgets_values")
        if not isinstance(widgets_values_raw, list):
            continue
        widgets_values = cast(List[Any], widgets_values_raw)
            
        # 遍历 widgets_values 数组，寻找满足 [整数值, 'fixed'/'increment'/'decrement'/'randomize'] 邻接特征的项
        for idx in range(len(widgets_values) - 1):
            val: Any = widgets_values[idx]
            val_next: Any = widgets_values[idx + 1]
            if isinstance(val, int) and val_next in ["fixed", "increment", "decrement", "randomize"]:
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
                _LOGGER.info(f"Workflow node {node_id_str} ({node_type_for_log}) seed updated: {old_seed} -> {new_seed} (strategy: {strategy})")
                
                # B. 映射到 API (prompt) 端节点 ID 列表。对于子图节点，需根据其所有顶层实例生成前缀 ID 列表。
                api_node_ids: List[str] = []
                if not is_subgraph:
                    api_node_ids = [node_id_str]
                else:
                    for parent_node in workflow.get("nodes", []):
                        if parent_node.get("type") == subgraph_id:
                            api_node_ids.append(f"{parent_node.get('id')}:{node_id_str}")
                
                # 同步修改 API 端 (prompt) 结构中对应的连接源头。
                for api_node_id in api_node_ids:
                    if api_node_id in prompt:
                        inputs: Dict[str, Any] = prompt[api_node_id].get("inputs", {})
                        # 遍历此节点在 prompt 中的所有 inputs，寻找和当前种子关联的端口并追溯修改其源头值
                        for ik in list(inputs.keys()):
                            src_node_id: str
                            src_key: str
                            src_node_id, src_key = find_terminal_input(prompt, api_node_id, ik)
                            if src_node_id in prompt:
                                src_node: Dict[str, Any] = prompt[src_node_id]
                                src_inputs: Dict[str, Any] = src_node.get("inputs", {})
                                current_val: Any = src_inputs.get(src_key)
                                
                                # 校验当前值是否等于 old_seed 且满足种子标识或 Primitive 属性
                                is_primitive = src_node.get("class_type") in KNOWN_PRIMITIVE_TYPES
                                if (current_val == old_seed or str(current_val) == str(old_seed)) and \
                                   ("seed" in ik or "seed" in src_key or is_primitive):
                                    src_inputs[src_key] = new_seed
                                    _LOGGER.info(f"  -> Prompt structure sync: updated source node {src_node_id} key {src_key} = {new_seed}")
                                    
    return modified_count

def send_to_comfyui(comfyui_url: str, prompt: Dict[str, Any], workflow: Dict[str, Any]) -> bool:
    """
    提交工作流到 ComfyUI 的 /prompt 接口。
    """
    client_id: str = str(uuid.uuid4())
    payload: Dict[str, Any] = {
        "prompt": prompt,
        "client_id": client_id,
        "extra_data": {
            "extra_pnginfo": {
                "workflow": workflow
            }
        }
    }
    data: bytes = json.dumps(payload).encode('utf-8')
    req: urllib.request.Request = urllib.request.Request(
        f"{comfyui_url}/prompt",
        data=data,
        headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req) as f:
            res: Dict[str, Any] = json.loads(f.read().decode('utf-8'))
            prompt_id: Any = res.get("prompt_id")
            _LOGGER.info(f"Workflow successfully queued to ComfyUI, prompt_id: {prompt_id}")
            return True
    except Exception as e:
        _LOGGER.error(f"Failed to submit to ComfyUI: {e}")
        return False

def update_image_label(graphql_url: str, token: str, image_id: str, label: str) -> bool:
    """
    通过 GraphQL 更新图片颜色标签。
    """
    query: str = """
    mutation UpdateImageMetadata($input: UpdateImageMetadataInput!) {
      updateImageMetadata(input: $input) {
        id
      }
    }
    """
    payload: Dict[str, Any] = {
        "query": query,
        "variables": {
            "input": {
                "id": image_id,
                "label": label
            }
        }
    }
    data: bytes = json.dumps(payload).encode('utf-8')
    req: urllib.request.Request = urllib.request.Request(
        graphql_url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}"
        }
    )
    try:
        with urllib.request.urlopen(req) as f:
            res: Dict[str, Any] = json.loads(f.read().decode('utf-8'))
            if "errors" in res:
                _LOGGER.error(f"Failed to update image {image_id} label via GraphQL: {res['errors']}")
                return False
            else:
                _LOGGER.info(f"Image {image_id} color label successfully updated to {label}")
                return True
    except Exception as e:
        _LOGGER.error(f"Exception occurred when calling GraphQL: {e}")
        return False

def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S"
    )

    image_paths_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "")
    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    graphql_url: str = os.getenv("IMAGE_FUNNEL_GRAPHQL_URL", "")
    token: str = os.getenv("IMAGE_FUNNEL_TOKEN", "")
    comfyui_url: str = os.getenv("COMFYUI_URL", "http://127.0.0.1:8188")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    # 支持通过环境变量配置每张图片提交给 ComfyUI 的生成次数（默认 1 次）
    queue_count_env = os.getenv("HOOK_QUEUE_COUNT", "1")
    try:
        queue_count = int(queue_count_env)
        if queue_count < 1:
            queue_count = 1
    except ValueError:
        queue_count = 1

    # 错误追踪状态
    has_errors = False
    success_count = 0

    if not image_paths_str:
        _LOGGER.error("IMAGE_FUNNEL_IMAGE_PATHS environment variable not set or empty.")
        sys.exit(1)

    try:
        image_paths: List[str] = json.loads(image_paths_str)
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_PATHS as JSON: {e}")
        sys.exit(1)

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS as JSON: {e}")
        sys.exit(1)

    if not image_paths:
        _LOGGER.error("No image paths found to process.")
        sys.exit(1)

    _LOGGER.info(f"Received {len(image_paths)} image(s) to process")

    for idx, path in enumerate(image_paths):
        # 1. 检查图片路径是否存在
        if not os.path.exists(path):
            _LOGGER.error(f"File does not exist: {path}")
            has_errors = True
            continue

        _LOGGER.info(f"[{idx+1}/{len(image_paths)}] Processing image: {path}")
        
        prompt: Dict[str, Any]
        workflow: Dict[str, Any]
        # 2. 读取图片并解析 ComfyUI 元数据
        try:
            with Image.open(path) as img:
                info: Any = img.info
                prompt_str: Optional[str] = info.get("prompt")
                workflow_str: Optional[str] = info.get("workflow")

                if not prompt_str:
                    _LOGGER.error(f"This PNG image does not contain prompt metadata from ComfyUI: {path}")
                    has_errors = True
                    continue

                prompt = json.loads(prompt_str)
                workflow = json.loads(workflow_str) if workflow_str else {}
        except Exception as e:
            _LOGGER.error(f"Failed to read PNG properties: {e}")
            has_errors = True
            continue

        # 3. 循环提交 ComfyUI
        any_success = False
        for q_idx in range(queue_count):
            if queue_count > 1:
                _LOGGER.info(f"  -> Queueing run {q_idx+1}/{queue_count}")

            # A. 修改种子（在 prompt 结构与 workflow 结构中同步修改并尝试保持在 workflow widget 里的状态数值）
            modified_count: int = update_seeds(prompt, workflow)
            if modified_count == 0 and q_idx == 0:
                _LOGGER.info("No control nodes containing random seeds found in workflow, submitting as is.")

            # B. 动态更新输出文件名中的占位符日期为当前时间
            update_output_filenames(prompt, workflow)

            # C. 提交给 ComfyUI
            success: bool = send_to_comfyui(comfyui_url, prompt, workflow)
            if success:
                any_success = True
            else:
                _LOGGER.error(f"Failed to submit to ComfyUI for run {q_idx+1}/{queue_count}")
                has_errors = True
        
        # 4. 成功后，更新该图片颜色标签以防止重复处理
        if any_success:
            success_count += 1
            if label_to_set and idx < len(image_ids) and graphql_url and token:
                image_id: str = image_ids[idx]
                success_label_update: bool = update_image_label(graphql_url, token, image_id, label_to_set)
                if not success_label_update:
                    has_errors = True

    print(f"processed {success_count}/{len(image_paths)} image(s) successfully.")

    if has_errors or success_count == 0:
        sys.exit(1)
    else:
        sys.exit(0)

if __name__ == "__main__":
    main()
