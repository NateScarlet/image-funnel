#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
command_handlers.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import copy
import os
import tempfile
import unittest
from typing import Any, Dict, List, cast
from unittest.mock import MagicMock, patch
from argparse import Namespace

from .__main__ import get_parser
from .command_handlers import RemixHandler, CommandContext
from .model_format import ModelFormatConfig
from .prompt_locator import get_workflow_node_text
from .remix import RemixRequest, from_args


def _request(argv: List[str]) -> RemixRequest:
    return from_args(get_parser().parse_args(["remix", *argv]))


def _context(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    request: RemixRequest,
    jobs: int = 1,
    history: Any = None,
) -> CommandContext:
    return CommandContext(
        img_id="img1",
        path="/path/to/img.png",
        prompt=prompt,
        workflow=workflow,
        request=request,
        comfyui_url="http://localhost:8188",
        jobs=jobs,
        label_to_set=None,
        history=history or MagicMock(),
        client=MagicMock(),
        hook_name="test_hook",
    )


def _workflow_and_prompt(
    text: str, seed_strategy: str = "fixed"
) -> tuple[Dict[str, Any], Dict[str, Any]]:
    workflow: Dict[str, Any] = {
        "nodes": [
            {"id": "1", "type": "CLIPTextEncode", "widgets_values": [text]},
            {"id": "3", "type": "EmptyLatentImage", "widgets_values": [768, 1344, 1]},
            {
                "id": "4",
                "type": "KSampler",
                "widgets_values": [12345, seed_strategy, 20, 8.0],
            },
        ]
    }
    prompt: Dict[str, Any] = {
        "1": {"class_type": "CLIPTextEncode", "inputs": {"text": text}},
        "3": {
            "class_type": "EmptyLatentImage",
            "inputs": {"width": 768, "height": 1344, "batch_size": 1},
        },
        "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
    }
    return workflow, prompt


def _submissions(patcher: Any) -> List[tuple[Dict[str, Any], Dict[str, Any]]]:
    """挂上提交捕获，返回每次提交的 (prompt, workflow) 快照列表。

    变体各自持有工作流副本，因此「实际送出了什么」只能在提交边界观察。
    """
    captured: List[tuple[Dict[str, Any], Dict[str, Any]]] = []

    def capture(p: Dict[str, Any], w: Dict[str, Any], url: str) -> None:
        captured.append((copy.deepcopy(p), copy.deepcopy(w)))

    patcher.side_effect = capture
    return captured


class TestRemixHandlerAdd(unittest.TestCase):
    def test_add_multiple_tags_added_as_multiple_lines(self) -> None:
        """传入多个提示词时，是添加为多行，而不是用空格拼接在一行。"""
        text = "// #region positive\nmasterpiece,\n// #endregion"
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(prompt, workflow, _request(["--add", "prompt1", "prompt2"]))

        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)

        self.assertEqual(len(captured), 1)
        sent_prompt, sent_workflow = captured[0]
        wf_text = get_workflow_node_text(sent_workflow, "1")
        assert wf_text is not None
        pr_text = sent_prompt["1"]["inputs"]["text"]

        self.assertNotIn("prompt1 prompt2", wf_text)
        self.assertNotIn("prompt1 prompt2", pr_text)
        self.assertIn("prompt1,\nprompt2,", wf_text)
        self.assertIn("prompt1,\nprompt2,", pr_text)

    def test_add_tags_accumulate_across_flags(self) -> None:
        """`--add a --add b` 与 `--add a b` 等价，都只产出一个变体。"""
        results = []
        for argv in (
            ["--add", "solo", "smile"],
            ["--add", "solo", "--add", "smile"],
        ):
            workflow, prompt = _workflow_and_prompt("masterpiece,")
            ctx = _context(prompt, workflow, _request(argv))
            with patch("comfyui.command_handlers._submit_fn") as submit:
                captured = _submissions(submit)
                RemixHandler().run(ctx)
            results.append((len(captured), captured[0][0]["1"]["inputs"]["text"]))

        for count, text in results:
            self.assertEqual(count, 1)
            self.assertIn("solo,", text)
            self.assertIn("smile,", text)

    def test_add_keeps_duplicate_in_other_region_unless_keep(self) -> None:
        """默认从非目标区域移除同名提示词，--keep 保留。"""
        text = (
            "// #region positive\nmasterpiece,\n// #endregion\n"
            "// #region extra\nsolosock,\n// #endregion"
        )
        for argv, expect_commented in (
            (["--add", "solosock"], True),
            (["--add", "solosock", "--keep"], False),
        ):
            with self.subTest(argv=argv):
                workflow, prompt = _workflow_and_prompt(text)
                ctx = _context(prompt, workflow, _request(argv))
                with patch("comfyui.command_handlers._submit_fn") as submit:
                    captured = _submissions(submit)
                    RemixHandler().run(ctx)

                sent_workflow = captured[0][1]
                wf_text = get_workflow_node_text(sent_workflow, "1")
                assert wf_text is not None
                extra = wf_text.split("// #region extra")[1]
                # 默认移除会把该行注释掉，--keep 则原样保留
                self.assertEqual("// solosock" in extra, expect_commented)

    def test_raw_add_skips_trailing_comma(self) -> None:
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--add", "rawtag", "--raw"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        self.assertIn("rawtag\n", get_workflow_node_text(captured[0][1], "1") or "")


