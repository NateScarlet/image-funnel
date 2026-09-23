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
from typing import Any, Dict
from unittest.mock import MagicMock, patch
from argparse import Namespace

from .command_handlers import (
    AddHandler,
    AdjustHandler,
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


class TestAdjustHandlerAspectVariants(unittest.TestCase):
    def test_aspect_shift_submits_each_variant_not_only_last(self) -> None:
        """+4 应按档位依次提交，而不是把生成器耗尽后重复提交末档。

        变体生成器是原地变异 + yield：每次 yield 时 prompt 处于当前档。
        若先 list() 耗尽再循环提交，prompt 已停在末档，中间档从未被提交。
        """
        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "EmptyLatentImage",
                    "widgets_values": [768, 1344, 1],
                },
                {
                    "id": "2",
                    "type": "KSampler",
                    "widgets_values": [12345, "randomize", 20, 7.0],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 768, "height": 1344, "batch_size": 1},
            },
            "2": {"class_type": "KSampler", "inputs": {"seed": 12345}},
        }
        args = Namespace(
            adjust_type="aspect",
            ratio="+4",
            update_seed=False,
            no_skip=False,
            node=None,
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

        submitted_sizes: list[tuple[int, int]] = []

        def capture_submit(p: Dict[str, Any], w: Dict[str, Any], url: str) -> None:
            submitted_sizes.append(
                (p["1"]["inputs"]["width"], p["1"]["inputs"]["height"])
            )

        with patch("comfyui.command_handlers._submit_fn", side_effect=capture_submit):
            AdjustHandler().run(ctx)

        # 768x1344 (4:7) 上 +4 → 13:19, 7:9, 1:1, 9:7 共 4 档
        self.assertEqual(
            submitted_sizes,
            [(840, 1232), (896, 1152), (1016, 1016), (1152, 896)],
        )


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
