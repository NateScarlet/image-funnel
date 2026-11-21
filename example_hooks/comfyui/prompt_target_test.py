# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownMemberType=false, reportUnknownVariableType=false, reportArgumentType=false

"""目标定位 seam 的不变量测试：规则收敛在 prompt_target 深 module 后，
提交路径与补全路径共用同一判定入口。"""

import argparse
import json
import os
import shutil
import tempfile
import unittest
from typing import Any, Dict, Optional, Sequence

from PIL import Image
from PIL.PngImagePlugin import PngInfo

from test_support import pin_tempdir

pin_tempdir()

from . import remix
from .autocomplete import _load_workflow_data
from .prompt_target import (
    clip_text_encode_node_ids,
    extract_region_names,
    primary_target,
    region_names_in_text,
    target_specs,
)
from .workflow_prompt_pair import WorkflowPromptPair


def _prompt_node(text: str) -> Dict[str, Any]:
    return {"class_type": "CLIPTextEncode", "inputs": {"text": text}}


def _workflow_node(node_id: str, text: str) -> Dict[str, Any]:
    return {"id": node_id, "widgets_values": [text]}


def _primary_node_id(pair: WorkflowPromptPair, is_neg: bool) -> str:
    """在 seam 上取默认目标节点 id，缺失即视为测试失败。"""
    fragment = primary_target(list(pair.locate_prompts(is_neg=is_neg)))
    assert fragment is not None
    return fragment.node_id


class TestTargetSpecs(unittest.TestCase):
    """显式目标优先于默认区域，默认区域按 is_neg 选极性。"""

    def test_explicit_nodes_then_regions_then_default(self):
        self.assertEqual(
            target_specs(["n2", "n1"], ["custom"], False),
            [("node", "n2"), ("node", "n1"), ("region", "custom")],
        )
        self.assertEqual(target_specs([], [], False), [("region", "positive")])
        self.assertEqual(target_specs([], [], True), [("region", "negative")])


class TestTargetLocation(unittest.TestCase):
    """在 locate_prompts seam 上断言定位规则。"""

    def test_explicit_node_wins_over_default_region(self):
        prompt = {"n1": _prompt_node("a, masterpiece"), "n2": _prompt_node("b")}
        pair = WorkflowPromptPair({"nodes": []}, prompt)
        fragments = list(pair.locate_prompts(nodes=["n2"], is_neg=False))
        self.assertEqual([(f.node_id, f.region) for f in fragments], [("n2", "")])

    def test_region_target_matches_workflow_marker(self):
        prompt = {"n1": _prompt_node("plain text")}
        workflow = {
            "nodes": [
                _workflow_node("n1", "// #region custom\nmasterpiece\n// #endregion\n")
            ]
        }
        pair = WorkflowPromptPair(workflow, prompt)
        fragments = list(pair.locate_prompts(regions=["custom"]))
        self.assertEqual([(f.node_id, f.region) for f in fragments], [("n1", "custom")])

    def test_region_target_falls_back_to_default_node(self):
        prompt = {"n1": _prompt_node("masterpiece, score_7"), "n2": _prompt_node("x")}
        pair = WorkflowPromptPair({"nodes": []}, prompt)
        fragments = list(pair.locate_prompts(regions=["custom"], is_neg=False))
        self.assertEqual([(f.node_id, f.region) for f in fragments], [("n1", "custom")])

    def test_default_region_selects_node_by_polarity_keywords(self):
        prompt = {
            "n1": _prompt_node("masterpiece, best quality, score_7"),
            "n2": _prompt_node("simple background"),
            "n3": _prompt_node("worst quality, low quality, score_1"),
        }
        pair = WorkflowPromptPair({"nodes": []}, prompt)
        self.assertEqual(_primary_node_id(pair, is_neg=False), "n1")
        self.assertEqual(_primary_node_id(pair, is_neg=True), "n3")

    def test_default_region_traces_ksampler_and_picks_longest(self):
        prompt = {
            "n1": _prompt_node("short"),
            "n2": _prompt_node("masterpiece, a much longer positive prompt text"),
            "sampler": {
                "class_type": "KSampler",
                "inputs": {
                    "positive": ["concat", 0],
                    "negative": ["n1", 0],
                },
            },
            "concat": {
                "class_type": "ConditioningConcat",
                "inputs": {
                    "conditioning_to": ["n1", 0],
                    "conditioning_from": ["n2", 0],
                },
            },
        }
        pair = WorkflowPromptPair({"nodes": []}, prompt)
        self.assertEqual(_primary_node_id(pair, is_neg=False), "n2")
        self.assertEqual(_primary_node_id(pair, is_neg=True), "n1")

    def test_default_region_keyword_env_override_and_tie_break(self):
        previous = os.environ.get("HOOK_POSITIVE_KEYWORDS")
        os.environ["HOOK_POSITIVE_KEYWORDS"] = "custom_kw1, custom_kw2"
        try:
            prompt = {
                "n1": _prompt_node("custom_kw1, simple text"),
                "n2": _prompt_node("custom_kw1, custom_kw2"),
            }
            pair = WorkflowPromptPair({"nodes": []}, prompt)
            self.assertEqual(_primary_node_id(pair, is_neg=False), "n2")
        finally:
            if previous is None:
                del os.environ["HOOK_POSITIVE_KEYWORDS"]
            else:
                os.environ["HOOK_POSITIVE_KEYWORDS"] = previous