class TestRemixHandlerRemove(unittest.TestCase):
    def test_remove_ignores_scope_flags(self) -> None:
        """移除总是作用于所有 CLIPTextEncode 节点，--node/--region/--neg 均被忽略。"""
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["best quality,"],
                },
                {"id": "2", "type": "CLIPTextEncode", "widgets_values": ["bad hands,"]},
                {
                    "id": "4",
                    "type": "KSampler",
                    "widgets_values": [12345, "randomize", 20, 8.0],
                },
            ]
        }
        prompt: Dict[str, Any] = {
            "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "best quality,"}},
            "2": {"class_type": "CLIPTextEncode", "inputs": {"text": "bad hands,"}},
            "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        }
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "bad hands", "--node", "1", "--neg"]),
        )
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)

        sent_prompt, sent_workflow = captured[0]
        self.assertNotIn("bad hands", sent_prompt["2"]["inputs"]["text"])
        self.assertIn("// bad hands", get_workflow_node_text(sent_workflow, "2") or "")

    def test_remove_each_tag(self) -> None:
        workflow, prompt = _workflow_and_prompt("best quality,\nbad hands,")
        ctx = _context(
            prompt, workflow, _request(["--remove", "best quality", "bad hands"])
        )
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        text = captured[0][0]["1"]["inputs"]["text"]
        self.assertNotIn("best quality", text)
        self.assertNotIn("bad hands", text)

    def test_skip_when_nothing_removed(self) -> None:
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--remove", "not_present"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)

    def test_remove_again_replays_history_ignoring_scope(self) -> None:
        workflow, prompt = _workflow_and_prompt("best quality,\nbad hands,")
        history = MagicMock()
        history.list_remove.return_value = [_remove_record("bad hands")]
        ctx = _context(prompt, workflow, _request(["--remove-again"]), history=history)
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        self.assertNotIn("bad hands", captured[0][0]["1"]["inputs"]["text"])


def _remove_record(prompt: str, raw: bool = False, hard: bool = False) -> Any:
    from .operation_history import RemoveRecord

    return RemoveRecord(prompt=prompt, raw=raw, hard=hard)


