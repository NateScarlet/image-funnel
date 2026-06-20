import unittest
import os
import json
from typing import List, Any, Dict, cast
from PIL import Image

# 允许直接在此目录下运行，也可以在项目根目录下运行
import sys

current_dir = os.path.dirname(os.path.abspath(__file__))
if current_dir not in sys.path:
    sys.path.append(current_dir)

from comfyui import (
    get_workflow_node_text,
    get_target_clip_node,
    process_double_track,
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

    def test_double_track_modification_flow(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                # 1. 加载真实样本元数据
                with Image.open(png_path) as img:
                    info = img.info
                    prompt_str = info.get("prompt")
                    workflow_str = info.get("workflow")

                    self.assertIsNotNone(
                        prompt_str, f"Prompt metadata missing in {png_path}"
                    )
                    self.assertIsNotNone(
                        workflow_str, f"Workflow metadata missing in {png_path}"
                    )

                    assert prompt_str is not None
                    assert workflow_str is not None

                    prompt = json.loads(prompt_str)
                    workflow = json.loads(workflow_str)

                # 2. 定位正向提示词节点（通常是 ID "1" 或其它能自动定位出的节点）
                is_neg = False
                target_node_id = get_target_clip_node(prompt, is_neg)
                self.assertIsNotNone(
                    target_node_id, f"Failed to locate target clip node in {png_path}"
                )
                assert target_node_id is not None

                node = prompt[target_node_id]

                start_marker = "//#region hook-positive"
                end_marker = "//#endregion hook-positive"

                # 辅助函数用于模拟在 workflow 结构中写回节点值
                def update_workflow_node_text_mock(
                    wf: Dict[str, Any], nid_str: str, new_text: str
                ) -> None:
                    nodes: Any = wf.get("nodes")
                    if isinstance(nodes, list):
                        for n in cast(List[Any], nodes):
                            n_dict: Dict[str, Any] = cast(Dict[str, Any], n)
                            if str(n_dict.get("id")) == nid_str:
                                widgets_values: Any = n_dict.get("widgets_values")
                                if isinstance(widgets_values, list) and widgets_values:
                                    widgets_values[0] = new_text
                                    return
                    self.fail(f"Node {nid_str} not found in workflow")

                # --- 第一次 add "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"
                args_raw = False
                args_no_skip = False

                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None
                self.assertNotIn(
                    start_marker,
                    workflow_text,
                    "Start marker should not be in raw sample workflow text yet",
                )

                prompt_text = node.get("text", "")
                if not isinstance(prompt_text, str):
                    prompt_text = ""

                # 直接调用被测模块的核心函数
                new_workflow_text, new_prompt_text = process_double_track(
                    workflow_text,
                    prompt_text,
                    "add",
                    prompt_str_arg,
                    start_marker,
                    end_marker,
                    args_raw,
                    args_no_skip,
                    False,
                )

                self.assertIsNotNone(new_workflow_text)
                self.assertIsNotNone(new_prompt_text)
                assert new_workflow_text is not None
                assert new_prompt_text is not None

                # 分别更新双轨道
                node["inputs"]["text"] = new_prompt_text
                update_workflow_node_text_mock(
                    workflow, target_node_id, new_workflow_text
                )

                # 验证第一次写入后的结果
                wf_text_got = get_workflow_node_text(workflow, target_node_id)
                pr_text_got = prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertIn("beautiful scenery,", pr_text_got)

                # --- 第二次 add "golden sunset" ---
                prompt_str_arg = "golden sunset"
                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None

                prompt_text = prompt[target_node_id]["inputs"]["text"]
                if not isinstance(prompt_text, str):
                    prompt_text = ""

                # 直接调用被测模块的核心函数
                new_workflow_text, new_prompt_text = process_double_track(
                    workflow_text,
                    prompt_text,
                    "add",
                    prompt_str_arg,
                    start_marker,
                    end_marker,
                    args_raw,
                    args_no_skip,
                    False,
                )

                self.assertIsNotNone(new_workflow_text)
                self.assertIsNotNone(new_prompt_text)
                assert new_workflow_text is not None
                assert new_prompt_text is not None

                # 分别更新双轨道
                node["inputs"]["text"] = new_prompt_text
                update_workflow_node_text_mock(
                    workflow, target_node_id, new_workflow_text
                )

                # 验证第二次写入后的结果
                wf_text_got = get_workflow_node_text(workflow, target_node_id)
                pr_text_got = prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第三次 remove "beautiful scenery" ---
                prompt_str_arg = "beautiful scenery"
                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None

                prompt_text = prompt[target_node_id]["inputs"]["text"]
                if not isinstance(prompt_text, str):
                    prompt_text = ""

                # 直接调用被测模块的核心函数
                new_workflow_text, new_prompt_text = process_double_track(
                    workflow_text,
                    prompt_text,
                    "remove",
                    prompt_str_arg,
                    start_marker,
                    end_marker,
                    args_raw,
                    args_no_skip,
                    True,
                )

                self.assertIsNotNone(new_workflow_text)
                self.assertIsNotNone(new_prompt_text)
                assert new_workflow_text is not None
                assert new_prompt_text is not None

                # 分别更新双轨道
                node["inputs"]["text"] = new_prompt_text
                update_workflow_node_text_mock(
                    workflow, target_node_id, new_workflow_text
                )

                wf_text_got = get_workflow_node_text(workflow, target_node_id)
                pr_text_got = prompt[target_node_id]["inputs"]["text"]

                self.assertIsNotNone(wf_text_got)
                assert wf_text_got is not None
                self.assertIn(start_marker, wf_text_got)
                self.assertNotIn(start_marker, pr_text_got)
                self.assertNotIn("beautiful scenery", pr_text_got)
                self.assertIn("golden sunset", pr_text_got)

                # --- 第四次 remove "golden sunset" (hard=False) ---
                prompt_str_arg = "golden sunset"
                workflow_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(workflow_text)
                assert workflow_text is not None

                prompt_text = prompt[target_node_id]["inputs"]["text"]
                if not isinstance(prompt_text, str):
                    prompt_text = ""

                new_workflow_text, new_prompt_text = process_double_track(
                    workflow_text,
                    prompt_text,
                    "remove",
                    prompt_str_arg,
                    start_marker,
                    end_marker,
                    args_raw,
                    args_no_skip,
                    False,
                )

                self.assertIsNotNone(new_workflow_text)
                self.assertIsNotNone(new_prompt_text)
                assert new_workflow_text is not None
                assert new_prompt_text is not None

                # 验证非 hard 模式下，被移除项应该在 workflow 文本中以 // 注释，且在 prompt 中被删除
                self.assertIn("// golden sunset", new_workflow_text)
                self.assertNotIn("golden sunset", new_prompt_text)

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


if __name__ == "__main__":
    unittest.main()