class TestGlobalRemovalTarget(unittest.TestCase):
    """移除是全局操作，作用于全部 CLIPTextEncode 节点。"""

    def test_clip_text_encode_node_ids_covers_all_nodes(self):
        prompt: Dict[str, Any] = {
            "n1": _prompt_node("a"),
            "sampler": {"class_type": "KSampler", "inputs": {}},
            "n2": _prompt_node("b"),
            "n3": _prompt_node("c"),
        }
        self.assertEqual(clip_text_encode_node_ids(prompt), ["n1", "n2", "n3"])

    def test_primary_target_is_first_or_none(self):
        self.assertEqual(primary_target([1, 2, 3]), 1)
        self.assertIsNone(primary_target([]))


class TestRegionExtraction(unittest.TestCase):
    """区域标记提取只在深 module 定义一次。"""

    def test_region_names_in_text_keeps_scan_order_and_dedups(self):
        text = (
            "// #region negative\nworst quality\n// #endregion\n"
            "// #region custom\ntag\n// #endregion\n"
            "// #region negative\nagain\n// #endregion\n"
        )
        self.assertEqual(region_names_in_text(text), ["negative", "custom"])

    def test_extract_region_names_ranks_preferred_then_scan_order(self):
        # 标记写在非首个 widget 字符串里也要被扫到（与收敛前行为一致）
        workflow = {
            "nodes": [
                {
                    "id": 1,
                    "widgets_values": [
                        "plain text",
                        "// #region negative\n// #endregion\n// #region custom\n// #endregion\n",
                    ],
                },
                {
                    "id": 2,
                    "widgets_values": ["// #region positive\n// #endregion\n"],
                },
            ]
        }
        self.assertEqual(
            list(extract_region_names([workflow])), ["positive", "negative", "custom"]
        )


class TestCommitCompletionConsistency(unittest.TestCase):
    """同一修饰参数与工作流，提交路径和补全路径派生出相同目标。"""

    def _sample_path(
        self, prompt: Dict[str, Any], workflow: Dict[str, Any], tmp_dir: str
    ) -> str:
        image = Image.new("RGB", (1, 1), "white")
        meta = PngInfo()
        meta.add_text("prompt", json.dumps(prompt))
        meta.add_text("workflow", json.dumps(workflow))
        path = os.path.join(tmp_dir, "sample.png")
        image.save(path, pnginfo=meta)
        return path

    def _assert_consistent(
        self,
        nodes: Optional[Sequence[str]],
        regions: Optional[Sequence[str]],
    ) -> None:
        prompt = {
            "n1": _prompt_node("masterpiece, score_7"),
            "n2": _prompt_node("worst quality, score_1"),
        }
        workflow = {
            "nodes": [
                _workflow_node("n1", "// #region custom\nmasterpiece\n// #endregion\n")
            ]
        }
        tmp_dir = tempfile.mkdtemp()
        try:
            path = self._sample_path(prompt, workflow, tmp_dir)
            parsed = argparse.Namespace(
                neg=False,
                region=regions,
                node=nodes,
                raw=False,
                hard=False,
                no_skip=False,
            )
            _, _, _, target, _ = _load_workflow_data([path], parsed, "add")

            pair = WorkflowPromptPair(workflow, prompt)
            commit_fragments = remix.locate_fragments(
                pair,
                remix.RemixRequest(
                    add_tags=("tag",),
                    nodes=tuple(nodes or []),
                    regions=tuple(regions or []),
                ),
            )
            self.assertTrue(commit_fragments)
            assert target is not None
            self.assertEqual(target.node_id, commit_fragments[0].node_id)
            self.assertEqual(
                {(f.node_id, f.region) for f in commit_fragments},
                {("n1", "custom")} if regions else {("n2", "")},
            )
        finally:
            shutil.rmtree(tmp_dir, ignore_errors=True)

    def test_region_target_consistent(self):
        self._assert_consistent(nodes=None, regions=["custom"])

    def test_explicit_node_target_consistent(self):
        self._assert_consistent(nodes=["n2"], regions=None)


if __name__ == "__main__":
    unittest.main()
