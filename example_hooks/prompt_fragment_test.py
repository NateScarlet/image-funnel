#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
prompt_fragment.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# 测试文件允许访问被测模块的私有成员
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import unittest

from workflow_prompt_pair import WorkflowPromptPair
from prompt_fragment import PromptFragment, strip_comments_for_prompt


class TestStripCommentsForPrompt(unittest.TestCase):
    def test_strip_comments(self):
        text = "hello\n// this is a comment\nworld\n// another comment"
        result = strip_comments_for_prompt(text)
        self.assertEqual(result, "hello\nworld")

    def test_strip_comments_empty(self):
        self.assertEqual(strip_comments_for_prompt(""), "")

    def test_strip_comments_no_comments(self):
        self.assertEqual(strip_comments_for_prompt("hello\nworld"), "hello\nworld")


class TestAdjustPromptWeightInText(unittest.TestCase):
    def test_bare_word(self):
        """裸词被转换为带权重的格式"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "hello world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_brackets(self):
        """带括号无权重的格式被转换为带权重的格式"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "(hello) world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_existing_weight(self):
        """已有权重的格式被更新"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "(hello:1.2) world", "hello", 1.5
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:1.5) world")

    def test_not_found(self):
        """未找到目标词"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "hello world", "nonexistent", 1.5
        )
        self.assertFalse(modified)
        self.assertEqual(new_text, "hello world")

    def test_negative_weight(self):
        """负权重"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "(hello:-0.5) world", "hello", -1.0
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(hello:-1.0) world")

    def test_escaped_parens(self):
        """文本含 `\\(text\\)` 转义括号时，裸词匹配仍能正确添加权重"""
        new_text, modified = PromptFragment._adjust_prompt_weight_in_text(
            "oil painting \\(medium\\), other", "oil painting \\(medium\\)", 0.9
        )
        self.assertTrue(modified)
        self.assertEqual(new_text, "(oil painting \\(medium\\):0.9), other")


class TestAddPromptToNode(unittest.TestCase):
    def _add_via_double_track(
        self, fragment, node_id, added_text, start_marker, end_marker, use_markers
    ):
        return fragment._process_double_track(
            node_id,
            "add",
            added_text,
            start_marker,
            end_marker,
            raw=False,
            no_skip=False,
            hard=False,
            use_markers=use_markers,
        )

    def test_with_marker(self):
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
        fragment = PromptFragment(
            pair, "1", "//#region hook-positive", "//#endregion hook-positive", True
        )
        self._add_via_double_track(
            fragment,
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_without_marker(self):
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
        fragment = PromptFragment(pair, "1")
        self._add_via_double_track(
            fragment,
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            False,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])

    def test_empty_marker(self):
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
        fragment = PromptFragment(
            pair, "1", "//#region hook-positive", "//#endregion hook-positive", True
        )
        self._add_via_double_track(
            fragment,
            "1",
            "(beautiful:1.5)",
            "//#region hook-positive",
            "//#endregion hook-positive",
            True,
        )
        self.assertIn("(beautiful:1.5)", workflow["nodes"][0]["widgets_values"][0])
        self.assertIn("(beautiful:1.5)", prompt["1"]["inputs"]["text"])


class TestFragmentTextAndModify(unittest.TestCase):
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


class TestDoubleTrack(unittest.TestCase):
    def setUp(self):
        self.start_marker = "//#region hook-positive"
        self.end_marker = "//#endregion hook-positive"

    def _make_fragment(self, pair, node_id, use_markers=True):
        return PromptFragment(
            pair,
            node_id,
            self.start_marker if use_markers else "",
            self.end_marker if use_markers else "",
            use_markers,
        )

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
        fragment = self._make_fragment(pair, "1", use_markers=False)
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1", use_markers=False)
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1", use_markers=False)
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1", use_markers=False)
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        fragment = self._make_fragment(pair, "999", use_markers=False)
        result = fragment._process_double_track(
            "999", "add", "test", "", "", False, False, False, False
        )
        self.assertFalse(result)

    def test_modify_weight_ignores_commented_out_prompts(self):
        """modify_weight 不应修改注释行中的提示词"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "//#region hook-positive\n// foo,\nactive_prompt,\n//#endregion hook-positive"
                    ],
                }
            ],
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "active_prompt,"}}
        }
        pair = WorkflowPromptPair(workflow, prompt)
        fragment = PromptFragment(
            pair, "1", "//#region hook-positive", "//#endregion hook-positive", True
        )

        # 尝试修改已注释提示词的权重，skip_add=True 时不应视为有效文本——应返回 False
        modified = fragment.modify_weight("foo", 1.5, skip_add=True)
        self.assertFalse(modified)
        # foo 在 workflow 中应保持注释状态不变
        self.assertIn("// foo,", workflow["nodes"][0]["widgets_values"][0])
        # active_prompt 不受影响
        self.assertIn("active_prompt", workflow["nodes"][0]["widgets_values"][0])

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
        fragment = self._make_fragment(pair, "1")
        result = fragment._process_double_track(
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
        self.assertIn("beautiful scenery", workflow["nodes"][0]["widgets_values"][0])


if __name__ == "__main__":
    unittest.main()
