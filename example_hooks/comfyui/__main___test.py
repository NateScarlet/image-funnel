# 只允许使用项目测试脚本运行测试

import unittest
import os
import json
from PIL import Image

from .__main__ import get_relative_output_dir
from .prompt_fragment import strip_comments_for_prompt
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
                        stripped_wf = strip_comments_for_prompt(wf_text)

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
        print("Equivalence tests successfully verified on all sample PNG files!")

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
        from unittest.mock import patch
        from .__main__ import main as comfyui_main
        from .config import ComfyUIConfig
        from typing import Any

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        # 1. queue command, no images, auto trigger → exit 0, write KEEP
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        with patch("sys.argv", ["comfyui/__main__.py", "queue"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch("comfyui.__main__._write_action_override") as mock_override:
            with self.assertRaises(SystemExit) as cm:
                comfyui_main(config)
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 2. add command, no images, auto trigger → exit 0, write KEEP
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="post_commit_session",
        )
        with patch("sys.argv", ["comfyui/__main__.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch("comfyui.__main__._write_action_override") as mock_override, patch(
            "comfyui.__main__.fetch_images", return_value=[]
        ):
            with self.assertRaises(SystemExit) as cm:
                comfyui_main(config)
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 3. No trigger → ValueError
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="",
        )
        with patch("sys.argv", ["comfyui/__main__.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch("comfyui.__main__._write_action_override") as mock_override, patch(
            "comfyui.__main__.fetch_images", return_value=[]
        ):
            # Clear any IMAGE_FUNNEL_TRIGGER from os.environ for safety
            with patch.dict(os.environ, {}, clear=True):
                with self.assertRaises(ValueError) as cm:
                    comfyui_main(config)
                self.assertIn("IMAGE_FUNNEL_TRIGGER is missing", str(cm.exception))
            mock_override.assert_called_once_with("KEEP", "dummy_action_path")

        # 4. Manual trigger (note_dispatch), no images → exit 1
        config = ComfyUIConfig(
            image_paths=[],
            image_ids=[],
            action_path="dummy_action_path",
            trigger="note_dispatch",
        )
        with patch("sys.argv", ["comfyui/__main__.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch("comfyui.__main__._write_action_override") as mock_override, patch(
            "comfyui.__main__.fetch_images", return_value=[]
        ):
            with self.assertRaises(SystemExit) as cm:
                comfyui_main(config)
            self.assertEqual(cm.exception.code, 1)
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


if __name__ == "__main__":
    unittest.main()
