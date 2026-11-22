# 只允许使用项目测试脚本运行测试
# pyright: reportArgumentType=false

"""补全位置 seam 的不变量测试。

位置规则收敛在 completion_position 深 module 后，测试直接对小输入表断言
CompletionPosition 的六个事实，不再依赖 autocomplete() 端到端样例。
"""

import argparse
import unittest
from dataclasses import FrozenInstanceError, replace
from typing import Any, List, Optional, Set

from .completion_position import (
    ADD_ACTION,
    CFG_ACTION,
    NO_ACTION,
    REMOVE_ACTION,
    CompletionPosition,
    compute_completion_position,
)


def _parsed_args(**overrides: Any) -> argparse.Namespace:
    """构造只带区域/节点目标的解析结果（位置判定只关心这两个字段）。"""
    values: dict[str, Any] = {"region": None, "node": None}
    values.update(overrides)
    return argparse.Namespace(**values)


def _compute(
    cwords: List[str],
    *,
    target_command: str = "remix",
    query: str = "",
    prev_word: str = "",
    option_with_args: Optional[Set[str]] = None,
    parsed_args: Optional[argparse.Namespace] = None,
) -> CompletionPosition:
    """以默认位置信号调用唯一 seam，用例只覆盖与当前断言相关的输入。"""
    return compute_completion_position(
        target_command=target_command,
        query=query,
        prev_word=prev_word,
        cleaned_cwords=cwords,
        option_with_args=option_with_args or set(),
        parsed_args=parsed_args,
    )


class TestCompletionPositionAction(unittest.TestCase):
    """动作由最后一个编辑旗标决定，取值选项的参数位不算旗标。"""

    def test_no_flag_means_no_action(self):
        self.assertEqual(_compute(["/remix"]).action, NO_ACTION)

    def test_last_edit_flag_wins(self):
        self.assertEqual(_compute(["/remix", "--add"]).action, ADD_ACTION)
        self.assertEqual(
            _compute(["/remix", "--add", "blue", "--remove", "x"]).action,
            REMOVE_ACTION,
        )

    def test_option_value_is_skipped_instead_of_treated_as_flag(self):
        """非动作旗标的取值选项（如 `-j`）后面的 `--add` 是它的值，不能再切动作。"""
        self.assertEqual(
            _compute(
                ["/remix", "--cfg", "-j", "--add"], option_with_args={"-j"}
            ).action,
            CFG_ACTION,
        )

    def test_end_of_options_token_is_not_an_action(self):
        """`--` 本身不切换动作（其后的旗标仍按既有判定参与动作扫描）。"""
        self.assertEqual(_compute(["/remix", "--", "--add"]).action, ADD_ACTION)


class TestCompletionPositionActionTags(unittest.TestCase):
    """标签取当前动作的位置值，按旗标出现次数分组后扁平化。"""

    def test_tags_from_current_action_only(self):
        self.assertEqual(
            _compute(["/remix", "--add", "blue", "eyes"]).action_tags,
            ("blue", "eyes"),
        )
        self.assertEqual(
            _compute(["/remix", "--add", "blue", "--remove", "x"]).action_tags,
            ("x",),
        )

    def test_comma_separated_value_is_split(self):
        self.assertEqual(_compute(["/remix", "--add", "a,b"]).action_tags, ("a", "b"))


class TestCompletionPositionEditingModifierValue(unittest.TestCase):
    """当前词是否落在取值型修饰选项的参数位。"""

    def test_modifier_option_value_is_editing(self):
        self.assertTrue(
            _compute(
                ["/remix", "--node"],
                prev_word="--node",
                option_with_args={"--node"},
            ).editing_modifier_value,
        )

    def test_edit_flag_value_is_not_editing(self):
        """编辑旗标的值是待补全的标签，不能当成修饰选项参数。"""
        self.assertFalse(
            _compute(
                ["/remix", "--add"],
                prev_word="--add",
                option_with_args={"--add", "--region"},
            ).editing_modifier_value,
        )

    def test_unknown_prev_word_falls_back_to_editing(self):
        """前一个词不在已完成词列表里时按「正在写值」处理。"""
        self.assertTrue(
            _compute(
                ["/remix"], prev_word="--region", option_with_args={"--region"}
            ).editing_modifier_value,
        )

    def test_end_of_options_before_modifier_is_not_editing(self):
        """`--` 之后的选项形 token 是位置值，不是仍在等待参数的修饰选项。"""
        self.assertFalse(
            _compute(
                ["/remix", "--", "--region"],
                prev_word="--region",
                option_with_args={"--region"},
            ).editing_modifier_value,
        )


