#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""补全位置判定：把「光标位置信号 → 一组位置事实」收敛在单一纯计算 module 中。

入口、补全上下文与所有建议提供者共享同一份 CompletionPosition。动作旗标、取值选项跳过、
`--` 结束标记、显式目标回退、`--add` 扫描范围等规则只在此处表达一次。本 module 零 I/O、
零网络、不构建依赖，所需输入全部由调用方传入。

位置事实只描述「光标此刻站在哪一类位置」，是否加载了工作流属于数据状态，由消费方
（目标选项 provider）自行检查，不混入位置判定。
"""

import argparse
from dataclasses import dataclass
from typing import Dict, List, Optional, Sequence, Set, Tuple

# 编辑动作：位置事实 action 的取值
NO_ACTION = ""
ADD_ACTION = "add"
REMOVE_ACTION = "remove"
LORA_ACTION = "lora"
CFG_ACTION = "cfg"
ASPECT_ACTION = "aspect"
BRANCH_ACTION = "branch"

# 每个旗标把补全位置切到哪个动作；`--cfg` / `--aspect` 只取一个值，其余多值
_ACTION_FLAGS: Dict[str, str] = {
    "--add": ADD_ACTION,
    "--remove": REMOVE_ACTION,
    "--lora": LORA_ACTION,
    "--cfg": CFG_ACTION,
    "--aspect": ASPECT_ACTION,
    "--branch": BRANCH_ACTION,
}


@dataclass(frozen=True)
class CompletionPosition:
    """一次补全请求在当前光标处判定的一组位置事实（不可变）。"""

    action: str
    action_tags: Tuple[str, ...]
    in_target_option_position: bool
    has_explicit_target: bool
    has_add_flag: bool
    editing_modifier_value: bool


# #region 位置规则（模块私有实现）


def _classify_action(cwords: Sequence[str], option_with_args: Set[str]) -> str:
    """扫描最后一个编辑旗标定位当前动作；取值选项的参数值跳过。"""
    action: str = NO_ACTION
    expect_value = False
    for word in cwords[1:]:
        if expect_value:
            expect_value = False
            continue
        if word == "--":
            continue
        if word in _ACTION_FLAGS:
            action = _ACTION_FLAGS[word]
        elif word in option_with_args:
            expect_value = True
    return action


def action_values(cwords: Sequence[str], action: str) -> List[List[str]]:
    """按旗标出现次数分组取出给定动作的位置值（可再按逗号拆分）。

    同指令 `--add` 的标签补全（分支名候选）也复用这里：那不是位置判定，而是从
    已完成词里提取某动作的值，因此与本 module 的判定同处一份实现。
    """
    flag = next((f for f, a in _ACTION_FLAGS.items() if a == action), None)
    if flag is None:
        return []

    groups: List[List[str]] = []
    collecting: Optional[List[str]] = None
    for word in cwords[1:]:
        if word.startswith("-"):
            collecting = [] if word == flag else None
            if collecting is not None:
                groups.append(collecting)
            continue
        if collecting is None:
            continue
        cleaned = word.strip().strip(",").strip('"').strip("'").strip()
        for part in cleaned.split(","):
            part_cleaned = part.strip()
            if part_cleaned:
                collecting.append(part_cleaned)
    return groups


def _editing_modifier_value(
    prev_word: str, option_with_args: Set[str], cwords: Sequence[str]
) -> bool:
    """当前词是否落在取值型修饰选项（--region / --node）的参数位。"""
    if prev_word not in option_with_args or prev_word in _ACTION_FLAGS:
        return False
    try:
        return "--" not in cwords or cwords.index("--") > cwords.index(prev_word)
    except ValueError:
        # 前一个词不在已完成词列表里（如刚从编辑器占位符过渡），按正在写值处理
        return True


def _has_explicit_target(
    cwords: Sequence[str], parsed_args: Optional[argparse.Namespace]
) -> bool:
    """显式指定插入目标：解析结果优先，解析失败时回退扫描已完成词。"""
    if parsed_args is not None:
        return bool(getattr(parsed_args, "region", None)) or bool(
            getattr(parsed_args, "node", None)
        )
    return any(w in ("--region", "--node") for w in cwords[1:])


def _has_add_flag(cwords: Sequence[str]) -> bool:
    """命令行是否已出现 `--add`；`--` 之后是位置值，扫描到此为止。"""
    for word in cwords[1:]:
        if word == "--":
            return False
        if word == "--add":
            return True
    return False


def _in_target_option_position(
    target_command: str,
    action: str,
    cwords: Sequence[str],
    query: str,
    editing_modifier_value: bool,
    has_explicit_target: bool,
) -> bool:
    """是否处于可写 `--region` / `--node` 的 `/remix ` 空位。"""
    if target_command != "remix" or action != NO_ACTION:
        return False
    if "--" in cwords:
        return False
    if query.strip():
        return False
    if editing_modifier_value:
        return False
    if has_explicit_target:
        return False
    return True


# #endregion


def compute_completion_position(
    *,
    target_command: str,
    query: str,
    prev_word: str,
    cleaned_cwords: Sequence[str],
    option_with_args: Set[str],
    parsed_args: Optional[argparse.Namespace],
) -> CompletionPosition:
    """位置判定的唯一 seam：输入本次请求的位置信号，一次产出全部位置事实。

    调用方传入已完成词（已去掉编辑器占位符）、解析器收集到的取值选项集合，以及
    已解析的参数；本函数只读这些输入，不做任何 I/O 或依赖构建。
    """
    action = _classify_action(cleaned_cwords, option_with_args)
    editing_modifier_value = _editing_modifier_value(
        prev_word, option_with_args, cleaned_cwords
    )
    has_explicit_target = _has_explicit_target(cleaned_cwords, parsed_args)

    return CompletionPosition(
        action=action,
        action_tags=tuple(
            tag for group in action_values(cleaned_cwords, action) for tag in group
        ),
        in_target_option_position=_in_target_option_position(
            target_command,
            action,
            cleaned_cwords,
            query,
            editing_modifier_value,
            has_explicit_target,
        ),
        has_explicit_target=has_explicit_target,
        has_add_flag=_has_add_flag(cleaned_cwords),
        editing_modifier_value=editing_modifier_value,
    )
