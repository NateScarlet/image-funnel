# 只允许使用项目测试脚本运行测试

import logging
import tempfile
import unittest
import os
import json
from PIL import Image

logging.disable(logging.CRITICAL)

from .output_directory import (
    get_relative_output_dir,
    get_source_rel_dir,
    resolve_output_rel_dir,
)
from .prompt_comment import strip_comments
from .prompt_locator import (
    get_workflow_node_text,
    get_target_clip_node,
)


class TestComfyUIHook(unittest.TestCase):

    def setUp(self):
        # 支持通过环境变量 HOOK_TEST_IMAGES_DIR 指定测试图片目录
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
            # 未指定时使用默认 samples 目录（可选）
            self.samples_dir = os.path.join(__file__, "..", "samples")
            self.png_files = []
            if os.path.exists(self.samples_dir):
                self.png_files = [
                    os.path.join(self.samples_dir, f)
                    for f in os.listdir(self.samples_dir)
                    if f.lower().endswith(".png")
                ]

    def test_strip_comments_equivalence(self):
        # 验证剥离注释行后得到的 prompt_text 是否与原图现有的 prompt 一致
        if not self.png_files:
            self.skipTest("没有 PNG 样本文件，跳过测试")

        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    info = img.info
                    prompt_str = info.get("prompt")
                    workflow_str = info.get("workflow")

                # 用户提供的样本必须包含必要元数据，否则视为无效样本
                self.assertIsNotNone(prompt_str, f"{png_path} 缺少 prompt 元数据")
                self.assertIsNotNone(workflow_str, f"{png_path} 缺少 workflow 元数据")

                assert prompt_str is not None
                assert workflow_str is not None

                prompt = json.loads(prompt_str)
                workflow = json.loads(workflow_str)

                # 获取 workflow 中的提示词节点和 prompt 中对应的提示词节点
                # 我们这里针对所有正向和反向的 CLIPTextEncode 节点比对
                clip_node_ids = [
                    nid
                    for nid, node in prompt.items()
                    if node.get("class_type") == "CLIPTextEncode"
                ]

                for nid in clip_node_ids:
                    wf_text = get_workflow_node_text(workflow, nid)
                    pr_text = prompt[nid]["inputs"]["text"]

                    if wf_text is not None and isinstance(pr_text, str):
                        stripped_wf = strip_comments(wf_text)

                        # 比对时，我们去除连续的空行和每一行前后的空白，以忽略纯排版造成的空白差异
                        wf_lines = [
                            line.strip()
                            for line in stripped_wf.splitlines()
                            if line.strip()
                        ]
                        pr_lines = [
                            line.strip()
                            for line in pr_text.splitlines()
                            if line.strip()
                        ]

                        self.assertEqual(
                            wf_lines,
                            pr_lines,
                            f"Equivalence failed for node {nid} in {png_path}.\nWorkflow stripped:\n{wf_lines}\nPrompt actual:\n{pr_lines}",
                        )

    def test_get_target_clip_node_no_locator(self):
        # 模拟没有 KSampler 连线（即没有定位符）的情况，测试通过关键词匹配查找最佳 CLIPTextEncode
        # 1. 测试正面提示词匹配
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

        # 默认正面关键词为 masterpiece, best quality, score_7
        # node_1 匹配 2 个: masterpiece, score_7
        # node_2 匹配 1 个: best quality
        # node_3 匹配 0 个
        # 预期选中 node_1
        target = get_target_clip_node(prompt, is_neg=False)
        self.assertEqual(target, "node_1")

        # 2. 测试反向提示词匹配
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
        # 默认负面关键词为 worst quality, low quality, score_1, score_2, score_3
        # node_1 匹配 2 个: worst quality, low quality
        # node_2 匹配 1 个: score_3
        # node_3 匹配 3 个: worst quality, low quality, score_1
        # 预期选中 node_3
        target_neg = get_target_clip_node(prompt_neg, is_neg=True)
        self.assertEqual(target_neg, "node_3")

        # 3. 测试平局时，选取文本长度较长的那个
        prompt_tie = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece, short text"},
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {
                    "text": "masterpiece, this is a much longer text to break the tie"
                },
            },
        }
        # 两者都匹配 1 个: masterpiece
        # 预期选中更长的 node_2
        target_tie = get_target_clip_node(prompt_tie, is_neg=False)
        self.assertEqual(target_tie, "node_2")

        # 4. 测试自定义关键词环境变量
        os.environ["HOOK_POSITIVE_KEYWORDS"] = "custom_kw1, custom_kw2"
        prompt_custom = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "custom_kw1, simple text"},
            },
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "custom_kw1, custom_kw2"},
            },
        }
        try:
            target_custom = get_target_clip_node(prompt_custom, is_neg=False)
            self.assertEqual(target_custom, "node_2")
        finally:
            del os.environ["HOOK_POSITIVE_KEYWORDS"]

        # 5. 测试空格与下划线等价性匹配
        os.environ["HOOK_POSITIVE_KEYWORDS"] = "best quality, score_7"
        prompt_eq = {
            # 匹配 best quality (通过 best_quality) 和 score_7 (匹配 score_7) -> 匹配 2 个
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece, best_quality, score_7"},
            },
            # 仅匹配 score_7 (通过 score 7) -> 匹配 1 个
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "score 7, simple text"},
            },
        }
        try:
            target_eq = get_target_clip_node(prompt_eq, is_neg=False)
            self.assertEqual(target_eq, "node_1")
        finally:
            del os.environ["HOOK_POSITIVE_KEYWORDS"]

        os.environ["HOOK_NEGATIVE_KEYWORDS"] = "worst_quality, low quality"
        prompt_eq_neg = {
            # 仅匹配 low quality (通过 low_quality)
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "low_quality, safe text"},
            },
            # 匹配 worst_quality (通过 worst quality) 和 low quality (通过 low quality) -> 匹配 2 个
            "node_2": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "worst quality, low quality"},
            },
        }
        try:
            target_eq_neg = get_target_clip_node(prompt_eq_neg, is_neg=True)
            self.assertEqual(target_eq_neg, "node_2")
        finally:
            del os.environ["HOOK_NEGATIVE_KEYWORDS"]

    def test_main_no_images(self):
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient
        from typing import Any

        def _make_mock_client() -> MagicMock:
            return MagicMock(spec=GraphQLClient)

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        # 1. queue command, post_commit_session trigger, no images → exit 0, write KEEP
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        mock_client = _make_mock_client()
        mock_client.fetch_images.return_value = []
        with patch("sys.argv", ["comfyui/__main__.py", "queue"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(SystemExit) as cm:
                run_comfyui(mock_client, config)
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 2. remix command, no images, auto trigger → exit 0, write KEEP
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        mock_client = _make_mock_client()
        mock_client.fetch_images.return_value = []
        with patch(
            "sys.argv", ["comfyui/__main__.py", "remix", "--add", "dummy_prompt"]
        ), patch("sys.exit", side_effect=exit_side_effect), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(SystemExit) as cm:
                run_comfyui(mock_client, config)
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 3. No trigger → ValueError
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="",
        )
        mock_client = _make_mock_client()
        mock_client.fetch_images.return_value = []
        with patch(
            "sys.argv", ["comfyui/__main__.py", "remix", "--add", "dummy_prompt"]
        ), patch("sys.exit", side_effect=exit_side_effect), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(ValueError) as cm:
                run_comfyui(mock_client, config)
            self.assertIn("IMAGE_FUNNEL_TRIGGER is missing", str(cm.exception))
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 4. Manual trigger (note_dispatch), no images → exit 1
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="note_dispatch",
        )
        mock_client = _make_mock_client()
        mock_client.fetch_images.return_value = []
        with patch(
            "sys.argv", ["comfyui/__main__.py", "remix", "--add", "dummy_prompt"]
        ), patch("sys.exit", side_effect=exit_side_effect), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(SystemExit) as cm:
                run_comfyui(mock_client, config)
            self.assertEqual(cm.exception.code, 1)
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

    def test_invalid_flag_combination_keeps_directive_line(self):
        """旗标组合非法时一次变体都没发送，保留指令行让用户改写后重试。"""
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        mock_client = MagicMock(spec=GraphQLClient)
        mock_client.fetch_images.return_value = []
        with patch(
            "sys.argv",
            ["comfyui/__main__.py", "remix", "--add", "a", "b", "c", "0.8"],
        ), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(ValueError):
                run_comfyui(mock_client, config)

        mock_override.assert_called_once_with("KEEP", "dummy_action_path")

    def test_branch_missing_value_keeps_directive_line(self):
        """`--branch` 缺值在 sys.argv 级别同样报错并保留指令行。"""
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        mock_client = MagicMock(spec=GraphQLClient)
        with patch("sys.argv", ["comfyui/__main__.py", "remix", "--branch"]), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override:
            with self.assertRaises(ValueError):
                run_comfyui(mock_client, config)

        mock_override.assert_called_once_with("KEEP", "dummy_action_path")

    def test_branch_is_wired_to_output_adjustment_and_keeps_summary(self):
        """`--branch` 真正传给目录调整（`:inherit:` 也被覆盖），且摘要截断仍保留分支。"""
        import contextlib
        import io
        from typing import Any
        from unittest.mock import patch, MagicMock
        from PIL.PngImagePlugin import PngInfo
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n// #endregion"
                    ],
                },
                {
                    "id": "4",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 8.0],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece,"}},
            "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        }

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        with tempfile.TemporaryDirectory() as temp_dir:
            output_root = os.path.join(temp_dir, "output")
            os.makedirs(output_root)
            image_path = os.path.join(output_root, "img.png")
            info = PngInfo()
            info.add_text("prompt", json.dumps(prompt))
            info.add_text("workflow", json.dumps(workflow))
            Image.new("RGB", (1, 1)).save(image_path, pnginfo=info)

            config = ComfyUIConfig(
                image_paths=[],
                image_ids=[],
                action_path="dummy_action_path",
                trigger="",
                hook_output_dir=":inherit:",
                jobs=1,
            )
            mock_client = MagicMock(spec=GraphQLClient)
            mock_client.fetch_images.return_value = [("img:1", image_path)]
            fake_filename_mgr = MagicMock()
            # 目录调整的真实契约返回可选提示文本，此处无同名节点
            fake_filename_mgr.adjust_output_directory.return_value = None
            # 足够长的编辑项，确保摘要超过 40 字而被截断
            long_tag = "very_long_tag_" * 5
            stdout = io.StringIO()

            with patch(
                "sys.argv",
                ["comfyui/__main__.py", "remix", "--branch", "b1", "--add", long_tag],
            ), patch("sys.exit", side_effect=exit_side_effect), patch(
                "comfyui.__main__.FilenameManager", return_value=fake_filename_mgr
            ), patch(
                "comfyui.command_handlers._submit_fn"
            ), patch(
                "comfyui.__main__.operation_history.OperationHistory"
            ), patch.dict(
                os.environ,
                {
                    "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                    "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                    "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "",
                },
            ), contextlib.redirect_stdout(
                stdout
            ):
                with self.assertRaises(SystemExit) as cm:
                    run_comfyui(mock_client, config)
                self.assertEqual(cm.exception.code, 0)

            fake_filename_mgr.adjust_output_directory.assert_called_once_with("b1", ".")
            self.assertIn("分支 b1", stdout.getvalue())

    def test_current_uses_companion_images_without_rating_filter(self):
        """`--current` 只处理钩子上下文注入的配套图片，不按 HOOK_IMAGE_RATING 拉取目录。"""
        import contextlib
        import io
        from typing import Any
        from unittest.mock import patch, MagicMock
        from PIL.PngImagePlugin import PngInfo
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n// #endregion"
                    ],
                },
                {
                    "id": "4",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 8.0],
                },
            ]
        }
        prompt = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece,"}},
            "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        }

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        with tempfile.TemporaryDirectory() as temp_dir:
            output_root = os.path.join(temp_dir, "output")
            os.makedirs(output_root)
            image_path = os.path.join(output_root, "img.png")
            info = PngInfo()
            info.add_text("prompt", json.dumps(prompt))
            info.add_text("workflow", json.dumps(workflow))
            Image.new("RGB", (1, 1)).save(image_path, pnginfo=info)

            # 配置里的评分与图片实际评分无关：--current 根本不会走到目录评分筛选
            config = ComfyUIConfig(
                image_paths=[image_path],
                image_ids=["img:1"],
                action_path="dummy_action_path",
                trigger="note_dispatch",
                hook_output_dir=":inherit:",
                required_rating=4,
            )
            mock_client = MagicMock(spec=GraphQLClient)
            stdout = io.StringIO()

            with patch(
                "sys.argv",
                ["comfyui/__main__.py", "remix", "--current", "--add", "solosock"],
            ), patch("sys.exit", side_effect=exit_side_effect), patch(
                "comfyui.command_handlers._submit_fn"
            ) as submit, patch(
                "comfyui.__main__.operation_history.OperationHistory"
            ), patch.dict(
                os.environ,
                {
                    "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                    "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "",
                },
            ), contextlib.redirect_stdout(
                stdout
            ):
                with self.assertRaises(SystemExit) as cm:
                    run_comfyui(mock_client, config)
                self.assertEqual(cm.exception.code, 0)

            mock_client.fetch_images.assert_not_called()
            submit.assert_called_once()

    def test_current_without_companion_images_fails_and_keeps_directive(self):
        """`--current` 没有配套图片时必须报错（进程非 0 退出）并保留指令行。"""
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="note_dispatch",
        )
        mock_client = MagicMock(spec=GraphQLClient)
        with patch(
            "sys.argv", ["comfyui/__main__.py", "remix", "--current", "--add", "a"]
        ), patch("comfyui.__main__._write_action_override") as mock_override:
            with self.assertRaises(ValueError):
                run_comfyui(mock_client, config)

        mock_override.assert_called_once_with("KEEP", "dummy_action_path")
        mock_client.fetch_images.assert_not_called()

    def test_remix_without_current_still_fetches_by_rating(self):
        """不带 `--current` 时选图行为不变：仍按目录 + 评分拉取，忽略配套图片列表。"""
        from typing import Any
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        config = ComfyUIConfig(
            image_paths=["companion.png"],
            image_ids=["img:1"],
            action_path="dummy_action_path",
            trigger="post_commit_session",
            required_rating=4,
        )
        mock_client = MagicMock(spec=GraphQLClient)
        mock_client.fetch_images.return_value = []
        with patch("sys.argv", ["comfyui/__main__.py", "remix", "--add", "a"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch("comfyui.__main__._write_action_override"), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(SystemExit) as cm:
                run_comfyui(mock_client, config)
            self.assertEqual(cm.exception.code, 0)

        mock_client.fetch_images.assert_called_once_with("dir:test", "/mock/root", 4)

    def test_duplicate_title_notification_reaches_progress_update(self):
        """同名节点被同步时，入列/重混入口通过既有进度通知告知用户一次"""
        import contextlib
        import io
        from typing import Any
        from unittest.mock import patch, MagicMock
        from PIL.PngImagePlugin import PngInfo
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient

        workflow = {
            "nodes": [
                {
                    "id": "1",
                    "type": "SaveImage",
                    "widgets_values": ["%Project.value%/%date:yyyyMMdd_hhmmss%"],
                },
                {"id": "2", "type": "PrimitiveString", "widgets_values": ["TODO"]},
                {"id": "3", "type": "PrimitiveString", "widgets_values": ["TODO"]},
                {
                    "id": "5",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 8.0],
                },
            ]
        }
        prompt = {
            "1": {
                "class_type": "SaveImage",
                "inputs": {"filename_prefix": "TODO/20260601000000"},
                "_meta": {"title": "Save Image"},
            },
            "2": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO"},
                "_meta": {"title": "Project"},
            },
            "3": {
                "class_type": "PrimitiveString",
                "inputs": {"value": "TODO"},
                "_meta": {"title": "Project"},
            },
            "5": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        }

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        with tempfile.TemporaryDirectory() as temp_dir:
            output_root = os.path.join(temp_dir, "output")
            os.makedirs(output_root)
            image_path = os.path.join(output_root, "img.png")
            info = PngInfo()
            info.add_text("prompt", json.dumps(prompt))
            info.add_text("workflow", json.dumps(workflow))
            Image.new("RGB", (1, 1)).save(image_path, pnginfo=info)

            config = ComfyUIConfig(
                image_paths=[],
                image_ids=[],
                action_path="dummy_action_path",
                trigger="",
                hook_output_dir=":inherit:",
                jobs=1,
            )
            mock_client = MagicMock(spec=GraphQLClient)
            mock_client.fetch_images.return_value = [("img:1", image_path)]
            stdout = io.StringIO()

            with patch(
                "sys.argv", ["comfyui/__main__.py", "remix", "--branch", "b1"]
            ), patch("sys.exit", side_effect=exit_side_effect), patch(
                "comfyui.command_handlers._submit_fn"
            ), patch(
                "comfyui.__main__.operation_history.OperationHistory"
            ), patch(
                "comfyui.__main__._write_action_override"
            ), patch.dict(
                os.environ,
                {
                    "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                    "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                    "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "",
                },
            ), contextlib.redirect_stdout(
                stdout
            ):
                with self.assertRaises(SystemExit) as cm:
                    run_comfyui(mock_client, config)
                self.assertEqual(cm.exception.code, 0)

            bodies = [
                call.kwargs.get("body")
                for call in mock_client.send_notification.call_args_list
            ]
            self.assertTrue(
                any(
                    isinstance(body, str) and "存在同名节点 Project" in body
                    for body in bodies
                )
            )
            # 进度通知会被后续图片覆盖并在退出时撤回，最终摘要必须同样带上提示
            self.assertIn("存在同名节点 Project", stdout.getvalue())

    def test_queue_post_commit_session_fetches_unlabeled_images(self):
        from unittest.mock import patch, MagicMock
        from .__main__ import run_comfyui
        from .config import ComfyUIConfig
        from graphql_utils import GraphQLClient
        from typing import Any

        # queue 命令 + post_commit_session 触发：应从 GraphQL 拉取目录内匹配评分且未打标的图片
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
            required_rating=4,
        )
        mock_client = MagicMock(spec=GraphQLClient)
        mock_client.fetch_images.return_value = []

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        with patch("sys.argv", ["comfyui/__main__.py", "queue"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui.__main__._write_action_override"
        ) as mock_override, patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_ROOT_DIR": "/mock/root",
            },
        ):
            with self.assertRaises(SystemExit) as cm:
                run_comfyui(mock_client, config)
            self.assertEqual(cm.exception.code, 0)
            # 以空标签过滤拉取，避免已入列（已打标）图片被重复提交
            mock_client.fetch_images.assert_called_once_with(
                "dir:test", "/mock/root", 4, filter_label=[""]
            )
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")


