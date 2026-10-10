# 只允许使用项目测试脚本运行测试

import io
import json
import logging
import os
import tempfile
import unittest
import urllib.error
import urllib.request
from contextlib import redirect_stdout
from typing import Any, Dict, List, Optional, Tuple
from unittest.mock import patch

logging.disable(logging.CRITICAL)

from .model_format import ModelFormatConfig
from .open_workflow import (
    OPEN_ROUTE_PATH,
    ROUTE_MISSING_ERROR,
    UNREACHABLE_ERROR,
    OpenRequest,
    build_request_from_env,
    build_workflow_name,
    main,
    open_workflow,
    send_workflow,
)


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


def _make_sender() -> Tuple[List[Tuple[OpenRequest, Dict[str, Any]]], Any]:
    """构造注入的发送函数 mock：记录 (request, workflow) 调用"""

    sent: List[Tuple[OpenRequest, Dict[str, Any]]] = []

    def _send(request: OpenRequest, workflow: Dict[str, Any]) -> None:
        sent.append((request, workflow))

    return sent, _send


def _make_request(image_path: str, hook_output_dir: str = "") -> OpenRequest:
    return OpenRequest(
        image_path=image_path,
        comfyui_url="http://127.0.0.1:8188",
        comfyui_output_dir="",
        hook_output_dir=hook_output_dir,
    )


class TestOpenWorkflow(unittest.TestCase):

    def test_delivers_adjusted_workflow_to_sender(self):
        """正常路径：送达的工作流已把输出目录调整为图片所在目录"""
        workflow, prompt = _make_pair_fixture("ComfyUI")
        sent, send = _make_sender()

        result = open_workflow(
            _make_request(r"C:\output\sub1\sub2\image.png"),
            _make_loader(prompt, workflow),
            send,
        )

        self.assertIn("打开", result)
        self.assertEqual(len(sent), 1)
        request, delivered = sent[0]
        self.assertEqual(request.comfyui_url, "http://127.0.0.1:8188")
        self.assertEqual(
            delivered["nodes"][0]["widgets_values"][0], "sub1/sub2/ComfyUI"
        )

    def test_inherit_passthrough_original_workflow(self):
        """:inherit: 时完全关闭目录调整，原样送达工作流"""
        workflow, prompt = _make_pair_fixture("ComfyUI")
        sent, send = _make_sender()

        open_workflow(
            _make_request(r"C:\anywhere\image.png", ":inherit:"),
            _make_loader(prompt, workflow),
            send,
        )

        self.assertEqual(sent[0][1]["nodes"][0]["widgets_values"][0], "ComfyUI")

    def test_applies_model_formatting(self):
        """与入列/复制一致：送达前按节点模型格式重排提示词"""
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
        sent, send = _make_sender()

        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": tempfile.mkdtemp()}):
            config = ModelFormatConfig.load()
            config.models["animaPencilXL_v10.safetensors"] = "anima"
            config.save()
            open_workflow(
                _make_request(r"C:\output\sub\image.png"),
                _make_loader(prompt, workflow),
                send,
            )

        self.assertEqual(
            sent[0][1]["nodes"][0]["widgets_values"][0], "masterpiece, blue hair"
        )

    def test_missing_metadata_raises_error(self):
        """无内嵌工作流的图片不适用：快速失败，不静默降级"""
        sent, send = _make_sender()
        with self.assertRaises(ValueError):
            open_workflow(
                _make_request(r"C:\out\image.png"),
                _make_loader(None, None),
                send,
            )
        self.assertEqual(sent, [])

    def test_matches_copy_workflow_output(self):
        """与复制增强产出一致：同一张图片两条路径得到相同的工作流"""
        wf_copy, prompt_copy = _make_pair_fixture("ComfyUI")
        wf_open, prompt_open = _make_pair_fixture("ComfyUI")
        sent, send = _make_sender()

        from .copy_workflow import CopyRequest, build_copy_content

        copy_result = build_copy_content(
            CopyRequest(
                image_paths=[r"C:\output\sub\image.png"],
                comfyui_output_dir="",
                hook_output_dir="",
            ),
            _make_loader(prompt_copy, wf_copy),
        )
        open_workflow(
            _make_request(r"C:\output\sub\image.png"),
            _make_loader(prompt_open, wf_open),
            send,
        )

        assert copy_result is not None
        self.assertEqual(json.loads(copy_result.content), sent[0][1])


class TestBuildWorkflowName(unittest.TestCase):

    def test_joins_directory_and_image_name(self):
        """名称 = 目录 basename + 双下划线 + 图片名（去扩展名）"""
        self.assertEqual(build_workflow_name(r"C:\output\sub\image.png"), "sub__image")

    def test_uses_containing_directory_not_ancestors(self):
        """只取图片的直接上级目录名，更上层目录不参与"""
        self.assertEqual(
            build_workflow_name(r"C:\output\sub1\sub2\image.png"), "sub2__image"
        )

    def test_strips_only_the_extension(self):
        """文件名中的其他点号保留，只剥掉最后一个扩展名"""
        self.assertEqual(
            build_workflow_name(r"C:\output\sub\my.photo.png"), "sub__my.photo"
        )


