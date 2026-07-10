#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
workflow_prompt_pair.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# 测试文件允许访问被测模块的私有成员
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false

import unittest
import os
import json
from typing import Any, Dict, List, cast
from PIL import Image


from workflow_prompt_pair import WorkflowPromptPair
from prompt_fragment import PromptFragment
from prompt_locator import (
    get_workflow_node_text,
    get_target_clip_node,
)


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

    def test_date_filename_detection_mixed(self):
        """混合非日期变量和日期模板的文件名节点仍可检测"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
                {
                    "id": "2",
                    "type": "SaveImage",
                    "widgets_values": ["%Project.value%"],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO//20260602_000533"},
            },
            "2": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        # 只有带 %date: 的节点应被检测
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
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "beautiful"
        )
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
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "beautiful"
        )
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
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "beautiful"
        )
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
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "nonexistent"
        )
        self.assertIsNone(weight)

    def test_get_current_prompt_weight_node_not_found(self):
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([PromptFragment(pair, "999")], "test")
        self.assertIsNone(weight)

    def test_get_current_prompt_weight_bare_with_escaped_parens(self):
        """目标文本含有 `\\(medium\\)`（ComfyUI 转义括号）时能通过裸词匹配找到权重 1.0"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["oil painting \\(medium\\), other"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "oil painting \\(medium\\), other"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "oil painting \\(medium\\)"
        )
        self.assertEqual(weight, 1.0)

    def test_get_current_prompt_weight_bare_with_escaped_parens_after_weight(self):
        """文本中以 `\\(text\\)` 结尾、后面紧跟非单词字符时，`(?<!\\w)...(?!\\w)` 仍能匹配"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["\\(medium\\), "],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "\\(medium\\), "},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight(
            [PromptFragment(pair, "1")], "\\(medium\\)"
        )
        self.assertEqual(weight, 1.0)

    def test_get_current_prompt_weight_bare_normal_still_works(self):
        """普通的裸词匹配依然正常（负向环视不破坏原有功能）"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["hello world today"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "hello world today"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        # 匹配完整词
        weight = pair.get_current_prompt_weight([PromptFragment(pair, "1")], "hello")
        self.assertEqual(weight, 1.0)
        # 不匹配子串（如 "world" 中的 "or" 不应匹配）
        weight = pair.get_current_prompt_weight([PromptFragment(pair, "1")], "orl")
        self.assertIsNone(weight)

    def test_get_current_prompt_weight_escaped_parens_not_substring(self):
        """即使文本含转义括号，负向环视仍能防止子串误匹配"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["\\(medium\\)"],
                }
            ],
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "\\(medium\\)"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = pair.get_current_prompt_weight([PromptFragment(pair, "1")], "mediu")
        self.assertIsNone(weight)

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
            [PromptFragment(pair, "1")], "nonexistent", 1.5, skip_add=True
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
            [PromptFragment(pair, "1")], "beautiful", 1.5, skip_add=False
        )
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])

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
        text = pair.get_workflow_node_text("1")
        self.assertEqual(text, "hello world")

    def test_get_workflow_node_text_not_found(self):
        workflow = {"nodes": []}
        prompt = {}
        pair = WorkflowPromptPair(workflow, prompt)
        text = pair.get_workflow_node_text("999")
        self.assertIsNone(text)

    def test_get_workflow_node_text_non_string(self):
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        text = pair.get_workflow_node_text("1")
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
                [PromptFragment(pair, "1")], "nonexistent", "1.5", skip_add=True
            )
        )
        self.assertEqual(len(variants), 0)


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
        custom_dir = os.environ.get("HOOK_TEST_IMAGES_DIR")
        if custom_dir:
            self.samples_dir = custom_dir
            if not os.path.exists(self.samples_dir):
                self.fail(f"指定的测试样本目录不存在: {self.samples_dir}")
            self.png_files = [
                os.path.join(self.samples_dir, f)
                for f in os.listdir(self.samples_dir)
                if f.lower().endswith(".png")
            ]
            if not self.png_files:
                self.fail(f"指定的测试样本目录中没有 PNG 文件: {self.samples_dir}")
        else:
            self.samples_dir = os.path.join(__file__, "..", "samples")
            self.png_files = []
            if os.path.exists(self.samples_dir):
                self.png_files = [
                    os.path.join(self.samples_dir, f)
                    for f in os.listdir(self.samples_dir)
                    if f.lower().endswith(".png")
                ]

    def test_double_track_modification_flow(self):
        if not self.png_files:
            self.skipTest("没有 PNG 样本文件，跳过测试")

        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    info = img.info
                    prompt_str = info.get("prompt")
                    workflow_str = info.get("workflow")

                self.assertIsNotNone(prompt_str, f"{png_path} 缺少 prompt 元数据")
                self.assertIsNotNone(workflow_str, f"{png_path} 缺少 workflow 元数据")

                assert prompt_str is not None
                assert workflow_str is not None

                prompt = json.loads(prompt_str)
                workflow = json.loads(workflow_str)

                is_neg = False
                target_node_id = get_target_clip_node(prompt, is_neg)
                if target_node_id is None:
                    continue

                region_name = "positive"

                pair = WorkflowPromptPair(workflow, prompt)
                fragment = PromptFragment(pair, target_node_id, region=region_name)

                # --- 第一次 add "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"
                args_raw = False
                args_no_skip = False

                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None
                self.assertNotIn(
                    "// #region positive",
                    workflow_text,
                    "Start marker should not be in raw sample workflow text yet",
                )

                self.assertTrue(
                    fragment._process_double_track(
                        target_node_id,
                        "add",
                        prompt_str_arg,
                        region_name,
                        args_raw,
                        args_no_skip,
                        False,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn("// #region positive", wf_text_got)
                self.assertNotIn("// #region positive", pr_text_got)
                self.assertIn("beautiful scenery,", pr_text_got)

                # --- 第二次 add "golden sunset" ---
                prompt_str_arg = "golden sunset"

                self.assertTrue(
                    fragment._process_double_track(
                        target_node_id,
                        "add",
                        prompt_str_arg,
                        region_name,
                        args_raw,
                        args_no_skip,
                        False,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn("// #region positive", wf_text_got)
                self.assertNotIn("// #region positive", pr_text_got)
                self.assertIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第三次 remove "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"

                self.assertTrue(
                    fragment._process_double_track(
                        target_node_id,
                        "remove",
                        prompt_str_arg,
                        region_name,
                        args_raw,
                        args_no_skip,
                        True,
                    )
                )

                wf_text_got = get_workflow_node_text(pair.workflow, target_node_id)
                pr_text_got = pair.prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn("// #region positive", wf_text_got)
                self.assertNotIn("// #region positive", pr_text_got)
                self.assertNotIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第四次 remove "golden sunset" (hard=False) ---
                prompt_str_arg = "golden sunset"

                self.assertTrue(
                    fragment._process_double_track(
                        target_node_id,
                        "remove",
                        prompt_str_arg,
                        region_name,
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
        if not self.png_files:
            self.skipTest("没有 PNG 样本文件，跳过测试")

        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                # 动态查找样本中的第一个 Lora 节点名称
                lora_name = None
                for v in prompt.values():
                    if isinstance(v, dict):
                        v_dict = cast(Dict[str, Any], v)
                        if v_dict.get("class_type") == "LoraLoader":
                            ln = v_dict.get("inputs", {}).get("lora_name", "")
                            if isinstance(ln, str) and ln:
                                lora_name = ln
                                break
                        elif v_dict.get("class_type") == "Power Lora Loader (rgthree)":
                            inputs = cast(Dict[str, Any], v_dict.get("inputs", {}))
                            for k, v2 in inputs.items():
                                if k.startswith("lora_") and isinstance(v2, dict):
                                    v2_dict = cast(Dict[str, Any], v2)
                                    lora_val = v2_dict.get("lora", "")
                                    if isinstance(lora_val, str) and lora_val:
                                        lora_name = lora_val
                                        break
                            if lora_name:
                                break

                if not lora_name:
                    continue

                pair = WorkflowPromptPair(workflow, prompt)
                variants = list(pair.generate_lora_variants(lora_name, "0.99"))
                if not variants:
                    continue
                prompt, workflow = pair.prompt, pair.workflow

                query_lower = lora_name.lower()
                found_prompt_updated = False
                found_workflow_updated = False

                updated_weight = pair.get_current_lora_weight(lora_name)
                if isinstance(updated_weight, (int, float)):
                    found_prompt_updated = updated_weight == 0.99

                for node in workflow.get("nodes", []):
                    node_type = node.get("type", "")
                    wv = node.get("widgets_values", [])
                    if not isinstance(wv, list):
                        continue
                    if node_type == "LoraLoader":
                        if (
                            len(wv) >= 2
                            and isinstance(wv[0], str)
                            and query_lower in wv[0].lower()
                        ):
                            if any(
                                isinstance(v, (int, float)) and v == 0.99
                                for v in wv[1:]
                            ):
                                found_workflow_updated = True
                    elif node_type == "Power Lora Loader (rgthree)":
                        for val in wv:
                            if isinstance(val, dict):
                                lora_v = val.get("lora", "")
                                strength = val.get("strength")
                                if (
                                    isinstance(lora_v, str)
                                    and isinstance(strength, (int, float))
                                    and query_lower in lora_v.lower()
                                ):
                                    if strength == 0.99:
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
        if not self.png_files:
            self.skipTest("没有 PNG 样本文件，跳过测试")

        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                target_node_id = get_target_clip_node(prompt, is_neg=False)
                if target_node_id is None:
                    continue

                pair = WorkflowPromptPair(workflow, prompt)
                target_nodes = list(
                    pair.locate_prompts(nodes=[target_node_id], is_neg=False)
                )

                wf_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(wf_text)
                assert wf_text is not None

                # 从 workflow 文本中动态选取第一个非注释的单词作为测试目标
                target_word = None
                for line in wf_text.splitlines():
                    stripped = line.strip()
                    if (
                        stripped
                        and not stripped.startswith("//")
                        and not stripped.startswith("#")
                    ):
                        target_word = stripped.split(",")[0].strip()
                        break
                if not target_word:
                    continue

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
        self.assertEqual(prompt["1"]["inputs"]["strength_model"], 0.8)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][1], 0.8)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][2], 0.8)

    def test_modify_lora_weights_power_lora_disabled_node(self):
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
                    "mode": 2,
                    "widgets_values": [
                        {"lora": "my_style.safetensors", "strength": 0.8}
                    ],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["1"]["inputs"]["lora_1"]["strength"], 0.8)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0]["strength"], 0.8)

    def test_modify_lora_weights_no_workflow(self):
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
        workflow = {}
        pair = WorkflowPromptPair(workflow, prompt)
        pair.modify_lora_weights("my_style", 0.5)
        # 应该直接忽略而不报错
        self.assertEqual(prompt["1"]["inputs"]["strength_model"], 0.8)

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
        fragment = PromptFragment(pair, "1")
        weight = pair.get_current_prompt_weight([fragment], "test")
        self.assertIsNone(weight)

    def test_modify_prompt_weights_no_text(self):
        """节点没有文本 widget 时跳过"""
        workflow = {
            "nodes": [{"id": "1", "type": "KSampler", "widgets_values": [123, "fixed"]}]
        }
        prompt = {"1": {"class_type": "KSampler", "inputs": {}}}
        pair = WorkflowPromptPair(workflow, prompt)
        fragment = PromptFragment(pair, "1")
        pair.modify_prompt_weights([fragment], "test", 1.5, skip_add=False)
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
        fragment = PromptFragment(pair, "999")
        pair.modify_prompt_weights([fragment], "test", 1.5, skip_add=False)

    def test_modify_prompt_weights_non_string_prompt_text(self):
        """prompt 中 text 不是字符串时降级为空字符串"""
        workflow = {
            "nodes": [
                {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["hello"]}
            ]
        }
        prompt = {"1": {"class_type": "CLIPTextEncode", "inputs": {"text": 123}}}
        pair = WorkflowPromptPair(workflow, prompt)
        fragment = PromptFragment(pair, "1")
        pair.modify_prompt_weights([fragment], "nonexistent", 1.5, skip_add=False)
        self.assertIn("(nonexistent:1.5)", prompt["1"]["inputs"]["text"])

    def test_update_workflow_node_text_no_node(self):
        """节点不存在时不报错"""
        pair = WorkflowPromptPair({"nodes": []}, {})
        pair.update_workflow_node_text("999", "new text")

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


# #region 输出目录调整测试
class TestAdjustOutputDirectory(unittest.TestCase):

    def test_adjust_output_directory_simple(self):
        workflow = {
            "nodes": [
                {
                    "id": "9",
                    "type": "SaveImage",
                    "widgets_values": ["ComfyUI"],
                }
            ]
        }
        prompt = {
            "9": {"class_type": "SaveImage", "inputs": {"filename_prefix": "ComfyUI"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("sub1/sub2")
        self.assertEqual(prompt["9"]["inputs"]["filename_prefix"], "sub1/sub2/ComfyUI")
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], "sub1/sub2/ComfyUI")

    def test_adjust_output_directory_simple_already_matched(self):
        """无模板时前缀与当前目录已匹配则不变"""
        workflow = {
            "nodes": [
                {
                    "id": "9",
                    "type": "SaveImage",
                    "widgets_values": ["sub/ComfyUI"],
                }
            ]
        }
        prompt = {
            "9": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "sub/ComfyUI"},
            }
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("sub")
        self.assertEqual(prompt["9"]["inputs"]["filename_prefix"], "sub/ComfyUI")
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0], "sub/ComfyUI")

    def test_adjust_output_directory_primitive_connection(self):
        workflow = {
            "nodes": [
                {"id": "9", "type": "SaveImage", "widgets_values": []},
                {"id": "2", "type": "PrimitiveString", "widgets_values": ["ComfyUI"]},
            ]
        }
        prompt = {
            "9": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": ["2", 0]},
            },
            "2": {"class_type": "PrimitiveString", "inputs": {"value": "ComfyUI"}},
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("sub1/sub2")
        self.assertEqual(prompt["2"]["inputs"]["value"], "sub1/sub2/ComfyUI")
        self.assertEqual(workflow["nodes"][1]["widgets_values"][0], "sub1/sub2/ComfyUI")

    def test_adjust_output_directory_with_template_vars(self):
        """含非日期模板变量的节点：更新源节点值并重建 filename_prefix"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO//20260601_120000"},
                "_meta": {"title": "Save Image"},
            },
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO"},
                "_meta": {"title": "Project"},
            },
            "3": {
                "class_type": "PrimitiveString",
                "inputs": {"value": ""},
                "_meta": {"title": "Title"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("NewProject/NewTitle")

        # 源节点值被更新
        self.assertEqual(prompt["2"]["inputs"]["value"], "NewProject")
        self.assertEqual(prompt["3"]["inputs"]["value"], "NewTitle")
        # filename_prefix 被重建，日期部分保留
        self.assertEqual(
            prompt["1"]["inputs"]["filename_prefix"],
            "NewProject/NewTitle/20260601_120000",
        )
        # workflow widget 不变（模板保留）
        self.assertEqual(
            workflow["nodes"][0]["widgets_values"][0],
            "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%",
        )

    def test_adjust_output_directory_template_vars_already_matched(self):
        """模板变量值与 rel_dir 分段已匹配时不变"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "MyProj/MyTitle/20260601_120000"},
                "_meta": {"title": "Save Image"},
            },
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "MyProj"},
                "_meta": {"title": "Project"},
            },
            "3": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "MyTitle"},
                "_meta": {"title": "Title"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("MyProj/MyTitle")
        # 值不变
        self.assertEqual(prompt["2"]["inputs"]["value"], "MyProj")
        self.assertEqual(prompt["3"]["inputs"]["value"], "MyTitle")
        self.assertEqual(
            prompt["1"]["inputs"]["filename_prefix"],
            "MyProj/MyTitle/20260601_120000",
        )
        self.assertEqual(
            workflow["nodes"][0]["widgets_values"][0],
            "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%",
        )

    def test_adjust_output_directory_template_vars_count_mismatch(self):
        """rel_dir 分段数与模板变量数不匹配时将路径分隔符替换为 __"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO//20260601_120000"},
                "_meta": {"title": "Save Image"},
            },
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO"},
                "_meta": {"title": "Project"},
            },
            "3": {
                "class_type": "PrimitiveString",
                "inputs": {"value": ""},
                "_meta": {"title": "Title"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)

        # 只有一段，但模板有两个非日期变量 -> 展平路径分隔符并拼入 rel_dir
        pair.adjust_output_directory("SingleDir")
        self.assertEqual(
            prompt["1"]["inputs"]["filename_prefix"],
            "SingleDir/TODO____20260601_120000",
        )
        # workflow widget 也拼入 rel_dir
        self.assertEqual(
            workflow["nodes"][0]["widgets_values"][0],
            "SingleDir/%Project.value%__%Title.value%__%date:yyyyMMdd_hhmmss%",
        )
        # 源节点值不被更新
        self.assertEqual(prompt["2"]["inputs"]["value"], "TODO")
        self.assertEqual(prompt["3"]["inputs"]["value"], "")

    def test_adjust_output_directory_template_vars_node_not_found(self):
        """模板变量引用的节点在 prompt 中不存在时将路径分隔符替换为 __"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["%Project.value%/%date:yyyyMMdd_hhmmss%"],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO/20260601_120000"},
            },
            # 没有 _meta.title 为 Project 的节点
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("NewProj")
        # 变量源节点未找到 -> 展平并拼入 rel_dir
        self.assertEqual(
            prompt["1"]["inputs"]["filename_prefix"],
            "NewProj/TODO__20260601_120000",
        )
        self.assertEqual(
            workflow["nodes"][0]["widgets_values"][0],
            "NewProj/%Project.value%__%date:yyyyMMdd_hhmmss%",
        )

    def test_adjust_output_directory_template_vars_partial_not_found(self):
        """多个变量中部分源节点不存在时，已存在的节点不被意外修改"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO//20260601_120000"},
                "_meta": {"title": "Save Image"},
            },
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO"},
                "_meta": {"title": "Project"},
            },
            # 没有 _meta.title 为 Title 的节点
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("NewProj/NewTitle")
        # 部分缺失 -> 展平并拼入 rel_dir
        self.assertEqual(
            prompt["1"]["inputs"]["filename_prefix"],
            "NewProj/NewTitle/TODO____20260601_120000",
        )
        # Project.value 不应被修改
        self.assertEqual(prompt["2"]["inputs"]["value"], "TODO")
        # workflow widget 也拼入 rel_dir
        self.assertEqual(
            workflow["nodes"][0]["widgets_values"][0],
            "NewProj/NewTitle/%Project.value%__%Title.value%__%date:yyyyMMdd_hhmmss%",
        )

    def test_adjust_output_directory_template_vars_through_primitive(self):
        """PrimitiveString 连接时终端节点无模板，走简单回退"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
                {
                    "id": "5",
                    "type": "PrimitiveString",
                    "widgets_values": ["TODO//20260601_120000"],
                },
                {
                    "id": "6",
                    "type": "PrimitiveString",
                    "widgets_values": [""],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": ["5", 0]},
                "_meta": {"title": "Save Image"},
            },
            "5": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO//20260601_120000"},
                "_meta": {"title": "Project"},
            },
            "6": {
                "class_type": "PrimitiveString",
                "inputs": {"value": ""},
                "_meta": {"title": "Title"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("NewProj/NewTitle")
        # 源节点得到 rel_dir + basename（无模板可用，走简单回退）
        self.assertEqual(
            prompt["5"]["inputs"]["value"], "NewProj/NewTitle/20260601_120000"
        )
        # 模板变量节点不被修改
        self.assertEqual(prompt["6"]["inputs"]["value"], "")
        # 终端节点的 workflow widget 同步更新
        self.assertEqual(
            workflow["nodes"][1]["widgets_values"][0],
            "NewProj/NewTitle/20260601_120000",
        )

    def test_adjust_output_directory_template_vars_in_terminal(self):
        """终端节点（PrimitiveString）自身含模板语法时也正确重组路径"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": [],
                },
                {
                    "id": "5",
                    "type": "PrimitiveString",
                    "widgets_values": [
                        "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                    ],
                },
                {
                    "id": "6",
                    "type": "PrimitiveString",
                    "widgets_values": ["DefaultTitle"],
                },
                {
                    "id": "7",
                    "type": "PrimitiveString",
                    "widgets_values": ["DefaultProj"],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": ["5", 0]},
                "_meta": {"title": "Save Image"},
            },
            "5": {
                "class_type": "PrimitiveString",
                "inputs": {
                    "value": "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%"
                },
            },
            "6": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "DefaultTitle"},
                "_meta": {"title": "Title"},
            },
            "7": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "DefaultProj"},
                "_meta": {"title": "Project"},
            },
        }
        pair = WorkflowPromptPair(workflow, prompt)
        pair.adjust_output_directory("MyProj/MyTitle")
        # 模板变量被更新
        self.assertEqual(prompt["6"]["inputs"]["value"], "MyTitle")
        self.assertEqual(prompt["7"]["inputs"]["value"], "MyProj")
        # 终端节点得到重组后的路径，日期保留为占位符
        self.assertEqual(
            prompt["5"]["inputs"]["value"],
            "MyProj/MyTitle/%date:yyyyMMdd_hhmmss%",
        )
        # workflow widget 不变（保留模板）
        self.assertEqual(
            workflow["nodes"][1]["widgets_values"][0],
            "%Project.value%/%Title.value%/%date:yyyyMMdd_hhmmss%",
        )


# #endregion


if __name__ == "__main__":
    unittest.main()
