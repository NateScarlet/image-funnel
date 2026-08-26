# 只允许使用项目测试脚本运行测试

import io
import json
import logging
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from typing import Any, Dict, Optional, Tuple
from unittest.mock import patch

logging.disable(logging.CRITICAL)

from .copy_workflow import (
    CopyRequest,
    build_copy_content,
    build_request_from_env,
    main,
)
from .model_format import ModelFormatConfig


def _make_loader(
    prompt: Optional[Dict[str, Any]],
    workflow: Optional[Dict[str, Any]],
):
    """构造注入的元数据加载器 mock：忽略路径直接返回给定元数据"""

    def _load(
        image_path: str,
    ) -> Tuple[Optional[Dict[str, Any]], Optional[Dict[str, Any]]]:
        return prompt, workflow

    return _load


def _make_pair_fixture(prefix: str) -> Tuple[Dict[str, Any], Dict[str, Any]]:
    """构造最小的 SaveImage 工作流/prompt 配对数据"""
    workflow: Dict[str, Any] = {
        "nodes": [
            {
                "id": "9",
                "type": "SaveImage",
                "widgets_values": [prefix],
            }
        ]
    }
    prompt: Dict[str, Any] = {
        "9": {"class_type": "SaveImage", "inputs": {"filename_prefix": prefix}}
    }
    return workflow, prompt


class TestBuildCopyContent(unittest.TestCase):

    def test_normal_adjustment_outputs_envelope(self):
        """正常路径：复制内容中的输出目录已调整为图片所在目录"""
        workflow, prompt = _make_pair_fixture("ComfyUI")
        request = CopyRequest(
            image_paths=[r"C:\output\sub1\sub2\image.png"],
            comfyui_output_dir="",
            hook_output_dir="",
        )
        result = build_copy_content(request, _make_loader(prompt, workflow))

        self.assertIsNotNone(result)
        assert result is not None
        # content 必须是合法 JSON 字符串且可解析回工作流结构
        content = json.loads(result.content)
        self.assertEqual(content["nodes"][0]["widgets_values"][0], "sub1/sub2/ComfyUI")
        self.assertIn("已复制", result.description)

    def test_normal_adjustment_keeps_date_template(self):
        """日期模板变量在复制结果中保持模板语法而非被求值"""
        workflow, prompt = _make_pair_fixture("C/D/%date:yyyyMMdd%image_")
        request = CopyRequest(
            image_paths=[r"C:\out\output\A\B\image.png"],
            comfyui_output_dir="",
            hook_output_dir="",
        )
        result = build_copy_content(request, _make_loader(prompt, workflow))

        self.assertIsNotNone(result)
        assert result is not None
        content = json.loads(result.content)
        # 字面目录拍平为 __ 并保留在 rel_dir 之后，日期模板保持原样
        self.assertEqual(
            content["nodes"][0]["widgets_values"][0],
            "A/B/C__D__%date:yyyyMMdd%image_",
        )

    def test_inherit_passthrough_original_workflow(self):
        """:inherit: 时完全关闭目录调整，复制原始未调整的工作流"""
        workflow, prompt = _make_pair_fixture("ComfyUI")
        request = CopyRequest(
            image_paths=[r"C:\anywhere\image.png"],
            comfyui_output_dir="",
            hook_output_dir=":inherit:",
        )
        result = build_copy_content(request, _make_loader(prompt, workflow))

        self.assertIsNotNone(result)
        assert result is not None
        content = json.loads(result.content)
        self.assertEqual(content["nodes"][0]["widgets_values"][0], "ComfyUI")

    def test_copy_applies_model_formatting(self):
        """复制增强与入列一致：复制内容按节点模型格式重排提示词。"""
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "6",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["masterpiece, Blue_Hair"],
                },
                {
                    "id": "9",
                    "type": "SaveImage",
                    "widgets_values": ["sub/ComfyUI"],
                },
            ]
        }
        prompt: Dict[str, Any] = {
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece, Blue_Hair", "clip": ["4", 0]},
            },
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
            },
            "9": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "sub/ComfyUI"},
            },
        }
        request = CopyRequest(
            image_paths=[r"C:\output\sub\image.png"],
            comfyui_output_dir="",
            hook_output_dir="",
        )

        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": tempfile.mkdtemp()}):
            config = ModelFormatConfig.load()
            config.models["animaPencilXL_v10.safetensors"] = "anima"
            config.save()
            result = build_copy_content(request, _make_loader(prompt, workflow))

        assert result is not None
        content = json.loads(result.content)
        self.assertEqual(
            content["nodes"][0]["widgets_values"][0], "masterpiece, blue hair"
        )
        self.assertEqual(content["nodes"][1]["widgets_values"][0], "sub/ComfyUI")

    def test_missing_metadata_returns_none(self):
        """无 ComfyUI 元数据的图片不适用：返回 None 表示放弃"""
        request = CopyRequest(
            image_paths=[r"C:\out\image.png"],
            comfyui_output_dir="",
            hook_output_dir="",
        )
        self.assertIsNone(build_copy_content(request, _make_loader(None, None)))
        self.assertIsNone(build_copy_content(request, _make_loader({"9": {}}, None)))

    def test_multiple_images_raises_error(self):
        """复制上下文必须恰好一张图片，多值应快速失败"""
        request = CopyRequest(
            image_paths=[r"C:\a.png", r"C:\b.png"],
            comfyui_output_dir="",
            hook_output_dir="",
        )
        with self.assertRaises(ValueError):
            build_copy_content(request, _make_loader(None, None))


