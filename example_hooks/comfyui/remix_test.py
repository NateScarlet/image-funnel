# -*- coding: utf-8 -*-
# 只允许使用项目测试脚本运行测试

import unittest
from typing import Any, Dict, List, cast

from .__main__ import get_parser
from .remix import RemixRequest, VariantMatrix, build_matrix, from_args
from .variant_engine import CfgStep
from .workflow_prompt_pair import WorkflowPromptPair


def _parse(argv: List[str]) -> RemixRequest:
    """按真实 CLI 解析 /remix 旗标再归类，确保测试覆盖到 argparse 与归类两层。"""
    return from_args(get_parser().parse_args(["remix", *argv]))


def _workflow_and_prompt() -> tuple[Dict[str, Any], Dict[str, Any]]:
    workflow: Dict[str, Any] = {
        "nodes": [
            {
                "id": "1",
                "type": "CLIPTextEncode",
                "widgets_values": [
                    "// #region positive\nmasterpiece,\nbest quality,\n// #endregion"
                ],
            },
            {
                "id": "2",
                "type": "CLIPTextEncode",
                "widgets_values": ["// #region negative\nworst quality\n// #endregion"],
            },
            {"id": "3", "type": "EmptyLatentImage", "widgets_values": [768, 1344, 1]},
            {
                "id": "4",
                "type": "KSampler",
                "widgets_values": [12345, "fixed", 20, 8.0],
            },
        ]
    }
    prompt: Dict[str, Any] = {
        "1": {
            "class_type": "CLIPTextEncode",
            "inputs": {"text": "masterpiece,\nbest quality,"},
        },
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": "worst quality"}},
        "3": {
            "class_type": "EmptyLatentImage",
            "inputs": {"width": 768, "height": 1344, "batch_size": 1},
        },
        "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
    }
    return workflow, prompt


def _steps(matrix: VariantMatrix) -> List[Any]:
    """按变体顺序取出各变体唯一维度实际选中的档位（无变化的维度不参与）。"""
    return [pairs[0][1] for pairs in matrix.iter_variants() if pairs]


class TestRemixRequestParsing(unittest.TestCase):
    def test_add_single_tag_is_plain_add(self) -> None:
        request = _parse(["--add", "a"])
        self.assertEqual(request.add_tags, ("a",))
        self.assertEqual(request.prompt_weights, ())

    def test_add_multiple_tags_accumulate(self) -> None:
        both_forms = [
            _parse(["--add", "a", "b"]),
            _parse(["--add", "a", "--add", "b"]),
        ]
        for request in both_forms:
            self.assertEqual(request.add_tags, ("a", "b"))
            self.assertEqual(request.prompt_weights, ())

    def test_remove_multiple_tags_accumulate(self) -> None:
        request = _parse(["--remove", "a", "b"])
        self.assertEqual(request.remove_tags, ("a", "b"))

    def test_add_single_tag_with_weight_is_adjust(self) -> None:
        request = _parse(["--add", "masterpiece", "0.8"])
        self.assertEqual(request.add_tags, ())
        self.assertEqual(len(request.prompt_weights), 1)
        self.assertEqual(request.prompt_weights[0].tag, "masterpiece")
        self.assertEqual(request.prompt_weights[0].weight_expr, "0.8")

    def test_add_quoted_multi_word_tag_with_weight(self) -> None:
        """含空格的 tag 由引号聚合为一个值，权重仍只作用于该单个 tag。"""
        request = _parse(["--add", "long hair", "0.8"])
        self.assertEqual(request.add_tags, ())
        self.assertEqual(request.prompt_weights[0].tag, "long hair")
        self.assertEqual(request.prompt_weights[0].weight_expr, "0.8")

    def test_add_multi_word_tag_without_weight(self) -> None:
        request = _parse(["--add", "long hair"])
        self.assertEqual(request.add_tags, ("long hair",))

    def test_add_multiple_tags_with_trailing_weight_errors(self) -> None:
        """多值加权重没有可依的语义，必须报错而不是猜测。"""
        with self.assertRaises(ValueError):
            _parse(["--add", "a", "b", "c", "0.8"])

    def test_lora_requires_name_and_weight(self) -> None:
        request = _parse(["--lora", "my_lora", "0.8"])
        self.assertEqual(len(request.loras), 1)
        self.assertEqual(request.loras[0].name, "my_lora")

    def test_lora_extra_value_errors(self) -> None:
        with self.assertRaises(ValueError):
            _parse(["--lora", "a", "b", "0.8"])

    def test_lora_missing_weight_errors(self) -> None:
        with self.assertRaises(ValueError):
            _parse(["--lora", "a"])

    def test_lora_non_weight_value_errors(self) -> None:
        with self.assertRaises(ValueError):
            _parse(["--lora", "a", "not_a_weight"])

    def test_cfg_and_aspect_collect_repeated_values(self) -> None:
        request = _parse(["--cfg", "5", "--cfg", "7", "--aspect", "16:9"])
        self.assertEqual(request.cfgs, ("5", "7"))
        self.assertEqual(request.aspects, ("16:9",))

    def test_no_flags_has_no_edits(self) -> None:
        self.assertFalse(_parse([]).has_edits)

    def test_every_edit_flag_marks_edits(self) -> None:
        for argv in (
            ["--add", "a"],
            ["--remove", "a"],
            ["--remove-again"],
            ["--add", "a", "0.8"],
            ["--lora", "a", "0.5"],
            ["--cfg", "5"],
            ["--aspect", "16:9"],
        ):
            with self.subTest(argv=argv):
                self.assertTrue(_parse(argv).has_edits)


