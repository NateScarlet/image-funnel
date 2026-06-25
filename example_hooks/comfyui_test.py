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
    modify_lora_weights,
    modify_prompt_weights,
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

    def test_adjust_lora_weights(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                # 两张样本图片均包含 "Power Lora Loader (rgthree)"
                lora_keywords = ["evanescia", "semi-nffa", "cunny_funky"]
                target_keyword = None
                for kw in lora_keywords:
                    # 检查样本中是否含有该 lora
                    for v in prompt.values():
                        if isinstance(v, dict):
                            v_dict = cast(Dict[str, Any], v)
                            if (
                                v_dict.get("class_type")
                                == "Power Lora Loader (rgthree)"
                            ):
                                inputs = cast(Dict[str, Any], v_dict.get("inputs", {}))
                                for k2, v2 in inputs.items():
                                    if k2.startswith("lora_") and isinstance(v2, dict):
                                        v2_dict = cast(Dict[str, Any], v2)
                                        lora_val = v2_dict.get("lora", "")
                                        if (
                                            isinstance(lora_val, str)
                                            and kw in lora_val.lower()
                                        ):
                                            target_keyword = kw
                                            break

                self.assertIsNotNone(
                    target_keyword, f"No expected Lora keyword found in {png_path}"
                )
                assert target_keyword is not None

                # 修改 Lora 权重
                variants = list(
                    modify_lora_weights(prompt, workflow, target_keyword, "0.99")
                )
                self.assertTrue(len(variants) > 0)
                prompt, workflow = variants[0]

                # 验证修改结果
                found_prompt_updated = False
                found_workflow_updated = False
                for v in prompt.values():
                    if isinstance(v, dict):
                        v_dict = cast(Dict[str, Any], v)
                        if v_dict.get("class_type") == "Power Lora Loader (rgthree)":
                            inputs = cast(Dict[str, Any], v_dict.get("inputs", {}))
                            for k2, v2 in inputs.items():
                                if k2.startswith("lora_") and isinstance(v2, dict):
                                    v2_dict = cast(Dict[str, Any], v2)
                                    lora_val = v2_dict.get("lora", "")
                                    if (
                                        isinstance(lora_val, str)
                                        and target_keyword in lora_val.lower()
                                    ):
                                        self.assertEqual(v2_dict.get("strength"), 0.99)
                                        found_prompt_updated = True

                for node in workflow.get("nodes", []):
                    node_dict = cast(Dict[str, Any], node)
                    if node_dict.get("type") == "Power Lora Loader (rgthree)":
                        widgets_values = node_dict.get("widgets_values", [])
                        if isinstance(widgets_values, list):
                            widgets_values_list = cast(List[Any], widgets_values)
                            for val in widgets_values_list:
                                if isinstance(val, dict) and "lora" in val:
                                    val_dict = cast(Dict[str, Any], val)
                                    lora_val = val_dict.get("lora", "")
                                    if (
                                        isinstance(lora_val, str)
                                        and target_keyword in lora_val.lower()
                                    ):
                                        self.assertEqual(val_dict.get("strength"), 0.99)
                                        found_workflow_updated = True

                self.assertTrue(found_prompt_updated)
                self.assertTrue(found_workflow_updated)

    def test_adjust_lora_weights_native_and_primitive(self):
        # 伪造一个原生的 LoraLoader 节点，且其权重通过 PrimitiveFloat 连接
        prompt: Dict[str, Any] = {
            "node_lora": {
                "class_type": "LoraLoader",
                "inputs": {
                    "lora_name": "style_test.safetensors",
                    "strength_model": ["node_prim", 0],
                    "strength_clip": ["node_prim", 0],
                },
            },
            "node_prim": {"class_type": "PrimitiveFloat", "inputs": {"value": 1.0}},
        }
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "node_lora",
                    "type": "LoraLoader",
                    "widgets_values": ["style_test.safetensors", 1.0, 1.0],
                },
                {"id": "node_prim", "type": "PrimitiveFloat", "widgets_values": [1.0]},
            ]
        }

        variants = list(modify_lora_weights(prompt, workflow, "style_test", "-0.75"))
        self.assertTrue(len(variants) > 0)
        prompt, workflow = variants[0]

        # 验证 prompt 侧被连接的 Primitive 节点值是否被修改为 -0.75
        node_prim = cast(Dict[str, Any], prompt["node_prim"])
        prim_inputs = cast(Dict[str, Any], node_prim["inputs"])
        self.assertEqual(prim_inputs["value"], -0.75)
        # 验证 workflow 侧的 Primitive 节点的 widgets_values[0] 是否被修改为 -0.75
        nodes_list = cast(List[Dict[str, Any]], workflow["nodes"])
        for node in nodes_list:
            if node["id"] == "node_prim":
                widgets_values = node["widgets_values"]
                if isinstance(widgets_values, list):
                    self.assertEqual(widgets_values[0], -0.75)
            elif node["id"] == "node_lora":
                self.assertEqual(node["widgets_values"][1], -0.75)
                self.assertEqual(node["widgets_values"][2], -0.75)

    def test_adjust_prompt_weights(self):
        for png_path in self.png_files:
            with self.subTest(png_path=png_path):
                with Image.open(png_path) as img:
                    prompt = json.loads(img.info["prompt"])
                    workflow = json.loads(img.info["workflow"])

                # 定位正向提示词节点
                target_node_id = get_target_clip_node(prompt, is_neg=False)
                self.assertIsNotNone(target_node_id)
                assert target_node_id is not None

                target_nodes = [
                    (
                        target_node_id,
                        "//#region hook-positive",
                        "//#endregion hook-positive",
                        True,
                    )
                ]

                # 检查是否存在 "score_7"
                wf_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(wf_text)
                assert wf_text is not None

                target_word = "score_7" if "score_7" in wf_text else "masterpiece"
                self.assertIn(target_word, wf_text)

                modified = modify_prompt_weights(
                    prompt, workflow, target_nodes, target_word, "1.35", skip_add=True
                )
                variants = list(modified)
                self.assertTrue(len(variants) > 0)
                prompt, workflow = variants[0]

                new_wf_text = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text)
                assert new_wf_text is not None
                self.assertIn(f"({target_word}:1.35)", new_wf_text)

                new_pr_text = prompt[target_node_id]["inputs"]["text"]
                self.assertIn(f"({target_word}:1.35)", new_pr_text)

                # 再次修改：应该支持在带权重的括号中继续修改
                modified2 = modify_prompt_weights(
                    prompt, workflow, target_nodes, target_word, "-0.5", skip_add=True
                )
                variants2 = list(modified2)
                self.assertTrue(len(variants2) > 0)
                prompt, workflow = variants2[0]

                new_wf_text2 = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text2)
                assert new_wf_text2 is not None
                self.assertIn(f"({target_word}:-0.5)", new_wf_text2)

                # 测试不存在的词，且 skip_add=True -> 应不修改，生成器为空
                modified_skip = modify_prompt_weights(
                    prompt,
                    workflow,
                    target_nodes,
                    "non_existent_word_abc",
                    "1.5",
                    skip_add=True,
                )
                variants_skip = list(modified_skip)
                self.assertEqual(len(variants_skip), 0)

                # 测试不存在的词，且 skip_add=False -> 应修改成功，生成器非空且添加该词
                modified_add = modify_prompt_weights(
                    prompt,
                    workflow,
                    target_nodes,
                    "non_existent_word_abc",
                    "1.5",
                    skip_add=False,
                )
                variants_add = list(modified_add)
                self.assertTrue(len(variants_add) > 0)
                prompt, workflow = variants_add[0]

                new_wf_text3 = get_workflow_node_text(workflow, target_node_id)
                self.assertIsNotNone(new_wf_text3)
                assert new_wf_text3 is not None
                self.assertIn("(non_existent_word_abc:1.5)", new_wf_text3)


if __name__ == "__main__":
    unittest.main()
