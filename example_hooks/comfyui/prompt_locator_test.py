# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import unittest

from .prompt_locator import (
    find_terminal_input,
    get_region_markers,
    get_target_clip_node,
    is_node_disabled,
    find_region_boundaries,
    get_region_content,
    REGION_START_RE,
    REGION_END_RE,
)


class TestPromptLocator(unittest.TestCase):

    def test_find_terminal_input_simple(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "hello"},
            }
        }
        res = find_terminal_input(prompt, "node_1", "text")
        self.assertEqual(res, ("node_1", "text"))

    def test_find_terminal_input_primitive(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": ["node_2", 0]},
            },
            "node_2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "hello primitive"},
            },
        }
        res = find_terminal_input(prompt, "node_1", "text")
        self.assertEqual(res, ("node_2", "value"))

    def test_find_terminal_input_switch_comfy(self):
        prompt = {
            "node_1": {
                "class_type": "KSampler",
                "inputs": {"cfg": ["node_2", 0]},
            },
            "node_2": {
                "class_type": "ComfySwitchNode",
                "inputs": {
                    "on_true": ["node_3", 0],
                },
            },
            "node_3": {
                "class_type": "PrimitiveFloat",
                "inputs": {"value": 7.0},
            },
        }
        res = find_terminal_input(prompt, "node_1", "cfg")
        self.assertEqual(res, ("node_3", "value"))

    def test_find_terminal_input_switch_rgthree(self):
        prompt = {
            "node_1": {
                "class_type": "KSampler",
                "inputs": {"cfg": ["node_2", 0]},
            },
            "node_2": {
                "class_type": "Any Switch (rgthree)",
                "inputs": {
                    "any_1": ["node_3", 0],
                },
            },
            "node_3": {
                "class_type": "PrimitiveFloat",
                "inputs": {"value": 8.0},
            },
        }
        res = find_terminal_input(prompt, "node_1", "cfg")
        self.assertEqual(res, ("node_3", "value"))

    def test_get_region_markers(self):
        start, end = get_region_markers("positive")
        self.assertEqual(start, "// #region positive")
        self.assertEqual(end, "// #endregion positive")

    def test_get_target_clip_node_no_locator(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "1girl, masterpiece, score_7, cute"},
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "best quality, landscape"},
            },
            "node_3": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "simple background"},
            },
        }
        target = get_target_clip_node(prompt, is_neg=False)
        self.assertEqual(target, "node_1")

        prompt_neg = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "worst quality, low quality"},
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "bad hands, error, score_3"},
            },
            "node_3": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "worst quality, low quality, score_1"},
            },
        }
        target_neg = get_target_clip_node(prompt_neg, is_neg=True)
        self.assertEqual(target_neg, "node_3")


class TestIsNodeDisabled(unittest.TestCase):
    def test_mode_2_disabled(self):
        self.assertTrue(is_node_disabled({"mode": 2}))

    def test_mode_4_disabled(self):
        self.assertTrue(is_node_disabled({"mode": 4}))

    def test_mode_0_enabled(self):
        self.assertFalse(is_node_disabled({"mode": 0}))

    def test_no_mode_key(self):
        self.assertFalse(is_node_disabled({}))

    def test_mode_1_enabled(self):
        self.assertFalse(is_node_disabled({"mode": 1}))


class TestFindTerminalInput(unittest.TestCase):
    def test_direct_value(self):
        prompt = {"1": {"inputs": {"value": 42}}}
        self.assertEqual(find_terminal_input(prompt, "1", "value"), ("1", "value"))

    def test_through_primitive(self):
        prompt = {
            "1": {"inputs": {"seed": ["2", 0]}},
            "2": {"class_type": "PrimitiveInt", "inputs": {"value": 12345}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "seed"), ("2", "value"))

    def test_through_switch_comfy_on_false(self):
        prompt = {
            "1": {"inputs": {"cfg": ["2", 0]}},
            "2": {
                "class_type": "ComfySwitchNode",
                "inputs": {"on_false": ["3", 0]},
            },
            "3": {"class_type": "PrimitiveFloat", "inputs": {"value": 5.0}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "cfg"), ("3", "value"))

    def test_through_switch_comfy_on_true(self):
        prompt = {
            "1": {"inputs": {"cfg": ["2", 0]}},
            "2": {
                "class_type": "ComfySwitchNode",
                "inputs": {"on_true": ["3", 0]},
            },
            "3": {"class_type": "PrimitiveFloat", "inputs": {"value": 7.0}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "cfg"), ("3", "value"))

    def test_through_switch_rgthree(self):
        prompt = {
            "1": {"inputs": {"model": ["2", 0]}},
            "2": {
                "class_type": "Any Switch (rgthree)",
                "inputs": {"any_1": ["3", 0]},
            },
            "3": {"class_type": "PrimitiveFloat", "inputs": {"value": 0.5}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "model"), ("3", "value"))

    def test_through_custom_node(self):
        prompt = {
            "1": {"inputs": {"anything": ["2", 0]}},
            "2": {
                "class_type": "SomeCustomNode",
                "inputs": {"output": ["3", 0]},
            },
            "3": {"class_type": "PrimitiveFloat", "inputs": {"value": 1.0}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "anything"), ("3", "value"))

    def test_missing_node(self):
        prompt = {}
        self.assertEqual(
            find_terminal_input(prompt, "nonexistent", "key"),
            ("nonexistent", "key"),
        )

    def test_string_value(self):
        prompt = {"1": {"inputs": {"text": "hello"}}}
        self.assertEqual(find_terminal_input(prompt, "1", "text"), ("1", "text"))

    def test_wrong_list_format(self):
        prompt = {"1": {"inputs": {"key": [1, 2, 3]}}}
        self.assertEqual(find_terminal_input(prompt, "1", "key"), ("1", "key"))

    def test_list_non_string_first(self):
        prompt = {"1": {"inputs": {"key": [1, 0]}}}
        self.assertEqual(find_terminal_input(prompt, "1", "key"), ("1", "key"))

    def test_nested_switch(self):
        """嵌套 Switch 节点的追溯"""
        prompt = {
            "1": {"inputs": {"seed": ["2", 0]}},
            "2": {
                "class_type": "ComfySwitchNode",
                "inputs": {"on_false": ["3", 0]},
            },
            "3": {
                "class_type": "Any Switch (rgthree)",
                "inputs": {"any_1": ["4", 0]},
            },
            "4": {"class_type": "PrimitiveInt", "inputs": {"value": 42}},
        }
        self.assertEqual(find_terminal_input(prompt, "1", "seed"), ("4", "value"))


