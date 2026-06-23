import unittest
import os

# 允许直接在此目录下运行，也可以在项目根目录下运行
import sys

current_dir = os.path.dirname(os.path.abspath(__file__))
if current_dir not in sys.path:
    sys.path.append(current_dir)

from weight_parser import parse_weights, eval_part, generate_range, is_relative


class TestWeightParser(unittest.TestCase):
    """权重解析器单元测试，覆盖绝对值和相对表达式语法。"""

    # --- eval_part ---

    def test_eval_part_plain_number(self):
        self.assertEqual(eval_part("0.5", 1.0), 0.5)
        self.assertEqual(eval_part("-0.3", 2.0), -0.3)

    def test_eval_part_x_only(self):
        self.assertEqual(eval_part("x", 1.0), 1.0)
        self.assertEqual(eval_part("x", 0.0), 0.0)
        self.assertEqual(eval_part("x", -0.5), -0.5)

    def test_eval_part_x_plus(self):
        self.assertEqual(eval_part("x+0.2", 1.0), 1.2)
        self.assertEqual(eval_part("x+0.5", 0.3), 0.8)

    def test_eval_part_x_minus(self):
        self.assertEqual(eval_part("x-0.1", 1.0), 0.9)
        self.assertEqual(eval_part("x-0.5", 0.3), -0.2)

    def test_eval_part_x_with_spaces(self):
        self.assertEqual(eval_part(" x-0.1 ", 1.0), 0.9)

    # --- generate_range ---

    def test_generate_range_positive_step(self):
        self.assertEqual(generate_range(0.5, 0.7, 0.1, "test"), [0.5, 0.6, 0.7])

    def test_generate_range_negative_step(self):
        self.assertEqual(generate_range(0.7, 0.5, -0.1, "test"), [0.7, 0.6, 0.5])

    def test_generate_range_single_value(self):
        self.assertEqual(generate_range(0.5, 0.5, 0.1, "test"), [0.5])

    def test_generate_range_zero_step_raises(self):
        with self.assertRaises(ValueError):
            generate_range(0.5, 0.7, 0.0, "test")

    # --- is_relative ---

    def test_is_relative_absolute(self):
        self.assertFalse(is_relative("0.8"))
        self.assertFalse(is_relative("0.5:1.0:0.1"))
        self.assertFalse(is_relative("0.5:1.0"))

    def test_is_relative_x(self):
        self.assertTrue(is_relative("x-0.1"))
        self.assertTrue(is_relative("x-0.1:x+0.2"))
        self.assertTrue(is_relative("x-0.1:x+0.2:0.1"))

    def test_is_relative_symmetric(self):
        self.assertTrue(is_relative("+-0.3"))
        self.assertTrue(is_relative("+-0.3:0.1"))

    # --- parse_weights: 绝对权重 (已有逻辑) ---

    def test_parse_single_value(self):
        self.assertEqual(parse_weights("0.8"), [0.8])
        self.assertEqual(parse_weights("-0.5"), [-0.5])

    def test_parse_range_with_step(self):
        self.assertEqual(parse_weights("0.5:0.7:0.1"), [0.5, 0.6, 0.7])
        self.assertEqual(parse_weights("-0.5:0.5:0.5"), [-0.5, 0.0, 0.5])

    def test_parse_range_default_step(self):
        os.environ["HOOK_WEIGHT_STEP"] = "0.2"
        try:
            self.assertEqual(parse_weights("0.5:0.9"), [0.5, 0.7, 0.9])
        finally:
            del os.environ["HOOK_WEIGHT_STEP"]

    def test_parse_invalid_formats(self):
        with self.assertRaises(ValueError):
            parse_weights("abc")
        with self.assertRaises(ValueError):
            parse_weights("0.5:abc")

    # --- parse_weights: x 相对表达式 ---

    def test_parse_x_single(self):
        self.assertEqual(parse_weights("x-0.1", current_value=1.0), [0.9])
        self.assertEqual(parse_weights("x+0.2", current_value=0.5), [0.7])
        self.assertEqual(parse_weights("x", current_value=0.5), [0.5])

    def test_parse_x_range_with_step(self):
        # x-0.1:x+0.2:0.1, current=0.5 → 0.4, 0.5, 0.6, 0.7
        self.assertEqual(
            parse_weights("x-0.1:x+0.2:0.1", current_value=0.5),
            [0.4, 0.5, 0.6, 0.7],
        )

    def test_parse_x_range_default_step(self):
        os.environ["HOOK_WEIGHT_STEP"] = "0.2"
        try:
            # x-0.2:x+0.2, current=0.5, step=0.2 → 0.3, 0.5, 0.7
            self.assertEqual(
                parse_weights("x-0.2:x+0.2", current_value=0.5),
                [0.3, 0.5, 0.7],
            )
        finally:
            del os.environ["HOOK_WEIGHT_STEP"]

    def test_parse_x_with_negative_current(self):
        self.assertEqual(parse_weights("x-0.1", current_value=-0.5), [-0.6])
        self.assertEqual(
            parse_weights("x-0.2:x+0.2:0.2", current_value=-0.5),
            [-0.7, -0.5, -0.3],
        )

    def test_parse_x_requires_current_value(self):
        with self.assertRaises(ValueError):
            parse_weights("x-0.1")
        with self.assertRaises(ValueError):
            parse_weights("x-0.1:x+0.2")

    # --- parse_weights: +- 对称浮动 ---

    def test_parse_symmetric_with_step(self):
        # +-0.3:0.1, current=0.5 → 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8
        self.assertEqual(
            parse_weights("+-0.3:0.1", current_value=0.5),
            [0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8],
        )

    def test_parse_symmetric_default_step(self):
        os.environ["HOOK_WEIGHT_STEP"] = "0.2"
        try:
            # +-0.4, current=0.5, step=0.2 → 0.1, 0.3, 0.5, 0.7, 0.9
            self.assertEqual(
                parse_weights("+-0.4", current_value=0.5),
                [0.1, 0.3, 0.5, 0.7, 0.9],
            )
        finally:
            del os.environ["HOOK_WEIGHT_STEP"]

    def test_parse_symmetric_single_result(self):
        # +-0.0:0.1, current=0.5 → [0.5]
        self.assertEqual(
            parse_weights("+-0.0:0.1", current_value=0.5),
            [0.5],
        )

    def test_parse_symmetric_requires_current_value(self):
        with self.assertRaises(ValueError):
            parse_weights("+-0.3")
        with self.assertRaises(ValueError):
            parse_weights("+-0.3:0.1")


if __name__ == "__main__":
    unittest.main()
