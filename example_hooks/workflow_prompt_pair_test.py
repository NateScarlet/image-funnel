#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
workflow_prompt_pair.py 的单元测试。
"""

# 测试文件允许访问被测模块的私有成员
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false

import unittest
import os
import json
import sys
from typing import Any, Dict, List, cast
from PIL import Image

current_dir = os.path.dirname(os.path.abspath(__file__))
if current_dir not in sys.path:
    sys.path.append(current_dir)

from workflow_prompt_pair import (
    WorkflowPromptPair,
    is_node_disabled,
    find_terminal_input,
)
from comfyui import get_workflow_node_text, get_target_clip_node


# #region 基础函数测试
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


# #endregion


# #region 节点分析测试
class TestAnalyzeNodes(unittest.TestCase):
    def test_seed_detection_all_strategies(self):
        """检测各种种子策略"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [54321, "increment", 20, 7.0],
                },
                {
                    "id": "3",
                    "type": "KSampler",
                    "widgets_values": [99999, "decrement", 20, 7.0],
                },
                {
                    "id": "4",
                    "type": "KSampler",
                    "widgets_values": [11111, "randomize", 20, 7.0],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"seed": 12345}},
            "2": {"class_type": "KSampler", "inputs": {"seed": 54321}},
            "3": {"class_type": "KSampler", "inputs": {"seed": 99999}},
            "4": {"class_type": "KSampler", "inputs": {"seed": 11111}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertEqual(len(pair._seed_nodes), 4)
        self.assertTrue(pair.has_seeds_to_update())

    def test_disabled_node_skipped(self):
        """disabled 节点被跳过"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "mode": 2,
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "mode": 0,
                    "widgets_values": [456, "randomize", 20, 7.0],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"seed": 123}},
            "2": {"class_type": "KSampler", "inputs": {"seed": 456}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertTrue(pair.has_seeds_to_update())
        self.assertEqual(len(pair._seed_nodes), 1)
        self.assertEqual(pair._seed_nodes[0].node_id, "2")

    def test_date_filename_detection(self):
        """检测带日期模板的文件名节点"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["output_%date:yyyyMMdd_hhmmss%_001"],
                },
                {"id": "2", "type": "SaveImage", "widgets_values": ["output_no_date"]},
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "output_20260619_123456_001"},
            },
            "2": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "output_no_date"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertEqual(len(pair._date_filename_nodes), 1)
        self.assertEqual(pair._date_filename_nodes[0].node_id, "1")

    def test_subgraph_nodes_analyzed(self):
        """子图节点被正确分析"""
        workflow = {
            "nodes": [{"id": "1", "type": "MySubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "CLIPTextEncode",
                                "widgets_values": ["hello"],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {"1": {"class_type": "MySubgraph", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        subgraph_nodes = pair.find_nodes(is_subgraph=True)
        self.assertEqual(len(subgraph_nodes), 1)
        self.assertEqual(subgraph_nodes[0].node_id, "2")
        self.assertEqual(subgraph_nodes[0].subgraph_id, "MySubgraph")

    def test_subgraph_seed_detection(self):
        """子图中的种子节点被检测"""
        workflow = {
            "nodes": [{"id": "1", "type": "MySubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "KSampler",
                                "widgets_values": [100, "increment", 20, 7.0],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {"1": {"class_type": "MySubgraph", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertTrue(pair.has_seeds_to_update())
        self.assertEqual(len(pair._seed_nodes), 1)
        self.assertTrue(pair._seed_nodes[0].is_subgraph)

    def test_subgraph_disabled_node_skipped(self):
        """子图中 disabled 节点被跳过"""
        workflow = {
            "nodes": [{"id": "1", "type": "MySubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "KSampler",
                                "mode": 2,
                                "widgets_values": [100, "fixed", 20, 7.0],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {"1": {"class_type": "MySubgraph", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertFalse(pair.has_seeds_to_update())

    def test_non_list_widgets_values(self):
        """widgets_values 不是列表时跳过"""
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": "not a list"}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertFalse(pair.has_seeds_to_update())


# #endregion


# #region 节点查询测试
class TestFindNodes(unittest.TestCase):
    def setUp(self):
        self.workflow = {
            "nodes": [
                {"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]},
                {"id": "2", "type": "CLIPTextEncode", "widgets_values": ["hello"]},
            ]
        }
        self.prompt = {
            "1": {"class_type": "KSampler", "inputs": {"seed": 123}},
            "2": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}},
        }
        self.pair = WorkflowPromptPair(self.workflow, self.prompt)

    def test_by_node_type(self):
        result = self.pair.find_nodes(node_type="KSampler")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "1")

    def test_by_class_type(self):
        result = self.pair.find_nodes(class_type="CLIPTextEncode")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "2")

    def test_by_node_id(self):
        result = self.pair.find_nodes(node_id="1")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "1")

    def test_by_is_subgraph(self):
        result = self.pair.find_nodes(is_subgraph=False)
        self.assertEqual(len(result), 2)

    def test_multiple_criteria(self):
        result = self.pair.find_nodes(node_type="KSampler", is_subgraph=False)
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "1")

    def test_no_match(self):
        result = self.pair.find_nodes(node_type="NonExistent")
        self.assertEqual(len(result), 0)

    def test_find_nodes_by_class_type(self):
        result = self.pair.find_nodes_by_class_type("KSampler")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "1")

    def test_find_nodes_by_node_type(self):
        result = self.pair.find_nodes_by_node_type("CLIPTextEncode")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0].node_id, "2")


class TestGetNodeById(unittest.TestCase):
    def test_direct_node(self):
        workflow = {"nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123]}]}
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        node = pair.get_node_by_id("1")
        self.assertIsNotNone(node)
        self.assertEqual(node.node_id, "1")

    def test_subgraph_node(self):
        workflow = {
            "nodes": [{"id": "1", "type": "MySubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "CLIPTextEncode",
                                "widgets_values": ["text"],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {"1": {"class_type": "MySubgraph", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        node = pair.get_node_by_id("1:2")
        self.assertIsNotNone(node)
        self.assertEqual(node.node_id, "2")
        self.assertTrue(node.is_subgraph)

    def test_subgraph_wrong_parent(self):
        """父节点类型与子图 ID 不匹配"""
        workflow = {
            "nodes": [{"id": "1", "type": "OtherSubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "CLIPTextEncode",
                                "widgets_values": ["text"],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {"1": {"class_type": "OtherSubgraph", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        node = pair.get_node_by_id("1:2")
        self.assertIsNone(node)

    def test_not_found(self):
        workflow = {"nodes": [{"id": "1", "type": "KSampler"}]}
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertIsNone(pair.get_node_by_id("999"))

    def test_subgraph_parent_not_found(self):
        workflow = {"nodes": [{"id": "1", "type": "KSampler"}]}
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertIsNone(pair.get_node_by_id("999:1"))


# #endregion


# #region 种子更新测试
class TestSeedUpdate(unittest.TestCase):
    def test_has_seeds_true(self):
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertTrue(pair.has_seeds_to_update())

    def test_has_seeds_false(self):
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertFalse(pair.has_seeds_to_update())

    def test_has_seeds_false_no_widgets(self):
        workflow = {"nodes": [{"id": "1", "type": "KSampler"}]}
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertFalse(pair.has_seeds_to_update())

    def test_update_seeds_fixed(self):
        """fixed 策略：种子值不变"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 7.0],
                }
            ]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 12345}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 12345)

    def test_update_seeds_increment(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "increment", 20, 7.0],
                }
            ]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 12345}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 12346)

    def test_update_seeds_decrement(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "decrement", 20, 7.0],
                }
            ]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 12345}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 12344)

    def test_update_seeds_decrement_at_zero(self):
        """decrement 到 0 时不再减"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [0, "decrement", 20, 7.0],
                }
            ]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 0}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 0)

    def test_update_seeds_randomize(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "randomize", 20, 7.0],
                }
            ]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {"seed": 12345}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        new_seed = workflow["nodes"][0]["widgets_values"][0]
        self.assertIsInstance(new_seed, int)
        self.assertGreaterEqual(new_seed, 1)
        self.assertLessEqual(new_seed, 1125899906842624)

    def test_update_seeds_with_primitive_connection(self):
        """种子通过 PrimitiveInt 节点连接"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "increment", 20, 7.0],
                },
                {"id": "2", "type": "PrimitiveInt", "widgets_values": [12345]},
            ]
        }
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"seed": ["2", 0]}},
            "2": {"class_type": "PrimitiveInt", "inputs": {"value": 12345}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(prompt["2"]["inputs"]["value"], 12346)

    def test_update_seeds_subgraph(self):
        """子图中的种子节点"""
        workflow = {
            "nodes": [{"id": "1", "type": "MySubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "KSampler",
                                "widgets_values": [100, "increment", 20, 7.0],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {
            "1": {"class_type": "MySubgraph", "inputs": {}},
            "1:2": {"class_type": "KSampler", "inputs": {"seed": 100}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 1)
        self.assertEqual(prompt["1:2"]["inputs"]["seed"], 101)

    def test_update_seeds_missing_in_prompt(self):
        """种子节点在 prompt 中不存在时抛出异常"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [12345, "increment", 20, 7.0],
                }
            ]
        }
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        with self.assertRaises(ValueError):
            pair.update_seeds()

    def test_update_seeds_no_seeds(self):
        """没有种子节点时返回 0"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 0)

    def test_update_seeds_multiple(self):
        """多个种子节点同时更新"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [10, "increment", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [20, "increment", 20, 7.0],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"seed": 10}},
            "2": {"class_type": "KSampler", "inputs": {"seed": 20}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        count = pair.update_seeds()
        self.assertEqual(count, 2)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 11)
        self.assertEqual(workflow["nodes"][1]["widgets_values"][0], 21)


# #endregion


# #region 文件名更新测试
class TestFilenameUpdate(unittest.TestCase):
    def test_update_output_filenames_basic(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["my_output_%date:yyyyMMdd_hhmmss%_001"],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "my_output_20260601_000000_001"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.update_output_filenames()
        new_prefix = prompt["1"]["inputs"]["filename_prefix"]
        self.assertNotEqual(new_prefix, "my_output_20260601_000000_001")
        self.assertRegex(new_prefix, r"my_output_\d{8}_\d{6}_001")

    def test_update_output_filenames_no_date_nodes(self):
        """没有日期模板节点时不做任何修改"""
        workflow = {
            "nodes": [{"id": "1", "type": "SaveImage", "widgets_values": ["output"]}]
        }
        prompt = {
            "1": {"class_type": "SaveImage", "inputs": {"filename_prefix": "output"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.update_output_filenames()
        self.assertEqual(prompt["1"]["inputs"]["filename_prefix"], "output")

    def test_update_output_filenames_through_primitive(self):
        """文件名通过 PrimitiveString 节点连接"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["output_%date:yyyyMMdd_hhmmss%"],
                },
                {
                    "id": "2",
                    "type": "PrimitiveString",
                    "widgets_values": ["output_20260601_000000"],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "SaveImage", "inputs": {"filename_prefix": ["2", 0]}},
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "output_20260601_000000"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.update_output_filenames()
        self.assertRegex(prompt["2"]["inputs"]["value"], r"output_\d{8}_\d{6}")

    def test_update_output_filenames_subgraph(self):
        """子图中的文件名节点"""
        workflow = {
            "nodes": [{"id": "1", "type": "MySaveSubgraph"}],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySaveSubgraph",
                        "nodes": [
                            {
                                "id": "2",
                                "type": "SaveImage",
                                "widgets_values": ["output_%date:yyyyMMdd_hhmmss%"],
                            },
                        ],
                    }
                ]
            },
        }
        prompt = {
            "1": {"class_type": "MySaveSubgraph", "inputs": {}},
            "1:2": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "output_20260601_000000"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.update_output_filenames()
        self.assertRegex(
            prompt["1:2"]["inputs"]["filename_prefix"], r"output_\d{8}_\d{6}"
        )

    def test_update_output_filenames_missing_in_prompt(self):
        """文件名节点在 prompt 中不存在时抛出异常"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["output_%date:yyyyMMdd_hhmmss%"],
                },
            ]
        }
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        with self.assertRaises(ValueError):
            pair.update_output_filenames()

    def test_convert_comfy_date_format_full(self):
        pair = WorkflowPromptPair({"nodes": []}, {})
        py_fmt, regex = pair._convert_comfy_date_format_to_python("yyyyMMdd_hhmmss")
        self.assertEqual(py_fmt, "%Y%m%d_%H%M%S")
        self.assertEqual(regex, r"\d{4}\d{2}\d{2}_\d{2}\d{2}\d{2}")

    def test_convert_comfy_date_format_short(self):
        pair = WorkflowPromptPair({"nodes": []}, {})
        py_fmt, regex = pair._convert_comfy_date_format_to_python("yyMMdd")
        self.assertEqual(py_fmt, "%y%m%d")
        self.assertEqual(regex, r"\d{2}\d{2}\d{2}")


# #endregion


# #region Lora 权重测试
class TestLoraWeight(unittest.TestCase):
    def test_get_current_lora_weight_native_lora(self):
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": 0.8,
                    "strength_clip": 0.8,
                },
            }
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.8)

    def test_get_current_lora_weight_native_lora_through_primitive(self):
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": ["2", 0],
                    "strength_clip": ["2", 0],
                },
            },
            "2": {"class_type": "PrimitiveFloat", "inputs": {"value": 0.75}},
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.75)

    def test_get_current_lora_weight_power_lora(self):
        prompt = {
            "1": {
                "class_type": "Power Lora Loader (rgthree)",
                "inputs": {"lora_1": {"lora": "my_style.safetensors", "strength": 0.9}},
            }
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.9)

    def test_get_current_lora_weight_workflow_fallback(self):
        prompt = {}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "widgets_values": ["my_style.safetensors", 0.6, 0.6],
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.6)

    def test_get_current_lora_weight_power_lora_workflow(self):
        prompt = {}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "Power Lora Loader (rgthree)",
                    "widgets_values": [
                        {"lora": "my_style.safetensors", "strength": 0.7}
                    ],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.7)

    def test_get_current_lora_weight_not_found(self):
        prompt = {}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_lora_weight("nonexistent")
        self.assertIsNone(weight)

    def test_modify_lora_weights_native_lora(self):
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": 0.8,
                    "strength_clip": 0.8,
                },
            }
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "widgets_values": ["my_style.safetensors", 0.8, 0.8],
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["1"]["inputs"]["strength_model"], 0.5)
        self.assertEqual(prompt["1"]["inputs"]["strength_clip"], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][2], 0.5)

    def test_modify_lora_weights_native_lora_primitive(self):
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": ["2", 0],
                    "strength_clip": ["2", 0],
                },
            },
            "2": {"class_type": "PrimitiveFloat", "inputs": {"value": 0.8}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "widgets_values": ["my_style.safetensors", 0.8, 0.8],
                },
                {"id": "2", "type": "PrimitiveFloat", "widgets_values": [0.8]},
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["2"]["inputs"]["value"], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][2], 0.5)
        self.assertEqual(workflow["nodes"][1]["widgets_values"][0], 0.5)

    def test_modify_lora_weights_power_lora(self):
        prompt = {
            "1": {
                "class_type": "Power Lora Loader (rgthree)",
                "inputs": {"lora_1": {"lora": "my_style.safetensors", "strength": 0.8}},
            }
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "Power Lora Loader (rgthree)",
                    "widgets_values": [
                        {"lora": "my_style.safetensors", "strength": 0.8}
                    ],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["1"]["inputs"]["lora_1"]["strength"], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0]["strength"], 0.5)

    def test_generate_lora_variants_relative_no_current(self):
        """相对权重但无法获取当前值时不生成变体"""
        prompt = {}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_lora_variants("nonexistent", "x+0.1"))
        self.assertEqual(len(variants), 0)


