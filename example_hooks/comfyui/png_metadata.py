#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
PNG 内嵌 ComfyUI 元数据读取的共用模块。

供入列主流程（__main__.py）与复制增强脚本（copy_workflow.py）共同导入，
统一 prompt/workflow 元数据的读取与解析口径。
"""

import json
from typing import Any, Dict, Optional, Tuple

from PIL import Image


def _parse_json_object(raw: Optional[str]) -> Optional[Dict[str, Any]]:
    """解析 JSON 字符串并要求结果为对象：缺失或形状不符返回 None，JSON 非法抛出 ValueError"""
    if not raw:
        return None
    parsed = json.loads(raw)
    if not isinstance(parsed, dict):
        return None
    return parsed


def load_prompt_and_workflow(
    image_path: str,
) -> Tuple[Optional[Dict[str, Any]], Optional[Dict[str, Any]]]:
    """读取并解析 PNG 内嵌的 ComfyUI prompt/workflow 元数据。

    返回 (prompt, workflow)，某一侧元数据缺失或不是对象时对应元素为 None，
    缺失的语义由调用方决定（入列流程报错，复制增强视为不适用）。
    JSON 非法时抛出 ValueError，文件不可读时传播 OSError（快速失败，不静默兜底）。
    """
    with Image.open(image_path) as img:
        info = img.info
        prompt: Optional[Dict[str, Any]] = _parse_json_object(info.get("prompt"))
        workflow: Optional[Dict[str, Any]] = _parse_json_object(info.get("workflow"))
    return prompt, workflow