class TestComfyUIOutputDirectory(unittest.TestCase):

    def setUp(self):
        # 备份环境变量以防污染
        self.orig_env = {
            "COMFYUI_OUTPUT_DIR": os.environ.get("COMFYUI_OUTPUT_DIR"),
            "HOOK_OUTPUT_DIR": os.environ.get("HOOK_OUTPUT_DIR"),
        }
        for k in ["COMFYUI_OUTPUT_DIR", "HOOK_OUTPUT_DIR"]:
            if k in os.environ:
                del os.environ[k]

    def tearDown(self):
        # 恢复环境变量
        for k, v in self.orig_env.items():
            if v is None:
                if k in os.environ:
                    del os.environ[k]
            else:
                os.environ[k] = v

    def test_auto_find_output_dir(self):
        img_path = os.path.normpath("C:/project/output/sub1/sub2/image.png")
        rel_dir = get_relative_output_dir(img_path, "", "")
        self.assertEqual(rel_dir, "sub1/sub2")

    def test_auto_find_output_dir_multiple_output(self):
        img_path = os.path.normpath("C:/project/output/sub1/output/sub2/image.png")
        rel_dir = get_relative_output_dir(img_path, "", "")
        self.assertEqual(rel_dir, "sub2")

    def test_missing_output_dir_raises_error(self):
        img_path = os.path.normpath("C:/project/no_out_dir/image.png")
        with self.assertRaises(ValueError):
            get_relative_output_dir(img_path, "", "")

    def test_comfyui_output_dir_valid(self):
        img_path = os.path.normpath("C:/custom_output/my_project/image.png")
        rel_dir = get_relative_output_dir(
            img_path, os.path.normpath("C:/custom_output"), ""
        )
        self.assertEqual(rel_dir, "my_project")

    def test_comfyui_output_dir_invalid(self):
        img_path = os.path.normpath("C:/other_folder/my_project/image.png")
        with self.assertRaises(ValueError):
            get_relative_output_dir(img_path, os.path.normpath("C:/custom_output"), "")

    def test_hook_output_dir_override_absolute_valid(self):
        img_path = os.path.normpath("C:/project/output/sub1/image.png")
        rel_dir = get_relative_output_dir(
            img_path, "", os.path.normpath("C:/project/output/override_dir")
        )
        self.assertEqual(rel_dir, "override_dir")

    def test_hook_output_dir_override_absolute_invalid(self):
        img_path = os.path.normpath("C:/project/output/sub1/image.png")
        with self.assertRaises(ValueError):
            get_relative_output_dir(
                img_path, "", os.path.normpath("C:/other_project/override_dir")
            )

    def test_hook_output_dir_override_relative(self):
        img_path = os.path.normpath("C:/project/output/sub1/image.png")
        rel_dir = get_relative_output_dir(img_path, "", "relative_override/sub")
        self.assertEqual(rel_dir, "relative_override/sub")

    # #region 源图当前目录

    def test_source_rel_dir_auto_find_output_dir(self):
        img_path = os.path.normpath("C:/project/output/sub1/sub2/image.png")
        self.assertEqual(get_source_rel_dir(img_path, ""), "sub1/sub2")

    def test_source_rel_dir_image_at_output_root(self):
        img_path = os.path.normpath("C:/project/output/image.png")
        self.assertEqual(get_source_rel_dir(img_path, ""), ".")

    def test_source_rel_dir_respects_explicit_output_dir(self):
        img_path = os.path.normpath("C:/custom_output/my_project/image.png")
        self.assertEqual(
            get_source_rel_dir(img_path, os.path.normpath("C:/custom_output")),
            "my_project",
        )

    def test_source_rel_dir_missing_output_dir_raises(self):
        img_path = os.path.normpath("C:/project/no_out_dir/image.png")
        with self.assertRaises(ValueError):
            get_source_rel_dir(img_path, "")

    # #endregion

    # #region --branch 落盘目录

    def test_branch_root_image_goes_into_suffix_dir(self):
        img_path = os.path.normpath("C:/project/output/image.png")
        self.assertEqual(resolve_output_rel_dir(img_path, "", "", "b1"), "b1")

    def test_branch_sub_directory_gets_sibling_named_dir(self):
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertEqual(resolve_output_rel_dir(img_path, "", "", "b1"), "sub,b1")

    def test_branch_nested_directory_keeps_parent(self):
        img_path = os.path.normpath("C:/project/output/sub1/sub2/image.png")
        self.assertEqual(resolve_output_rel_dir(img_path, "", "", "b1"), "sub1/sub2,b1")

    def test_branch_respects_explicit_comfyui_output_dir(self):
        img_path = os.path.normpath("C:/custom_output/my_project/image.png")
        self.assertEqual(
            resolve_output_rel_dir(
                img_path, os.path.normpath("C:/custom_output"), "", "b1"
            ),
            "my_project,b1",
        )

    def test_branch_overrides_relative_hook_output_dir(self):
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertEqual(
            resolve_output_rel_dir(img_path, "", "relative_override/sub", "b1"),
            "sub,b1",
        )

    def test_branch_overrides_inherit_hook_output_dir(self):
        """`:inherit:` 只关闭 HOOK_OUTPUT_DIR 的调整，`--branch` 仍然生效。"""
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertEqual(
            resolve_output_rel_dir(img_path, "", ":inherit:", "b1"), "sub,b1"
        )

    def test_branch_overrides_absolute_hook_output_dir_outside_root(self):
        """`--branch` 优先于 HOOK_OUTPUT_DIR，不再因后者非法而报错。"""
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertEqual(
            resolve_output_rel_dir(
                img_path, "", os.path.normpath("C:/other/override"), "b1"
            ),
            "sub,b1",
        )

    def test_branch_target_may_already_exist(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            output_root = os.path.join(temp_dir, "output")
            os.makedirs(os.path.join(output_root, "sub,b1"))
            img_path = os.path.join(output_root, "sub", "image.png")
            self.assertEqual(
                resolve_output_rel_dir(img_path, output_root, "", "b1"), "sub,b1"
            )

    def test_branch_requires_resolvable_output_root(self):
        img_path = os.path.normpath("C:/project/no_out_dir/image.png")
        with self.assertRaises(ValueError):
            resolve_output_rel_dir(img_path, "", "", "b1")

    def test_branch_relative_traversal_is_rejected(self):
        """`--branch ..` 不得让产物逃逸 ComfyUI 输出根目录。"""
        img_path = os.path.normpath("C:/project/output/image.png")
        with self.assertRaises(ValueError):
            resolve_output_rel_dir(img_path, "", "", "..")

    def test_branch_drive_relative_suffix_is_rejected(self):
        """盘符相对路径会替换 join 的基路径，必须在计算落盘目录时拒绝。"""
        img_path = os.path.normpath("C:/project/output/image.png")
        for suffix in ("C:", "C:foo"):
            with self.subTest(suffix=suffix):
                with self.assertRaises(ValueError):
                    resolve_output_rel_dir(img_path, "", "", suffix)

    def test_branch_dotted_suffix_is_kept(self):
        img_path = os.path.normpath("C:/project/output/image.png")
        self.assertEqual(resolve_output_rel_dir(img_path, "", "", "v1.0"), "v1.0")

    def test_inherit_without_branch_skips_output_adjustment(self):
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertIsNone(resolve_output_rel_dir(img_path, "", ":inherit:", None))

    def test_queue_without_branch_uses_image_directory(self):
        img_path = os.path.normpath("C:/project/output/sub/image.png")
        self.assertEqual(resolve_output_rel_dir(img_path, "", "", None), "sub")

    # #endregion


if __name__ == "__main__":
    unittest.main()