class TestRemixHandlerVariants(unittest.TestCase):
    def test_each_variant_submitted_from_baseline(self) -> None:
        """相对权重逐档都基于基线值，且每个变体都完整带上其它编辑。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(
            prompt,
            workflow,
            _request(["--add", "newtag", "--cfg", "x-1", "--cfg", "x-2"]),
        )
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)

        self.assertEqual(len(captured), 2)
        for sent_prompt, _sent_workflow in captured:
            self.assertIn("newtag,", sent_prompt["1"]["inputs"]["text"])
        # 第二档的 CFG 基于基线 8.0 而不是第一档的 7.0
        self.assertEqual(
            [sent_prompt["4"]["inputs"]["cfg"] for sent_prompt, _ in captured],
            [7.0, 6.0],
        )

    def test_jobs_multiplies_submissions(self) -> None:
        """3 档 CFG × 2 个 job = 6 次提交。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(
            prompt,
            workflow,
            _request(["--cfg", "5:7:1"]),
            jobs=2,
        )
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)

        self.assertEqual(len(captured), 6)
        self.assertEqual(
            [sent_prompt["4"]["inputs"]["cfg"] for sent_prompt, _ in captured],
            [5.0, 5.0, 6.0, 6.0, 7.0, 7.0],
        )

    def test_no_edit_flags_submits_once(self) -> None:
        """无编辑旗标的重混等价于直接入列。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request([]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_called_once()
        self.assertFalse(ctx.skipped)

    def test_prompt_weight_edit_applies_to_single_tag(self) -> None:
        """`--add <tag> <weight>` 的权重只作用于该 tag，不新增标签。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,\nbest quality,")
        ctx = _context(prompt, workflow, _request(["--add", "masterpiece", "0.5"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        text = captured[0][0]["1"]["inputs"]["text"]
        self.assertIn("(masterpiece:0.5)", text)
        self.assertNotIn("masterpiece,\nmasterpiece", text)


class TestRemixSeedPolicy(unittest.TestCase):
    """种子更新：内容类编辑必定更新，纯参数调整只在需要多次提交时更新。"""

    def _seeds(self, argv: List[str], jobs: int = 1) -> List[float]:
        workflow, prompt = _workflow_and_prompt(
            "masterpiece,", seed_strategy="randomize"
        )
        ctx = _context(prompt, workflow, _request(argv), jobs=jobs)
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        self.assertTrue(captured, "本次请求应至少提交一次")
        return [cast(float, p["4"]["inputs"]["seed"]) for p, _ in captured]

    def test_requeue_always_updates_seed(self) -> None:
        self.assertNotEqual(self._seeds([])[0], 12345)

    def test_content_edits_always_update_seed(self) -> None:
        """新增提示词在单次提交下也必定换种子，否则提交出去的是同一张图。"""
        self.assertNotEqual(self._seeds(["--add", "solosock"])[0], 12345)

    def test_single_parameter_adjustment_keeps_seed(self) -> None:
        """纯参数调整单档单次提交时保持基线种子，便于同一噪波下对照。"""
        self.assertEqual(self._seeds(["--cfg", "6"])[0], 12345)

    def test_multiple_variants_force_seed_update(self) -> None:
        """多档变体之间必须换种子，否则各变体退化成同一张图。"""
        seeds = self._seeds(["--cfg", "6", "--cfg", "7"])
        self.assertEqual(len(seeds), 2)
        self.assertNotEqual(seeds[0], seeds[1])

    def test_multiple_jobs_force_seed_update(self) -> None:
        self.assertNotEqual(self._seeds(["--cfg", "6"], jobs=2)[0], 12345)

    def test_update_seed_flag_forces_it(self) -> None:
        self.assertNotEqual(self._seeds(["--cfg", "6", "--update-seed"])[0], 12345)


class TestSetModelFormatCommand(unittest.TestCase):
    def test_set_model_format_persists_disabled(self) -> None:
        """`/set-model-format <model> disabled` 应写入全局配置并保留 disabled 值。"""
        tmp_dir = tempfile.mkdtemp()

        args = Namespace(model="someModel.safetensors", format="disabled")
        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": tmp_dir}):
            from .command_handlers import handle_set_model_format_cmd

            handle_set_model_format_cmd(args)

            reloaded = ModelFormatConfig.load()
            self.assertEqual(reloaded.models.get("someModel.safetensors"), "disabled")

    def test_set_model_format_requires_both_arguments(self) -> None:
        """缺少 model 或 format 参数时快速失败。"""
        from .command_handlers import handle_set_model_format_cmd

        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="", format="anima"))
        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="m.safetensors", format=""))
