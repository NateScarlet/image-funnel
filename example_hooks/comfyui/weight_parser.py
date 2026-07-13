#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
权重表达式解析器，支持绝对值和基于当前值的相对表达式。
"""

import os
from typing import List, Optional


def eval_part(part: str, current: float) -> float:
    """
    计算权重表达式中的单个部分，'x' 代表当前值。
    支持: x, x-0.1, x+0.2 等形式。
    """
    part = part.strip()
    if part == "x":
        return current
    if part.startswith("x"):
        rest = part[1:]  # 如 '-0.1' 或 '+0.2'
        return current + float(rest)
    return float(part)


def generate_range(
    start: float, end: float, step: float, weight_str: str
) -> List[float]:
    """根据起止值和步长生成权重列表。"""
    if step == 0:
        raise ValueError("Step cannot be zero.")

    # 自动修正步长方向：如果步长符号与起止方向不一致，反转步长
    if (step > 0 and start > end) or (step < 0 and start < end):
        step = -step

    weights: List[float] = []
    curr = start
    while (step > 0 and curr <= end + (step * 0.01)) or (
        step < 0 and curr >= end + (step * 0.01)
    ):
        weights.append(round(curr, 4))
        curr += step
    if not weights:
        raise ValueError(f"No weights generated from range '{weight_str}'")
    return weights


def parse_weights(
    weight_str: str, current_value: Optional[float] = None
) -> List[float]:
    """
    解析权重值或范围值，支持：
    - 单一数值: 0.8
    - 范围带步长: 0.5:1.0:0.1
    - 范围默认步长: 0.5:1.0 (步进值默认由 HOOK_WEIGHT_STEP 环境变量指定，无指定时为 0.1)
    - 基于当前值的相对表达式: x-0.1, x-0.1:x+0.2:0.1, x-0.1:x+0.2
    - 上下对称浮动: +-0.3:0.1, +-0.3
    """
    # 处理 +- 对称浮动语法: +-0.3:0.1 或 +-0.3
    if weight_str.startswith("+-"):
        if current_value is None:
            raise ValueError(f"Relative weight '{weight_str}' requires a current value")
        rest = weight_str[2:]
        if ":" in rest:
            delta_str, step_str = rest.split(":", 1)
            delta = float(delta_str)
            step = float(step_str)
        else:
            delta = float(rest)
            default_step_str = os.getenv("HOOK_WEIGHT_STEP", "0.1")
            step = float(default_step_str)
        return generate_range(
            current_value - delta, current_value + delta, step, weight_str
        )

    # 处理 x 相对表达式
    if "x" in weight_str:
        if current_value is None:
            raise ValueError(f"Relative weight '{weight_str}' requires a current value")
        parts = weight_str.split(":")
        if len(parts) == 1:
            return [eval_part(parts[0], current_value)]
        elif len(parts) == 2:
            start = eval_part(parts[0], current_value)
            end = eval_part(parts[1], current_value)
            default_step_str = os.getenv("HOOK_WEIGHT_STEP", "0.1")
            step = float(default_step_str)
            return generate_range(start, end, step, weight_str)
        elif len(parts) == 3:
            start = eval_part(parts[0], current_value)
            end = eval_part(parts[1], current_value)
            step = float(parts[2])
            return generate_range(start, end, step, weight_str)
        else:
            raise ValueError(f"Invalid weight format: '{weight_str}'")

    # 原有绝对权重解析逻辑
    parts = weight_str.split(":")
    if len(parts) == 1:
        try:
            return [float(parts[0])]
        except ValueError:
            raise ValueError(f"Invalid weight format: '{weight_str}'")
    elif len(parts) in (2, 3):
        try:
            start = float(parts[0])
            end = float(parts[1])
            if len(parts) == 3:
                step = float(parts[2])
            else:
                default_step_str = os.getenv("HOOK_WEIGHT_STEP", "0.1")
                step = float(default_step_str)
        except ValueError:
            raise ValueError(f"Invalid weight range format: '{weight_str}'")
        return generate_range(start, end, step, weight_str)
    else:
        raise ValueError(f"Invalid weight format: '{weight_str}'")


def is_relative(weight_str: str) -> bool:
    """检测权重表达式是否为相对表达式（含 x 或 +- 前缀）。"""
    return "x" in weight_str or weight_str.startswith("+-")