# #region Region 标记测试


class TestRegionRegex(unittest.TestCase):
    """REGION_START_RE / REGION_END_RE 匹配测试"""

    def test_start_matches_standard(self):
        self.assertIsNotNone(REGION_START_RE.search("// #region positive"))

    def test_start_matches_no_space_after_slash(self):
        self.assertIsNotNone(REGION_START_RE.search("//#region positive"))

    def test_start_matches_multiple_spaces_after_slash(self):
        self.assertIsNotNone(REGION_START_RE.search("//   #region positive"))

    def test_start_matches_indented(self):
        self.assertIsNotNone(REGION_START_RE.search("  // #region positive"))

    def test_start_matches_indented_and_extra_spaces(self):
        self.assertIsNotNone(REGION_START_RE.search("\t//  #region positive"))

    def test_start_not_matches_no_slashes(self):
        self.assertIsNone(REGION_START_RE.search("#region positive"))

    def test_start_not_matches_no_hash(self):
        self.assertIsNone(REGION_START_RE.search("// region positive"))

    def test_start_extracts_name(self):
        m = REGION_START_RE.search("// #region my_region_name")
        self.assertIsNotNone(m)
        if m:
            self.assertEqual(m.group(1), "my_region_name")

    def test_end_matches_standard(self):
        self.assertIsNotNone(REGION_END_RE.search("// #endregion"))

    def test_end_matches_no_space_after_slash(self):
        self.assertIsNotNone(REGION_END_RE.search("//#endregion"))

    def test_end_matches_with_trailing_name(self):
        """endregion 可以附带名称（不要求，但兼容）"""
        self.assertIsNotNone(REGION_END_RE.search("// #endregion positive"))

    def test_end_matches_indented(self):
        self.assertIsNotNone(REGION_END_RE.search("  // #endregion"))

    def test_end_not_matches_no_slashes(self):
        self.assertIsNone(REGION_END_RE.search("#endregion"))


class TestFindRegionBoundaries(unittest.TestCase):
    """find_region_boundaries 测试"""

    def test_find_simple(self):
        text = "a\n// #region positive\ncontent\n// #endregion\nb"
        start, endregion_start = find_region_boundaries(text, "positive")
        self.assertNotEqual(start, -1)
        self.assertGreater(endregion_start, start)
        self.assertEqual(
            text[start : start + len("// #region positive")], "// #region positive"
        )
        self.assertEqual(
            text[endregion_start : endregion_start + len("// #endregion")],
            "// #endregion",
        )

    def test_find_content(self):
        text = "a\n// #region pos\nhello world\n// #endregion\nb"
        content = get_region_content(text, "pos")
        self.assertEqual(content, "hello world")

    def test_find_not_found(self):
        text = "no region here"
        start, end = find_region_boundaries(text, "positive")
        self.assertEqual(start, -1)
        self.assertEqual(end, -1)

    def test_find_no_end(self):
        text = "// #region positive\nno end marker"
        start, end = find_region_boundaries(text, "positive")
        self.assertEqual(start, -1)
        self.assertEqual(end, -1)

    def test_no_nesting_first_only(self):
        """不支持嵌套，返回第一个匹配"""
        text = "// #region pos\na\n// #endregion\n// #region pos\nb\n// #endregion"
        content = get_region_content(text, "pos")
        self.assertEqual(content, "a")

    def test_content_with_newlines(self):
        text = "// #region pos\na,\nb,\n// #endregion"
        content = get_region_content(text, "pos")
        self.assertEqual(content, "a,\nb,")

    def test_empty_content(self):
        text = "// #region pos\n// #endregion"
        content = get_region_content(text, "pos")
        self.assertEqual(content, "")

    def test_indented_markers(self):
        text = "  // #region pos\ncontent\n  // #endregion"
        content = get_region_content(text, "pos")
        self.assertEqual(content, "content")

    def test_complex_format(self):
        """各种格式变化"""
        text = "//#region foo\nx\n//#endregion"
        start, _ = find_region_boundaries(text, "foo")
        self.assertNotEqual(start, -1)
        self.assertEqual(get_region_content(text, "foo"), "x")

        text2 = "\t//  #region bar\ny\n\t//  #endregion"
        content = get_region_content(text2, "bar")
        self.assertEqual(content, "y")


# #endregion


if __name__ == "__main__":
    unittest.main()