class TestRemixVariantMatrix(unittest.TestCase):
    def _matrix(self, argv: List[str]) -> VariantMatrix:
        workflow, prompt = _workflow_and_prompt()
        return build_matrix(_parse(argv), WorkflowPromptPair(workflow, prompt))

    def test_add_alone_yields_one_variant(self) -> None:
        self.assertEqual(len(self._matrix(["--add", "a"])), 1)

    def test_weight_expression_yields_one_variant_per_step(self) -> None:
        matrix = self._matrix(["--add", "masterpiece", "0.5:0.7:0.1"])
        self.assertEqual(len(matrix), 3)
        self.assertEqual(_steps(matrix), [0.5, 0.6, 0.7])

    def test_weight_range_counts_every_step(self) -> None:
        """步长区间的档数由范围本身决定：0.5:1.0:0.1 共 6 档。"""
        self.assertEqual(len(self._matrix(["--add", "masterpiece", "0.5:1.0:0.1"])), 6)

    def test_aspect_repeated_values_yield_variants(self) -> None:
        self.assertEqual(len(self._matrix(["--aspect", "16:9", "--aspect", "4:3"])), 2)

    def test_aspect_repeated_duplicates_dedupe(self) -> None:
        self.assertEqual(len(self._matrix(["--aspect", "16:9", "--aspect", "16:9"])), 1)

    def test_aspect_expression_and_multiple_values_take_union(self) -> None:
        """`--aspect +3`（3 档）与 `--aspect 4:3`（1 档）取并集 → 4 档。"""
        self.assertEqual(len(self._matrix(["--aspect", "+3", "--aspect", "4:3"])), 4)

    def test_cfg_repeated_values_yield_variants(self) -> None:
        self.assertEqual(len(self._matrix(["--cfg", "5", "--cfg", "7"])), 2)

    def test_cross_flag_cartesian_product(self) -> None:
        """可累加编辑不产多变体：--add a 固定为 1 维，× 2 档长宽比 = 2 变体。"""
        self.assertEqual(
            len(self._matrix(["--add", "a", "--aspect", "16:9", "--aspect", "4:3"])), 2
        )

    def test_cfg_range_times_aspect_values(self) -> None:
        """`--cfg 5:7:1`（3 档）× 2 档长宽比 = 6 变体。"""
        self.assertEqual(
            len(
                self._matrix(["--cfg", "5:7:1", "--aspect", "16:9", "--aspect", "4:3"])
            ),
            6,
        )

    def test_relative_expression_does_not_drift_across_variants(self) -> None:
        """相对表达式逐档都基于基线当前值，不随变体顺序漂移。"""
        matrix = self._matrix(["--cfg", "x+1", "--cfg", "x+2"])
        self.assertEqual(
            [
                dict(cast(CfgStep, pairs[0][1]))
                for pairs in matrix.iter_variants()
                if pairs
            ],
            [{"4": 9.0}, {"4": 10.0}],
        )

    def test_dimension_without_variants_is_identity(self) -> None:
        """无法展开的维度（如相对表达式解析不出当前值）按恒等处理，不使整个积归零。"""
        matrix = self._matrix(["--add", "nonexistent_tag", "x+0.1", "--cfg", "5"])
        self.assertEqual(len(matrix), 1)
        self.assertEqual(_steps(matrix), [(("4", 5.0),)])

    def test_cfg_current_value_is_identity_and_filtered(self) -> None:
        """目标值等于当前值的档是恒等，不单独产出变体。"""
        matrix = self._matrix(["--cfg", "8"])
        self.assertEqual(len(matrix), 1)
        self.assertEqual(_steps(matrix), [])


if __name__ == "__main__":
    unittest.main()
