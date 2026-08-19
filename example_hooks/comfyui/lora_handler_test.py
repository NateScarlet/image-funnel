#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
lora_handler.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false

import unittest

from .workflow_prompt_pair import WorkflowPromptPair
from .weight_manager import WeightManager
from . import variant_engine


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
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "widgets_values": ["my_style.safetensors", 0.8, 0.8],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = WeightManager(pair).get_current_lora_weight("my_style")
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
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "LoraLoader",
                    "widgets_values": ["my_style.safetensors", 0.75, 0.75],
                },
                {"id": "2", "type": "PrimitiveFloat", "widgets_values": [0.75]},
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = WeightManager(pair).get_current_lora_weight("my_style")
        self.assertEqual(weight, 0.75)

    def test_get_current_lora_weight_power_lora(self):
        prompt = {
            "1": {
                "class_type": "Power Lora Loader (rgthree)",
                "inputs": {"lora_1": {"lora": "my_style.safetensors", "strength": 0.9}},
            }
        }
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "Power Lora Loader (rgthree)",
                    "widgets_values": [
                        {"lora": "my_style.safetensors", "strength": 0.9}
                    ],
                }
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        weight = WeightManager(pair).get_current_lora_weight("my_style")
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
        weight = WeightManager(pair).get_current_lora_weight("my_style")
        self.assertIsNone(weight)

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
        weight = WeightManager(pair).get_current_lora_weight("my_style")
        self.assertIsNone(weight)

    def test_get_current_lora_weight_not_found(self):
        prompt = {}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        weight = WeightManager(pair).get_current_lora_weight("nonexistent")
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
        WeightManager(pair).modify_lora_weights("my_style", 0.5)
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
        WeightManager(pair).modify_lora_weights("my_style", 0.5)
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
        WeightManager(pair).modify_lora_weights("my_style", 0.5)
        self.assertEqual(prompt["1"]["inputs"]["lora_1"]["strength"], 0.5)
        self.assertEqual(workflow["nodes"][0]["widgets_values"][0]["strength"], 0.5)

    def test_modify_lora_weights_native_lora_missing_widgets_throws(self):
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
                    "widgets_values": [
                        "my_style.safetensors"
                    ],  # Missing indices 1 and 2
                },
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        with self.assertRaises(ValueError):
            WeightManager(pair).modify_lora_weights("my_style", 0.5)

    def test_modify_lora_weights_native_lora_primitive_invalid_widgets_throws(self):
        prompt = {
            "1": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "my_style.safetensors",
                    "strength_model": ["2", 0],
                    "strength_clip": 0.8,
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
                {
                    "id": "2",
                    "type": "PrimitiveFloat",
                    "widgets_values": [],
                },  # Empty widgets
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        with self.assertRaises(ValueError):
            WeightManager(pair).modify_lora_weights("my_style", 0.5)

    def test_generate_lora_variants_relative_no_current(self):
        """相对权重但无法获取当前值时不生成变体"""
        prompt = {}
        workflow = {"nodes": []}
        pair = WorkflowPromptPair(workflow, prompt)
        variants = list(
            variant_engine.generate_lora_variants(
                WeightManager(pair), "nonexistent", "x+0.1"
            )
        )
        self.assertEqual(len(variants), 0)


if __name__ == "__main__":
    unittest.main()
