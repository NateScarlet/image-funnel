#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
command_handlers.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import os
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from argparse import Namespace

from .command_handlers import (
    AddHandler,
    CommandContext,
    handle_set_model_format_cmd,
)
from .model_format import ModelFormatConfig
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


class TestSetModelFormatCommand(unittest.TestCase):
    def test_set_model_format_persists_disabled(self) -> None:
        """`/set-model-format <model> disabled` 应写入全局配置并保留 disabled 值。"""
        tmp_dir = tempfile.mkdtemp()

        args = Namespace(model="someModel.safetensors", format="disabled")
        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": tmp_dir}):
            handle_set_model_format_cmd(args)

            reloaded = ModelFormatConfig.load()
            self.assertEqual(reloaded.models.get("someModel.safetensors"), "disabled")

    def test_set_model_format_requires_both_arguments(self) -> None:
        """缺少 model 或 format 参数时快速失败。"""
        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="", format="anima"))
        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="m.safetensors", format=""))