class _FakeResponse:
    """urlopen 的最小响应替身：只需支持上下文管理器与 read。"""

    def read(self) -> bytes:
        return b"{}"

    def __enter__(self) -> "_FakeResponse":
        return self

    def __exit__(self, *args: object) -> bool:
        return False


class TestSendWorkflow(unittest.TestCase):

    def _patch_urlopen(self, side_effect: BaseException):
        return patch("urllib.request.urlopen", side_effect=side_effect)

    def test_posts_workflow_with_name_to_open_route(self):
        """请求体为 {workflow, name}，name 由图片路径派生"""
        captured: Dict[str, Any] = {}

        def _urlopen(
            req: urllib.request.Request,
            *args: object,
            **kwargs: object,
        ) -> _FakeResponse:
            captured["url"] = req.full_url
            captured["method"] = req.get_method()
            captured["data"] = req.data
            return _FakeResponse()

        with patch("urllib.request.urlopen", _urlopen):
            send_workflow(_make_request(r"C:\output\sub\image.png"), {"nodes": []})

        self.assertEqual(captured["url"], f"http://127.0.0.1:8188{OPEN_ROUTE_PATH}")
        self.assertEqual(captured["method"], "POST")
        self.assertEqual(
            json.loads(captured["data"]),
            {"workflow": {"nodes": []}, "name": "sub__image"},
        )

    def test_legacy_target_reading_only_workflow_still_delivered(self):
        """旧版目标能理解的只有 workflow 字段：多余字段被忽略，请求依然有效送达"""

        def _urlopen(
            req: urllib.request.Request,
            *args: object,
            **kwargs: object,
        ) -> _FakeResponse:
            data: Any = req.data
            body = json.loads(data)
            self.assertIn("workflow", body)
            return _FakeResponse()

        with patch("urllib.request.urlopen", _urlopen):
            send_workflow(_make_request(r"C:\output\sub\image.png"), {"nodes": []})

    def test_404_raises_route_missing_error(self):
        """目标返回 404：提示 comfyui-nodes 扩展未安装/未启用"""
        error = urllib.error.HTTPError(
            url="http://127.0.0.1:8188",
            code=404,
            msg="Not Found",
            hdrs=None,  # type: ignore[arg-type]
            fp=None,
        )
        with self._patch_urlopen(side_effect=error):
            with self.assertRaises(ValueError) as ctx:
                send_workflow(_make_request(r"C:\out\image.png"), {"nodes": []})
        self.assertIn("comfyui-nodes", str(ctx.exception))

    def test_connection_error_raises_unreachable_error(self):
        """连不上目标：提示连不上 ComfyUI（与 404 文案不同）"""
        error = urllib.error.URLError("connection refused")
        with self._patch_urlopen(side_effect=error):
            with self.assertRaises(ValueError) as ctx:
                send_workflow(_make_request(r"C:\out\image.png"), {"nodes": []})
        self.assertEqual(str(ctx.exception), UNREACHABLE_ERROR)

    def test_route_missing_and_unreachable_messages_differ(self):
        """404 与连接失败是两种情况，文案必须不同以便用户采取对应行动"""
        self.assertNotEqual(ROUTE_MISSING_ERROR, UNREACHABLE_ERROR)


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
            COMFYUI_URL="http://example:8188",
            COMFYUI_OUTPUT_DIR=r"C:\comfy\output",
            HOOK_OUTPUT_DIR="custom",
        )
        with patch.dict(os.environ, env):
            request = build_request_from_env()
        self.assertEqual(request.image_path, r"C:\out\image.png")
        self.assertEqual(request.comfyui_url, "http://example:8188")
        self.assertEqual(request.comfyui_output_dir, r"C:\comfy\output")
        self.assertEqual(request.hook_output_dir, "custom")

    def test_multiple_images_raises_error(self):
        """上下文必须恰好一张图片，多值应快速失败"""
        env = dict(
            os.environ,
            IMAGE_FUNNEL_IMAGE_PATHS=json.dumps([r"C:\a.png", r"C:\b.png"]),
        )
        with patch.dict(os.environ, env):
            with self.assertRaises(ValueError):
                build_request_from_env()

    def test_main_prints_result_line(self):
        """成功时 stdout 打印一行结果描述"""
        workflow, prompt = _make_pair_fixture("ComfyUI")
        env = dict(
            os.environ,
            IMAGE_FUNNEL_IMAGE_PATHS=json.dumps([r"C:\output\sub\image.png"]),
        )
        with patch.dict(os.environ, env), patch(
            "comfyui.open_workflow.load_prompt_and_workflow",
            return_value=(prompt, workflow),
        ), patch("comfyui.open_workflow.send_workflow"):
            stdout = io.StringIO()
            with redirect_stdout(stdout):
                main()
        lines = stdout.getvalue().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertIn("打开", lines[0])


if __name__ == "__main__":
    unittest.main()
