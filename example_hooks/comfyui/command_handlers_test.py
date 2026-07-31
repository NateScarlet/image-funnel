#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
command_handlers.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import unittest
from unittest.mock import MagicMock, patch
from argparse import Namespace

from .command_handlers import AddHandler, CommandContext
from .prompt_locator import get_workflow_node_text


class TestAddHandlerMultiplePrompts(unittest.TestCase):
    def test_add_multiple_prompts_added_as_multiple_lines(self) -> None:
        """验证传入多个提示词时，是添加为多行，而不是用空格拼接在一行"""
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n// #endregion"
                    ],
                }
            ]
        }
        prompt = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,"},
            }
        }
        args = Namespace(
            node=None,
            region=None,
            neg=False,
            raw=False,
            no_skip=False,
            keep=False,
            prompt=["prompt1", "prompt2"],
        )
        ctx = CommandContext(
            img_id="img1",
            path="/path/to/img.png",
            prompt=prompt,
            workflow=workflow,
            args=args,
            comfyui_url="http://localhost:8188",
            jobs=1,
            label_to_set=None,
            history=MagicMock(),
            client=MagicMock(),
            hook_name="test_hook",
        )

        with patch("comfyui.command_handlers._submit_simple"):
            AddHandler().run(ctx)

        wf_text = get_workflow_node_text(workflow, "1")
        assert wf_text is not None
        pr_text = prompt["1"]["inputs"]["text"]

        # 校验提示词是分开添加为多行，而不是 "prompt1 prompt2,"
        self.assertNotIn("prompt1 prompt2", wf_text)
        self.assertNotIn("prompt1 prompt2", pr_text)
        self.assertIn("prompt1,", wf_text)
        self.assertIn("prompt2,", wf_text)
        self.assertIn("prompt1,\nprompt2,", wf_text)