# #endregion


# #region CFG 权重测试
class TestCfgWeight(unittest.TestCase):
    def test_get_current_cfg_weight_basic(self):
        prompt = {"1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}}}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        cfg = pair.get_current_cfg_weight()
        self.assertEqual(cfg, 7.0)

    def test_get_current_cfg_weight_through_primitive(self):
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": ["2", 0]}},
            "2": {"class_type": "PrimitiveFloat", "inputs": {"value": 8.5}},
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        cfg = pair.get_current_cfg_weight()
        self.assertEqual(cfg, 8.5)

    def test_get_current_cfg_weight_with_node_ids(self):
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
            "2": {"class_type": "KSampler", "inputs": {"cfg": 5.0}},
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        cfg = pair.get_current_cfg_weight(node_ids=["1"])
        self.assertEqual(cfg, 7.0)

    def test_get_current_cfg_weight_not_found(self):
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        cfg = pair.get_current_cfg_weight()
        self.assertIsNone(cfg)

    def test_modify_cfg_weights_basic(self):
        prompt = {"1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}}}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                }
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_cfg_weights(9.0)
        self.assertEqual(prompt["1"]["inputs"]["cfg"], 9.0)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][3], 9.0)

    def test_modify_cfg_weights_through_primitive(self):
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": ["2", 0]}},
            "2": {"class_type": "PrimitiveFloat", "inputs": {"value": 7.0}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {"id": "2", "type": "PrimitiveFloat", "widgets_values": [7.0]},
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_cfg_weights(9.0)
        self.assertEqual(prompt["2"]["inputs"]["value"], 9.0)
        self.assertEqual(workflow["nodes"][1]["widgets_values"][0], 9.0)

    def test_modify_cfg_weights_ksampler_advanced(self):
        prompt = {"1": {"class_type": "KSamplerAdvanced", "inputs": {"cfg": 7.0}}}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSamplerAdvanced",
                    "widgets_values": [123, "fixed", 20, 7.0, 1.0],
                },
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_cfg_weights(9.0)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][4], 9.0)

    def test_modify_cfg_weights_with_node_ids(self):
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
            "2": {"class_type": "KSampler", "inputs": {"cfg": 5.0}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [456, "fixed", 20, 5.0],
                },
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_cfg_weights(9.0, node_ids=["1"])
        self.assertEqual(prompt["1"]["inputs"]["cfg"], 9.0)
        self.assertEqual(prompt["2"]["inputs"]["cfg"], 5.0)

    def test_generate_cfg_variants_absolute(self):
        prompt = {"1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}}}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                }
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_cfg_variants("5.0:9.0:2.0"))
        self.assertEqual(len(variants), 3)

    def test_generate_cfg_variants_with_node_ids(self):
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
            "2": {"class_type": "KSampler", "inputs": {"cfg": 5.0}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [456, "fixed", 20, 5.0],
                },
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_cfg_variants("8.0", node_ids=["1"]))
        self.assertEqual(len(variants), 1)
        self.assertEqual(prompt["1"]["inputs"]["cfg"], 8.0)
        self.assertEqual(prompt["2"]["inputs"]["cfg"], 5.0)

    def test_generate_cfg_variants_no_ksampler(self):
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_cfg_variants("7.0"))
        self.assertEqual(len(variants), 0)

    def test_generate_cfg_variants_relative(self):
        """相对权重表达式"""
        prompt = {"1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}}}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                }
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_cfg_variants("x-1.0:x+1.0:1.0"))
        self.assertEqual(len(variants), 3)


