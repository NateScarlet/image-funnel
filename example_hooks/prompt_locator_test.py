# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import unittest

from prompt_locator import (
    find_terminal_input,
    get_region_markers,
    get_target_clip_node,
    is_node_disabled,
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
        self.assertTrue(start.endswith("positive"))
        self.assertTrue(end.endswith("positive"))

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


if __name__ == "__main__":
    unittest.main()
