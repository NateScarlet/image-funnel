import unittest
import os
import json
from PIL import Image

# 允许直接在此目录下运行，也可以在项目根目录下运行
import sys

current_dir = os.path.dirname(os.path.abspath(__file__))
if current_dir not in sys.path:
    sys.path.append(current_dir)

from comfyui import (
    get_workflow_node_text,
    get_target_clip_node,
    strip_comments_for_prompt,
)


class TestComfyUIHook(unittest.TestCase):

    def setUp(self):
        # 寻找 samples 目录的绝对路径（其在测试目录下）
        self.samples_dir = os.path.join(current_dir, "samples")
        self.assertTrue(
            os.path.exists(self.samples_dir),
            f"Samples directory not found at: {self.samples_dir}",
        )

        # 扫描该目录下的所有 .png 文件
        self.png_files = [
            os.path.join(self.samples_dir, f)
            for f in os.listdir(self.samples_dir)
            if f.lower().endswith(".png")
        ]
        self.assertTrue(
            len(self.png_files) > 0, "No PNG sample files found in samples directory"
        )

    def test_strip_comments_equivalence(self):
        # 验证剥离注释行后得到的 prompt_text 是否与原图现有的 prompt 一致
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    info = img.info
                    prompt_str = info.get("prompt")
                    workflow_str = info.get("workflow")

                    self.assertIsNotNone(prompt_str)
                    self.assertIsNotNone(workflow_str)

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
        import comfyui
        from typing import Any

        def exit_side_effect(code: Any = 0) -> None:
            raise SystemExit(code)

        # 1. 验证 queue 指令没有输入图片，但在有服务端自动（非手动）触发标识时，以 0 退出且写入 KEEP
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
                "IMAGE_FUNNEL_IMAGE_IDS": "[]",
                "IMAGE_FUNNEL_ACTION": "dummy_action_path",
                "IMAGE_FUNNEL_TRIGGER": "post_commit_session",
            },
        ), patch("sys.argv", ["comfyui.py", "queue"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui._write_action_override"
        ) as mock_override:

            with self.assertRaises(SystemExit) as cm:
                comfyui.main()
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP")

        # 2. 验证 add 指令没有输入图片，但在有服务端自动（非手动）触发标识时，以 0 退出且写入 KEEP
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
                "IMAGE_FUNNEL_IMAGE_IDS": "[]",
                "IMAGE_FUNNEL_ACTION": "dummy_action_path",
                "IMAGE_FUNNEL_TRIGGER": "post_commit_session",
            },
        ), patch("sys.argv", ["comfyui.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui._write_action_override"
        ) as mock_override, patch(
            "comfyui.fetch_images", return_value=[]
        ):

            with self.assertRaises(SystemExit) as cm:
                comfyui.main()
            self.assertEqual(cm.exception.code, 0)
            mock_override.assert_called_once_with("KEEP")

        # 3. 验证没有服务端触发标识（例如本地命令行直接运行）且没有图片时，直接抛出 ValueError 报错
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
                "IMAGE_FUNNEL_IMAGE_IDS": "[]",
                "IMAGE_FUNNEL_ACTION": "dummy_action_path",
            },
        ), patch("sys.argv", ["comfyui.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui._write_action_override"
        ) as mock_override, patch(
            "comfyui.fetch_images", return_value=[]
        ):

            # 清除可能存在的 IMAGE_FUNNEL_TRIGGER 保证测试纯净
            if "IMAGE_FUNNEL_TRIGGER" in os.environ:
                del os.environ["IMAGE_FUNNEL_TRIGGER"]

            with self.assertRaises(ValueError) as cm:
                comfyui.main()
            self.assertIn("IMAGE_FUNNEL_TRIGGER is missing", str(cm.exception))
            mock_override.assert_not_called()

        # 4. 验证虽然有服务端触发标识，但是为手动派发（如 note_dispatch）触发且没有图片时，依旧以 1 报错退出
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
                "IMAGE_FUNNEL_IMAGE_IDS": "[]",
                "IMAGE_FUNNEL_ACTION": "dummy_action_path",
                "IMAGE_FUNNEL_TRIGGER": "note_dispatch",
            },
        ), patch("sys.argv", ["comfyui.py", "add", "dummy_prompt"]), patch(
            "sys.exit", side_effect=exit_side_effect
        ), patch(
            "comfyui._write_action_override"
        ) as mock_override, patch(
            "comfyui.fetch_images", return_value=[]
        ):

            with self.assertRaises(SystemExit) as cm:
                comfyui.main()
            self.assertEqual(cm.exception.code, 1)
            mock_override.assert_not_called()


if __name__ == "__main__":
    unittest.main()
