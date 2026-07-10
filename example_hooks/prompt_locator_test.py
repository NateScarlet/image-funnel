import unittest
from typing import Dict, Any

from workflow_prompt_pair import WorkflowPromptPair
from prompt_locator import (
    find_terminal_input,
    get_region_markers,
    find_nodes_with_region,
    get_target_clip_node,
    resolve_target_to_nodes,
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

    def test_find_nodes_with_region(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
            },
        }
        workflow = {
            "nodes": [
                {
                    "id": "node_1",
                    "widgets_values": [
                        "//#region hook-myregion\nhello\n//#endregion hook-myregion"
                    ],
                },
                {
                    "id": "node_2",
                    "widgets_values": ["no region here"],
                },
            ]
        }
        res = find_nodes_with_region(prompt, workflow, "myregion")
        self.assertEqual(res, ["node_1"])

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

    def test_resolve_target_to_nodes_direct_node(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
            }
        }
        workflow: Dict[str, Any] = {}
        res = resolve_target_to_nodes(prompt, workflow, "node", "node_1", False)
        self.assertEqual(res, [("node_1", "", "", False)])

    def test_resolve_target_to_nodes_missing_node(self):
        prompt: Dict[str, Any] = {}
        workflow: Dict[str, Any] = {}
        res = resolve_target_to_nodes(prompt, workflow, "node", "node_99", False)
        self.assertEqual(res, [])

    def test_locate_prompt_fragments_default(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece"},
            }
        }
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "node_1",
                    "widgets_values": ["masterpiece"],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        res = list(pair.locate_prompts(is_neg=False))
        self.assertEqual(len(res), 1)
        self.assertEqual(res[0].node_id, "node_1")
        self.assertEqual(res[0].start_marker, "//#region hook-positive")
        self.assertEqual(res[0].use_markers, True)

    def test_locate_prompt_fragments_mixed(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
            },
        }
        workflow = {
            "nodes": [
                {
                    "id": "node_1",
                    "widgets_values": ["node 1 text"],
                },
                {
                    "id": "node_2",
                    "widgets_values": [
                        "//#region hook-myregion\nhello\n//#endregion hook-myregion"
                    ],
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        res = list(pair.locate_prompts(nodes=["node_1"], regions=["myregion"]))
        self.assertEqual(len(res), 2)

        self.assertEqual(res[0].node_id, "node_1")
        self.assertEqual(res[0].use_markers, False)
        self.assertEqual(res[0].text, "node 1 text")

        self.assertEqual(res[1].node_id, "node_2")
        self.assertEqual(res[1].use_markers, True)
        self.assertEqual(res[1].text, "\nhello\n")

    def test_fragment_text_and_modify(self):
        prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece"},
            }
        }
        workflow = {
            "nodes": [
                {
                    "id": "node_1",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece\n//#endregion hook-positive"
                    ],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        fragments = list(pair.locate_prompts(is_neg=False))
        self.assertEqual(len(fragments), 1)
        fragment = fragments[0]

        # 1. 验证获取文本
        self.assertEqual(fragment.text, "\nmasterpiece\n")

        # 2. 验证获取权重
        self.assertEqual(fragment.get_weight("masterpiece"), 1.0)
        self.assertIsNone(fragment.get_weight("missing"))

        # 3. 增加提示词并验证
        success = fragment.add("beautiful scenery")
        self.assertTrue(success)
        self.assertIn("beautiful scenery", fragment.text)

        # 4. 更改权重
        modified = fragment.modify_weight("beautiful scenery", 1.5, skip_add=False)
        self.assertTrue(modified)
        self.assertEqual(fragment.get_weight("beautiful scenery"), 1.5)

        # 5. 移除提示词
        removed = fragment.remove("beautiful scenery")
        self.assertTrue(removed)
        self.assertIsNone(fragment.get_weight("beautiful scenery"))


if __name__ == "__main__":
    unittest.main()