class TestCompletionPositionExplicitTarget(unittest.TestCase):
    """显式目标优先看解析结果，解析失败时回退扫词。"""

    def test_parsed_target_wins(self):
        self.assertTrue(
            _compute(
                ["/remix"], parsed_args=_parsed_args(region=["positive"])
            ).has_explicit_target
        )
        self.assertFalse(
            _compute(["/remix"], parsed_args=_parsed_args()).has_explicit_target
        )

    def test_fallback_scan_when_parse_failed(self):
        self.assertTrue(
            _compute(["/remix", "--region", "positive"]).has_explicit_target
        )
        self.assertFalse(_compute(["/remix", "--add", "blue"]).has_explicit_target)

    def test_fallback_scan_includes_positional_values(self):
        """沿用既有回退语义：扫描全部已完成词，`--` 之后的目标词也算。"""
        self.assertTrue(_compute(["/remix", "--", "--node"]).has_explicit_target)


class TestCompletionPositionAddFlag(unittest.TestCase):
    """`--add` 是否出现过：扫描止于 `--`。"""

    def test_add_flag_presence(self):
        self.assertTrue(_compute(["/remix", "--add", "blue"]).has_add_flag)
        self.assertTrue(
            _compute(["/remix", "--add", "blue", "--remove", "x"]).has_add_flag
        )
        self.assertFalse(_compute(["/remix", "--remove", "x"]).has_add_flag)

    def test_add_after_end_of_options_is_not_a_flag(self):
        self.assertFalse(_compute(["/remix", "--", "--add"]).has_add_flag)


class TestCompletionPositionTargetOptionPosition(unittest.TestCase):
    """是否处于可写 `--region`/`--node` 的空位：纯位置判定，不掺入数据加载状态。"""

    def test_empty_remix_position(self):
        self.assertTrue(_compute(["/remix"]).in_target_option_position)

    def test_not_in_position_after_edit_action(self):
        self.assertFalse(_compute(["/remix", "--add"]).in_target_option_position)

    def test_not_in_position_with_query(self):
        self.assertFalse(_compute(["/remix"], query="g").in_target_option_position)

    def test_not_in_position_in_modifier_value(self):
        self.assertFalse(
            _compute(
                ["/remix", "-j"], prev_word="-j", option_with_args={"-j"}
            ).in_target_option_position,
        )

    def test_not_in_position_with_explicit_target(self):
        self.assertFalse(
            _compute(
                ["/remix"], parsed_args=_parsed_args(node=["5"])
            ).in_target_option_position,
        )
        self.assertFalse(
            _compute(["/remix", "--region", "positive"]).in_target_option_position
        )

    def test_not_in_position_outside_remix(self):
        self.assertFalse(
            _compute(["/queue"], target_command="queue").in_target_option_position,
        )

    def test_not_in_position_after_end_of_options(self):
        self.assertFalse(
            _compute(["/remix", "--", "--region"]).in_target_option_position
        )


class TestCompletionPositionValueObject(unittest.TestCase):
    """位置事实对象不可变，且可由已有事实派生出测试夹具。"""

    def test_frozen(self):
        position = _compute(["/remix"])
        with self.assertRaises(FrozenInstanceError):
            position.action = ADD_ACTION  # pyright: ignore[reportAttributeAccessIssue]

    def test_replace_for_artificial_fixture(self):
        """测试可用 replace 构造「动作与位置事实不一致」的边界夹具。"""
        position = _compute(["/remix", "--add", "blue", "--remove", "x"])
        fixture = replace(
            position,
            action=NO_ACTION,
            in_target_option_position=True,
        )
        self.assertTrue(fixture.has_add_flag)
        self.assertEqual(fixture.action, NO_ACTION)