# #endregion


# #region 提示词权重测试
class TestPromptWeight(unittest.TestCase):
    def test_get_current_prompt_weight_with_weight(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["(beautiful:1.2) scenery"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "(beautiful:1.2) scenery"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("1", "", "", False)], "beautiful")
        self.assertEqual(weight, 1.2)

    def test_get_current_prompt_weight_brackets(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["(beautiful) scenery"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "(beautiful) scenery"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("1", "", "", False)], "beautiful")
        self.assertEqual(weight, 1.0)

    def test_get_current_prompt_weight_bare(self):
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["beautiful scenery"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("1", "", "", False)], "beautiful")
        self.assertEqual(weight, 1.0)

    def test_get_current_prompt_weight_not_found(self):
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello world"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello world"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("1", "", "", False)], "nonexistent")
        self.assertIsNone(weight)

    def test_get_current_prompt_weight_node_not_found(self):
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("999", "", "", False)], "test")
        self.assertIsNone(weight)

    def test_adjust_prompt_weight_in_text_bare_word(self):
        """裸词被转换为带权重的格式"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        new_text, modified = pair._adjust_prompt_weight_in_text(
            "hello world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_adjust_prompt_weight_in_text_brackets(self):
        """带括号无权重的格式被转换为带权重的格式"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        new_text, modified = pair._adjust_prompt_weight_in_text(
            "(hello) world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_adjust_prompt_weight_in_text_existing_weight(self):
        """已有权重的格式被更新"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        new_text, modified = pair._adjust_prompt_weight_in_text(
            "(hello:1.2) world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_adjust_prompt_weight_in_text_not_found(self):
        """未找到目标词"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        new_text, modified = pair._adjust_prompt_weight_in_text(
            "hello world", "nonexistent", 1.5
        )
        self.assertFalse(modified)
        self.assertEqual(new_text, "hello world")

    def test_adjust_prompt_weight_in_text_negative_weight(self):
        """负权重"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        new_text, modified = pair._adjust_prompt_weight_in_text(
            "(hello:-0.5) world", "hello", -1.0
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:-1.0) world")

    def test_modify_prompt_weights_skip_add(self):
        """skip_add=True 且提示词不存在时不添加"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello world"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello world"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_prompt_weights(
            [("1", "", "", False)], "nonexistent", 1.5, skip_add=True
        )
        self.assertNotIn("nonexistent", prompt["1"]["inputs"]["text"])

    def test_modify_prompt_weights_add(self):
        """skip_add=False 时添加不存在的提示词"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello world"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello world"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_prompt_weights(
            [("1", "", "", False)], "beautiful", 1.5, skip_add=False
        )
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])

    def test_strip_comments_for_prompt(self):
        pair = WorkflowPromptPair({"nodes": []}, {})
        text = "hello\n// this is a comment\nworld\n// another comment"
        result = pair._strip_comments_for_prompt(text)
        self.assertEqual(result, "hello\nworld")

    def test_add_prompt_to_node_with_marker(self):
        """通过 marker 区域添加提示词"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair._add_prompt_to_node(
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_add_prompt_to_node_without_marker(self):
        """无 marker 时直接追加"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["masterpiece, best quality"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece, best quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair._add_prompt_to_node(
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_add_prompt_to_node_empty_marker(self):
        """空 marker 区域"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "\nbest quality"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair._add_prompt_to_node(
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_get_workflow_node_text_direct(self):
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello world"]}
            ]
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello world"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        text = pair._get_workflow_node_text("1")
        self.assertEqual(text, "hello world")

    def test_get_workflow_node_text_not_found(self):
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        text = pair._get_workflow_node_text("999")
        self.assertIsNone(text)

    def test_get_workflow_node_text_non_string(self):
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        text = pair._get_workflow_node_text("1")
        self.assertIsNone(text)

    def test_generate_prompt_variants_relative_no_current(self):
        """相对权重但无法获取当前值时不生成变体"""
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(
            pair.generate_prompt_variants([], "nonexistent", "x+0.1", False)
        )
        self.assertEqual(len(variants), 0)

    def test_generate_prompt_variants_skip_add_no_exist(self):
        """skip_add=True 且提示词不存在时不生成变体"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(
            pair.generate_prompt_variants(
                [("1", "", "", False)], "nonexistent", "1.5", skip_add=True
            )
        )
        self.assertEqual(len(variants), 0)


# #endregion


# #region 双轨道文本处理测试
class TestDoubleTrack(unittest.TestCase):
    def setUp(self):
        self.start_marker = "//#region hook-positive"
        self.end_marker = "//#endregion hook-positive"

    def test_add_without_markers(self):
        """use_markers=False 时直接追加"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "add", "beautiful scenery", "", "", False, False, False, False
        )
        self.assertTrue(result)
        self.assertIn("beautiful scenery", workflow["nodes"][0]["widgets_values"][0])
        self.assertNotIn("//#region", workflow["nodes"][0]["widgets_values"][0])

    def test_add_raw(self):
        """raw=True 时不添加逗号"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "raw text here",
            self.start_marker,
            self.end_marker,
            True,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("raw text here", workflow["nodes"][0]["widgets_values"][0])

    def test_add_no_skip(self):
        """no_skip=True 时即使已存在也添加"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            True,
            False,
            True,
        )
        self.assertTrue(result)

    def test_add_skip_existing(self):
        """已存在且 no_skip=False 时跳过"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertFalse(result)

    def test_add_existing_no_marker_skip(self):
        """无 marker 时已存在跳过"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["beautiful scenery, masterpiece"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery, masterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "add", "beautiful scenery", "", "", False, False, False, False
        )
        self.assertFalse(result)

    def test_remove_hard(self):
        """hard=True 时直接删除"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            True,
            True,
        )
        self.assertTrue(result)
        self.assertNotIn("beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_remove_comment_out(self):
        """hard=False 时注释掉"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("// beautiful scenery", workflow["nodes"][0]["widgets_values"][0])
        self.assertNotIn("beautiful scenery", prompt["1"]["inputs"]["text"])

    def test_remove_no_marker_hard(self):
        """hard=True, has_marker=False"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["beautiful scenery, masterpiece"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery, masterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "beautiful scenery", "", "", False, False, True, False
        )
        self.assertTrue(result)
        self.assertNotIn("beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_remove_no_marker_comment_out(self):
        """hard=False, has_marker=False"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["beautiful scenery, masterpiece"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery, masterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "beautiful scenery", "", "", False, False, False, False
        )
        self.assertTrue(result)
        self.assertIn("// beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_remove_raw(self):
        """raw=True 时做纯文本替换"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nsome raw text,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "some raw text,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "raw text",
            self.start_marker,
            self.end_marker,
            True,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertNotIn("raw text", workflow["nodes"][0]["widgets_values"][0])

    def test_remove_no_skip(self):
        """no_skip=True 时即使没找到也继续"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "nonexistent_prompt",
            self.start_marker,
            self.end_marker,
            False,
            True,
            False,
            True,
        )
        self.assertTrue(result)

    def test_remove_skip_not_found(self):
        """没找到且 no_skip=False 时跳过"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece,\n//#endregion hook-positive"
                    ],
                }
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece,"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "nonexistent",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertFalse(result)

    def test_nonexistent_node(self):
        """节点不存在时返回 False"""
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "999", "add", "test", "", "", False, False, False, False
        )
        self.assertFalse(result)

    def test_add_with_comments_in_workflow(self):
        """workflow 中有注释但 prompt 没有时 (is_equivalent=False) 的回退"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// comment in workflow\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn(
            "// comment in workflow", workflow["nodes"][0]["widgets_values"][0]
        )
        self.assertIn("beautiful scenery", prompt["1"]["inputs"]["text"])

    def test_add_non_equivalent_no_match(self):
        """非等价情况下 marker 内容无法在 prompt 中匹配时的回退"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// some comment\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "completely different text"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("beautiful scenery", prompt["1"]["inputs"]["text"])

    def test_remove_non_equivalent(self):
        """workflow 和 prompt 文本不一致时的 remove 回退"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// comment in workflow\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn(
            "// comment in workflow", workflow["nodes"][0]["widgets_values"][0]
        )
        self.assertNotIn("beautiful scenery", prompt["1"]["inputs"]["text"])

    def test_remove_non_equivalent_no_match(self):
        """非等价 remove 且 target_match_content 不在 prompt 中"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// some comment\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "completely different text"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            self.start_marker,
            self.end_marker,
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertEqual(prompt["1"]["inputs"]["text"], "completely different text")

    def test_remove_hard_raw_no_marker(self):
        """hard=True, raw=True, has_marker=False"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["beautiful scenery, masterpiece"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery, masterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "beautiful scenery", "", "", True, False, True, False
        )
        self.assertTrue(result)
        self.assertNotIn("beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_remove_skip_not_found_no_marker(self):
        """没找到 (has_marker=False) 且 no_skip=False 时跳过"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "nonexistent", "", "", False, False, False, False
        )
        self.assertFalse(result)

    def test_remove_no_skip_not_found_no_marker(self):
        """no_skip=True 且没找到 (has_marker=False)"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "nonexistent", "", "", False, True, False, False
        )
        self.assertTrue(result)

    def test_add_raw_no_marker(self):
        """raw=True, use_markers=False"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "add", "raw_text", "", "", True, False, False, False
        )
        self.assertTrue(result)
        self.assertIn("raw_text", workflow["nodes"][0]["widgets_values"][0])


# #endregion


# #region 提交测试
class TestSubmit(unittest.TestCase):
    def test_submit_failure(self):
        """提交到不存在的服务器返回 False"""
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        workflow = {"nodes": [{"id": "1", "type": "KSampler"}]}
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.submit("http://127.0.0.1:19999")
        self.assertFalse(result)


# #endregion


# #region 样本文件集成测试
class TestWorkflowPromptPairIntegration(unittest.TestCase):

    def setUp(self):
        self.samples_dir = os.path.join(current_dir, "samples")
        self.assertTrue(
            os.path.exists(self.samples_dir),
            f"Samples directory not found at: {self.samples_dir}",
        )

        self.png_files = [
            os.path.join(self.samples_dir, f)
            for f in os.listdir(self.samples_dir)
            if f.lower().endswith(".png")
        ]
        self.assertTrue(
            len(self.png_files) > 0, "No PNG sample files found in samples directory"
        )

    def test_double_track_modification_flow(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    info = img.info
                    prompt_str = info.get("prompt")
                    workflow_str = info.get("workflow")

                    self.assertIsNotNone(
                        prompt_str, f"Prompt metadata missing in {png_path}"
                    )
                    self.assertIsNotNone(
                        workflow_str, f"Workflow metadata missing in {png_path}"
                    )

                    assert prompt_str is not None
                    assert workflow_str is not None

                    prompt = json.loads(prompt_str)
                    workflow = json.loads(workflow_str)

                is_neg = False
                target_node_id = get_target_clip_node(prompt, is_neg)
                self.assertIsNotNone(
                    target_node_id, f"Failed to locate target clip node in {png_path}"
                )
                assert target_node_id is not None

                start_marker = "//#region hook-positive"
                end_marker = "//#endregion hook-positive"

                pair = WorkflowPromptPair(workflow, prompt)

                # --- 第一次 add "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"
                args_raw = False
                args_no_skip = False

                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None
                self.assertNotIn(
                    start_marker,
                    workflow_text,
                    "Start marker should not be in raw sample workflow text yet",
                )

                self.assertTrue(
                    pair.process_double_track(
                        target_node_id,
                        "add",
                        prompt_str_arg,
                        start_marker,
                        end_marker,
                        args_raw,
                        args_no_skip,
                        False,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertIn("beautiful scenery,", pr_text_got)

                # --- 第二次 add "golden sunset" ---
                prompt_str_arg = "golden sunset"

                self.assertTrue(
                    pair.process_double_track(
                        target_node_id,
                        "add",
                        prompt_str_arg,
                        start_marker,
                        end_marker,
                        args_raw,
                        args_no_skip,
                        False,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第三次 remove "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"

                self.assertTrue(
                    pair.process_double_track(
                        target_node_id,
                        "remove",
                        prompt_str_arg,
                        start_marker,
                        end_marker,
                        args_raw,
                        args_no_skip,
                        True,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertNotIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第四次 remove "golden sunset" (hard=False) ---
                prompt_str_arg = "golden sunset"

                self.assertTrue(
                    pair.process_double_track(
                        target_node_id,
                        "remove",
                        prompt_str_arg,
                        start_marker,
                        end_marker,
                        args_raw,
                        args_no_skip,
                        False,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn("// golden sunset", wf_text_got)
                self.assertNotIn("golden sunset", pr_text_got)

    def test_adjust_lora_weights(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                lora_keywords = ["evanescia", "semi-nffa", "cunny_funky"]
                target_keyword = None
                for kw in lora_keywords:
                    for v in prompt.values():
                        if isinstance(v, dict):
                            v_dict = cast(Dict[str, Any], v)
                            if (
                                v_dict.get("class_type")
                                == "Power Lora Loader (rgthree)"
                            ):
                                inputs = cast(Dict[str, Any], v_dict.get("inputs", {}))
                                for k2, v2 in inputs.items():
                                    if k2.startswith("lora_") and isinstance(v2, dict):
                                        v2_dict = cast(Dict[str, Any], v2)
                                        lora_val = v2_dict.get("lora", "")
                                        if (
                                            isinstance(lora_val, str)
                                            and kw in lora_val.lower()
                                        ):
                                            target_keyword = kw
                                            break

                self.assertIsNotNone(
                    target_keyword, f"No expected Lora keyword found in {png_path}"
                )
                assert target_keyword is not None

                pair = WorkflowPromptPair(workflow, prompt)
                variants = list(pair.generate_lora_variants(target_keyword, "0.99"))
                self.assertTrue(len(variants) > 0)
                prompt, workflow = pair.prompt, pair.workflow

                found_prompt_updated = False
                found_workflow_updated = False
                for v in prompt.values():
                    if isinstance(v, dict):
                        v_dict = cast(Dict[str, Any], v)
                        if v_dict.get("class_type") == "Power Lora Loader (rgthree)":
                            inputs = cast(Dict[str, Any], v_dict.get("inputs", {}))
                            for k2, v2 in inputs.items():
                                if k2.startswith("lora_") and isinstance(v2, dict):
                                    v2_dict = cast(Dict[str, Any], v2)
                                    lora_val = v2_dict.get("lora", "")
                                    if (
                                        isinstance(lora_val, str)
                                        and target_keyword in lora_val.lower()
                                    ):
                                        self.assertEqual(v2_dict.get("strength"), 0.99)
                                        found_prompt_updated = True

                for node in workflow.get("nodes", []):
                    node_dict = cast(Dict[str, Any], node)
                    if node_dict.get("type") == "Power Lora Loader (rgthree)":
                        widgets_values = node_dict.get("widgets_values", [])
                        if isinstance(widgets_values, list):
                            widgets_values_list = cast(List[Any], widgets_values)
                            for val in widgets_values_list:
                                if isinstance(val, dict) and "lora" in val:
                                    val_dict = cast(Dict[str, Any], val)
                                    lora_val = val_dict.get("lora", "")
                                    if (
                                        isinstance(lora_val, str)
                                        and target_keyword in lora_val.lower()
                                    ):
                                        self.assertEqual(val_dict.get("strength"), 0.99)
                                        found_workflow_updated = True

                self.assertTrue(found_prompt_updated)
                self.assertTrue(found_workflow_updated)

    def test_adjust_lora_weights_native_and_primitive(self):
        prompt: Dict[str, Any] = {
            "node_lora": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "style_test.safetensors",
                    "strength_model": ["node_prim", 0],
                    "strength_clip": ["node_prim", 0],
                },
            },
            "node_prim": {"class_type": "PrimitiveFloat", "inputs": {"value": 1.0}},
        }
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "node_lora",
                    "type": "LoraLoader",
                    "widgets_values": ["style_test.safetensors", 1.0, 1.0],
                },
                {"id": "node_prim", "type": "PrimitiveFloat", "widgets_values": [1.0]},
            ]
        }

        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_lora_variants("style_test", "-0.75"))
        self.assertTrue(len(variants) > 0)
        prompt, workflow = pair.prompt, pair.workflow

        node_prim = cast(Dict[str, Any], prompt["node_prim"])
        prim_inputs = cast(Dict[str, Any], node_prim["inputs"])
        self.assertEqual(prim_inputs["value"], -0.75)
        nodes_list = cast(List[Dict[str, Any]], workflow["nodes"])
        for node in nodes_list:
            if node["id"] == "node_prim":
                widgets_values = node["widgets_values"]
                if isinstance(widgets_values, list):
                    self.assertEqual(widgets_values[0], -0.75)
            elif node["id"] == "node_lora":
                self.assertEqual(node["widgets_values"][1], -0.75)
                self.assertEqual(node["widgets_values"][2], -0.75)

    def test_adjust_prompt_weights(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                target_node_id = get_target_clip_node(prompt, is_neg=False)
                self.assertIsNotNone(target_node_id)
                assert target_node_id is not None

                target_nodes = [
                    (
                        target_node_id,
                        "//#region hook-positive",
                        "//#endregion hook-positive",
                        True,
                    )
                ]

                wf_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(wf_text)
                assert wf_text is not None

                target_word = "score_7" if "score_7" in wf_text else "masterpiece"
                self.assertIn(target_word, wf_text)

                pair = WorkflowPromptPair(workflow, prompt)
                modified = pair.generate_prompt_variants(
                    target_nodes, target_word, "1.35", skip_add=True
                )
                variants = list(modified)
                self.assertTrue(len(variants) > 0)
                prompt, workflow = pair.prompt, pair.workflow

                new_wf_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text)
                assert new_wf_text is not None
                self.assertIn(f"({target_word}:1.35)", new_wf_text)

                new_pr_text = prompt[target_node_id]["inputs"]["text"]
                self.assertIn(f"({target_word}:1.35)", new_pr_text)

                # 再次修改：支持在带权重的括号中继续修改
                pair2 = WorkflowPromptPair(workflow, prompt)
                modified2 = pair2.generate_prompt_variants(
                    target_nodes, target_word, "-0.5", skip_add=True
                )
                variants2 = list(modified2)
                self.assertTrue(len(variants2) > 0)
                prompt, workflow = pair2.prompt, pair2.workflow

                new_wf_text2 = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text2)
                assert new_wf_text2 is not None
                self.assertIn(f"({target_word}:-0.5)", new_wf_text2)

                # 测试不存在的词，且 skip_add=True -> 应不修改，生成器为空
                pair_skip = WorkflowPromptPair(workflow, prompt)
                modified_skip = pair_skip.generate_prompt_variants(
                    target_nodes, "non_existent_word_abc", "1.5", skip_add=True
                )
                variants_skip = list(modified_skip)
                self.assertEqual(len(variants_skip), 0)

                # 测试不存在的词，且 skip_add=False -> 应修改成功，生成器非空且添加该词
                pair_add = WorkflowPromptPair(workflow, prompt)
                modified_add = pair_add.generate_prompt_variants(
                    target_nodes, "non_existent_word_abc", "1.5", skip_add=False
                )
                variants_add = list(modified_add)
                self.assertTrue(len(variants_add) > 0)
                prompt, workflow = pair_add.prompt, pair_add.workflow

                new_wf_text3 = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text3)
                assert new_wf_text3 is not None
                self.assertIn("(non_existent_word_abc:1.5)", new_wf_text3)


# #endregion


# #region 补充覆盖率测试
class TestCoverageGaps(unittest.TestCase):
    """覆盖剩余未覆盖的分支和边界情况"""

    def test_get_current_lora_weight_disabled_workflow(self):
        """workflow 中 disabled 节点被跳过"""
        prompt = {}
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "mode": 2,
                    "widgets_values": ["lora.safetensors", 0.5, 0.5],
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertIsNone(pair.get_current_lora_weight("lora"))

    def test_get_current_lora_weight_non_list_widgets(self):
        """workflow 中 widgets_values 不是 list 时跳过"""
        prompt = {}
        workflow = {
            "nodes": [{"id": "1", "type": "LoraLoader", "widgets_values": "not_a_list"}]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        self.assertIsNone(pair.get_current_lora_weight("lora"))

    def test_modify_lora_weights_disabled_node(self):
        """disabled 的 workflow 节点不被修改"""
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": 0.8,
                    "strength_clip": 0.8,
                },
            }
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "mode": 2,
                    "widgets_values": ["my_style.safetensors", 0.8, 0.8],
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["1"]["inputs"]["strength_model"], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 0.8)

    def test_get_current_cfg_weight_node_ids_skip(self):
        """node_ids 过滤时跳过不匹配的节点"""
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
            "2": {"class_type": "KSampler", "inputs": {"cfg": 5.0}},
        }
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        cfg = pair.get_current_cfg_weight(node_ids=["2"])
        self.assertEqual(cfg, 5.0)

    def test_modify_cfg_weights_disabled_node(self):
        """disabled 的 KSampler 节点不被修改"""
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
            "2": {"class_type": "KSampler", "inputs": {"cfg": 5.0}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "mode": 2,
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [456, "fixed", 20, 5.0],
                },
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_cfg_weights(9.0)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][3], 7.0)
        self.assertEqual(workflow["nodes"][1]["widgets_values"][3], 9.0)

    def test_get_current_prompt_weight_no_text(self):
        """节点没有文本 widget 时返回 None"""
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([("1", "", "", False)], "test")
        self.assertIsNone(weight)

    def test_modify_prompt_weights_no_text(self):
        """节点没有文本 widget 时跳过"""
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_prompt_weights([("1", "", "", False)], "test", 1.5, skip_add=False)
        self.assertNotIn("test", prompt["1"].get("inputs", {}).get("text", ""))

    def test_modify_prompt_weights_no_node_info(self):
        """节点不在缓存中时跳过"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": "hello"}}}
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_prompt_weights(
            [("999", "", "", False)], "test", 1.5, skip_add=False
        )

    def test_modify_prompt_weights_non_string_prompt_text(self):
        """prompt 中 text 不是字符串时降级为空字符串"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_prompt_weights(
            [("1", "", "", False)], "nonexistent", 1.5, skip_add=False
        )
        self.assertIn("(nonexistent:1.5)", prompt["1"]["inputs"]["text"])

    def test_update_workflow_node_text_no_node(self):
        """节点不存在时不报错"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        pair._update_workflow_node_text("999", "new text")

    def test_add_prompt_to_node_no_node(self):
        """节点不存在时直接返回"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        pair._add_prompt_to_node("999", "test", "", "", False)

    def test_add_prompt_to_node_non_string_prompt_text(self):
        """prompt 中 text 不是字符串时降级"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        pair._add_prompt_to_node("1", "(beautiful:1.5)", "", "", False)
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_add_prompt_to_node_marker_no_comma(self):
        """marker 内容不以逗号结尾时自动添加"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair._add_prompt_to_node(
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])

    def test_process_double_track_non_string_prompt_text(self):
        """prompt_text 不是字符串时降级"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "add", "beautiful", "", "", False, False, False, False
        )
        self.assertTrue(result)

    def test_process_double_track_add_marker_no_comma(self):
        """add 时 marker 内容不以逗号结尾"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\nmasterpiece\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("beautiful,", workflow["nodes"][0]["widgets_values"][0])

    def test_process_double_track_add_empty_marker(self):
        """add 时 marker 区域为空"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "\nbest quality"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("beautiful,", workflow["nodes"][0]["widgets_values"][0])

    def test_process_double_track_add_non_equivalent_match(self):
        """非等价场景下 marker 内容可在 prompt 中匹配"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// comment\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,\nbest quality"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "beautiful",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("beautiful", prompt["1"]["inputs"]["text"])

    def test_process_double_track_add_non_equivalent_no_match_raw(self):
        """非等价场景下 marker 内容无法匹配且 raw=True"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// comment\nmasterpiece,\n//#endregion hook-positive\nbest quality"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "completely different"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "add",
            "raw text",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("raw text", prompt["1"]["inputs"]["text"])

    def test_process_double_track_remove_already_commented(self):
        """remove 碰到已注释的行时保留原样 (no_skip=True)"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// beautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
            True,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertIn("// beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_process_double_track_remove_non_equivalent_match(self):
        """非等价 remove 时 target_match_content 可在 prompt 中匹配"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// comment\nbeautiful scenery,\n//#endregion hook-positive\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1",
            "remove",
            "beautiful scenery",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
            False,
            False,
            True,
        )
        self.assertTrue(result)
        self.assertNotIn("beautiful scenery", prompt["1"]["inputs"]["text"])

    def test_process_double_track_remove_no_marker_already_commented(self):
        """无 marker remove 时碰到已注释的行 (no_skip=True)"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["// beautiful scenery,\nmasterpiece"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "// beautiful scenery,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "beautiful scenery", "", "", False, True, False, False
        )
        self.assertTrue(result)
        self.assertIn("// beautiful scenery", workflow["nodes"][0]["widgets_values"][0])

    def test_process_double_track_remove_no_marker_non_matching_line(self):
        """无 marker remove 时不匹配的行保留"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "beautiful scenery,\nkeep this line,\nmasterpiece"
                    ],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "beautiful scenery,\nkeep this line,\nmasterpiece"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        result = pair.process_double_track(
            "1", "remove", "beautiful scenery", "", "", False, False, False, False
        )
        self.assertTrue(result)
        self.assertIn("keep this line", workflow["nodes"][0]["widgets_values"][0])

    def test_generate_cfg_variants_inconsistent_lengths(self):
        """不同 KSampler 节点产生不同数量的变体时抛出异常"""
        prompt = {
            "1": {"class_type": "KSampler", "inputs": {"cfg": ["2", 0]}},
            "2": {"class_type": "PrimitiveFloat", "inputs": {"value": 7.0}},
            "3": {"class_type": "KSampler", "inputs": {"cfg": ["4", 0]}},
            "4": {"class_type": "PrimitiveFloat", "inputs": {"value": 5.0}},
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "KSampler",
                    "widgets_values": [123, "fixed", 20, 7.0],
                },
                {"id": "2", "type": "PrimitiveFloat", "widgets_values": [7.0]},
                {
                    "id": "3",
                    "type": "KSampler",
                    "widgets_values": [456, "fixed", 20, 5.0],
                },
                {"id": "4", "type": "PrimitiveFloat", "widgets_values": [5.0]},
            ],
        }
        pair = WorkflowPromptPair(workflow, prompt)
        with self.assertRaises(ValueError):
            list(pair.generate_cfg_variants("x:10:1"))

    def test_update_output_filenames_date_patterns_empty(self):
        """date 模板存在但正则不匹配（如 %date:% 格式）"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "SaveImage", "widgets_values": ["output_%date:%"]},
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "output_old"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.update_output_filenames()
        self.assertEqual(prompt["1"]["inputs"]["filename_prefix"], "output_old")


# #region 图像长宽比调整测试
class TestAspectAdjustment(unittest.TestCase):
    def test_direct_ratio_adjustment(self):
        # 原始 512x512，比率 1:1
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "EmptyLatentImage",
                    "widgets_values": [512, 512, 1],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 512, "height": 512, "batch_size": 1},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        # 调整为 16:9 (1.7778)
        variants = list(pair.generate_aspect_variants("16:9"))
        self.assertEqual(len(variants), 1)
        # 512*512 = 262144. 16:9 对应 W=680, H=384 (680*384 = 261120，最接近 262144 且是 8 的倍数)
        self.assertEqual(prompt["1"]["inputs"]["width"], 680)
        self.assertEqual(prompt["1"]["inputs"]["height"], 384)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 680)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 384)

    def test_aspect_swap(self):
        # 原始 768x1344，交换后应为 1344x768
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "EmptyLatentImage",
                    "widgets_values": [768, 1344, 1],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 768, "height": 1344, "batch_size": 1},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_aspect_variants("swap"))
        self.assertEqual(len(variants), 1)
        self.assertEqual(prompt["1"]["inputs"]["width"], 1344)
        self.assertEqual(prompt["1"]["inputs"]["height"], 768)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], 1344)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 768)

    def test_aspect_step_adjustment(self):
        # 原始 768x1344 (4:7 = 0.5714), 索引为 1
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "EmptyLatentImage",
                    "widgets_values": [768, 1344, 1],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 768, "height": 1344, "batch_size": 1},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)

        # w+1 索引增 1，到达 13:19 (0.6842)。768*1344 = 1032192 -> W = sqrt(1032192 * 13 / 19) = 840.0 -> round to 8: 840; H = 1228.2 -> round to 8: 1232
        list(pair.generate_aspect_variants("w+1"))
        self.assertEqual(prompt["1"]["inputs"]["width"], 840)
        self.assertEqual(prompt["1"]["inputs"]["height"], 1232)

        # 默认不带前缀视为 w。再次 +1，到达 7:9 (0.7778) -> W=896, H=1152
        list(pair.generate_aspect_variants("+1"))
        self.assertEqual(prompt["1"]["inputs"]["width"], 896)
        self.assertEqual(prompt["1"]["inputs"]["height"], 1152)

        # h+1 高度增加，代表比例索引减少 1。此时从 7:9 退回到 13:19
        list(pair.generate_aspect_variants("h+1"))
        self.assertEqual(prompt["1"]["inputs"]["width"], 840)
        self.assertEqual(prompt["1"]["inputs"]["height"], 1232)

        # h-1 高度减少，代表比例索引增加 1。再次从 13:19 到达 7:9
        list(pair.generate_aspect_variants("h-1"))
        self.assertEqual(prompt["1"]["inputs"]["width"], 896)
        self.assertEqual(prompt["1"]["inputs"]["height"], 1152)

    def test_aspect_symmetric_variants(self):
        # 原始 1024x1024 (1:1), 索引为 4
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "EmptyLatentImage",
                    "widgets_values": [1024, 1024, 1],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 1024, "height": 1024, "batch_size": 1},
            }
        }

        # 测试 +-1 (生成 3 个变体：7:9, 1:1, 9:7)
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_aspect_variants("+-1"))
        self.assertEqual(len(variants), 3)

        # 测试 +-2:2 (生成 3 个变体：13:19, 1:1, 19:13)
        pair2 = WorkflowPromptPair(workflow, prompt)
        variants2 = list(pair2.generate_aspect_variants("w+-2:2"))
        self.assertEqual(len(variants2), 3)

    def test_aspect_primitive_connections(self):
        # width 和 height 分别连接到 PrimitiveInt 节点
        workflow = {
            "nodes": [
                {"id": "1", "type": "EmptyLatentImage", "widgets_values": [1]},
                {"id": "2", "type": "PrimitiveInt", "widgets_values": [512]},
                {"id": "3", "type": "PrimitiveInt", "widgets_values": [512]},
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": ["2", 0], "height": ["3", 0], "batch_size": 1},
            },
            "2": {"class_type": "PrimitiveInt", "inputs": {"value": 512}},
            "3": {"class_type": "PrimitiveInt", "inputs": {"value": 512}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(pair.generate_aspect_variants("16:9"))
        self.assertEqual(len(variants), 1)
        self.assertEqual(prompt["2"]["inputs"]["value"], 680)
        self.assertEqual(prompt["3"]["inputs"]["value"], 384)
        self.assertEqual(workflow["nodes"][1]["widgets_values"][0], 680)
        self.assertEqual(workflow["nodes"][2]["widgets_values"][0], 384)


# #endregion


# #endregion


if __name__ == "__main__":
    unittest.main()