class TestMainEntrypoint(unittest.TestCase):

    def test_build_request_from_env_missing_var_raises(self):
        """缺失 IMAGE_FUNNEL_IMAGE_PATHS 时快速报错"""
        env = {k: v for k, v in os.environ.items() if k != "IMAGE_FUNNEL_IMAGE_PATHS"}
        with patch.dict(os.environ, env, clear=True):
            with self.assertRaises(ValueError):
                build_request_from_env()

    def test_build_request_from_env_parses_config(self):
        env = dict(
            os.environ,
            IMAGE_FUNNEL_IMAGE_PATHS=json.dumps([r"C:\out\image.png"]),
            COMFYUI_OUTPUT_DIR=r"C:\comfy\output",
            HOOK_OUTPUT_DIR="custom",
        )
        with patch.dict(os.environ, env):
            request = build_request_from_env()
        self.assertEqual(request.image_paths, [r"C:\out\image.png"])
        self.assertEqual(request.comfyui_output_dir, r"C:\comfy\output")
        self.assertEqual(request.hook_output_dir, "custom")

    def test_main_prints_single_line_json_envelope(self):
        """成功时 stdout 为单行 JSON 信封，非 ASCII 原样保留"""
        workflow, prompt = _make_pair_fixture('ComfyUI/"引号"')
        env = dict(
            os.environ,
            IMAGE_FUNNEL_IMAGE_PATHS=json.dumps([r"C:\output\sub\image.png"]),
        )
        with patch.dict(os.environ, env), patch(
            "comfyui.copy_workflow.load_prompt_and_workflow",
            return_value=(prompt, workflow),
        ):
            stdout = io.StringIO()
            with redirect_stdout(stdout):
                main()
        lines = stdout.getvalue().splitlines()
        self.assertEqual(len(lines), 1)
        envelope = json.loads(lines[0])
        # 信封结构与转义正确：content 为非空字符串且保留原始特殊字符
        self.assertEqual(
            envelope["description"], "已复制 ComfyUI 工作流（输出目录已调整）"
        )
        content = json.loads(envelope["content"])
        # rel_dir 之外的目录分隔符拍平为 __，非 ASCII 字符原样保留
        self.assertEqual(
            content["nodes"][0]["widgets_values"][0], 'sub/ComfyUI__"引号"'
        )

    def test_main_not_applicable_prints_nothing(self):
        """元数据缺失时不适用：stdout 保持为空并正常退出"""
        env = dict(
            os.environ,
            IMAGE_FUNNEL_IMAGE_PATHS=json.dumps([r"C:\out\image.png"]),
        )
        with patch.dict(os.environ, env), patch(
            "comfyui.copy_workflow.load_prompt_and_workflow",
            return_value=(None, None),
        ):
            stdout = io.StringIO()
            with redirect_stdout(stdout):
                main()
        self.assertEqual(stdout.getvalue(), "")


if __name__ == "__main__":
    unittest.main()
