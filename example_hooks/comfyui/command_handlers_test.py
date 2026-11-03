#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
command_handlers.py 的单元测试。
"""

# 只允许使用项目测试脚本运行测试
# pyright: reportPrivateUsage=false, reportUnknownArgumentType=false, reportUnknownVariableType=false, reportArgumentType=false, reportIndexIssue=false, reportAttributeAccessIssue=false, reportUnknownMemberType=false, reportOptionalMemberAccess=false, reportCallIssue=false, reportMissingParameterType=false, reportUnknownParameterType=false

import copy
import datetime
import io
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from typing import Any, Dict, List, cast
from unittest.mock import MagicMock, patch
from argparse import Namespace

from test_support import pin_tempdir

pin_tempdir()

from .__main__ import get_parser
from .command_handlers import RemixHandler, CommandContext, _submit_jobs
from .filename_manager import FilenameManager
from .model_format import ModelFormatConfig
from .prompt_comment import strip_comments
from .prompt_locator import get_workflow_node_text
from .remix import RemixRequest, from_args
from .workflow_prompt_pair import WorkflowPromptPair

# 注入的时间依赖：日期模板求值必须是确定值，测试才能断言稳定的 filename_prefix
_NOW = datetime.datetime(2026, 6, 1, 12, 0, 0)


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
        # 基线在预处理完成之后、编辑之前取：测试里预处理只涉及日期求值，需先执行
        baseline_prompt=copy.deepcopy(prompt),
        now=_NOW,
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


def _date_workflow_and_prompt(
    seed_strategy: str = "fixed",
) -> tuple[Dict[str, Any], Dict[str, Any]]:
    """带日期文件名模板与种子节点的最小工作流，用于验证预处理阶段的日期求值。"""
    workflow: Dict[str, Any] = {
        "nodes": [
            {"id": "1", "type": "CLIPTextEncode", "widgets_values": ["masterpiece,"]},
            {
                "id": "4",
                "type": "KSampler",
                "widgets_values": [12345, seed_strategy, 20, 8.0],
            },
            {
                "id": "9",
                "type": "SaveImage",
                "widgets_values": ["C/D/%date:yyyyMMdd_hhmmss%"],
            },
        ]
    }
    prompt: Dict[str, Any] = {
        "1": {"class_type": "CLIPTextEncode", "inputs": {"text": "masterpiece,"}},
        "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        "9": {
            "class_type": "SaveImage",
            # 预置一个过期快照：预处理阶段应把日期模板重新求值为 _NOW 对应的时间
            "inputs": {"filename_prefix": "C/D/20260601_000000"},
        },
    }
    return workflow, prompt


def _refresh_filenames(prompt: Dict[str, Any], workflow: Dict[str, Any]) -> None:
    """模拟 __main__ 的预处理阶段：按统一时间把日期模板求值进 prompt。

    必须在 `_context` 之前调用，基线才会是预处理完成后的快照（与生产一致）。
    """
    pair = WorkflowPromptPair(workflow, prompt)
    FilenameManager(
        pair, pair.date_filename_nodes, pair.title_to_node, _NOW
    ).update_output_filenames()


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


class TestRemixHandlerAddAfterRemovedLine(unittest.TestCase):
    """无显式目标时，`--add` 默认插到本次唯一被移除的那一行之后（原地替换）。"""

    def _run(self, ctx: CommandContext) -> List[tuple[Dict[str, Any], Dict[str, Any]]]:
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        return captured

    def _sent_texts(self, ctx: CommandContext) -> tuple[str, Any]:
        sent_prompt, sent_workflow = self._run(ctx)[0]
        wf_text = get_workflow_node_text(sent_workflow, "1")
        assert wf_text is not None
        return wf_text, sent_prompt["1"]["inputs"]["text"]

    def _assert_tracks_in_sync(self, wf_text: str, pr_text: Any) -> None:
        """两条轨道同步：prompt 是剔除注释与 region 标记后的工作流文本（忽略空行布局）。"""
        self.assertEqual(
            [line for line in strip_comments(wf_text).splitlines() if line.strip()],
            [line for line in str(pr_text).splitlines() if line.strip()],
        )

    def test_add_lands_after_removed_line_outside_regions(self) -> None:
        """region 外的旧词被注释掉后，新标签紧跟该注释行，positive 区域内容不变。"""
        text = (
            "// #region positive\n"
            "masterpiece,\n"
            "// #endregion positive\n"
            "old tag,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt, workflow, _request(["--remove", "old tag", "--add", "new tag"])
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\n// #endregion positive\n"
            "// old tag,\nnew tag,\n",
        )
        # 双轨道同步：prompt 轨道是剔除注释与 region 标记后的工作流文本
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_add_lands_inside_custom_region_after_removed_line(self) -> None:
        """自定义 region 内的旧词被注释掉后，新标签插在该 region 内而不是落到 positive。"""
        text = (
            "masterpiece,\n"
            "// #region custom\n"
            "old tag,\n"
            "// #endregion custom\n"
            "best quality,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt, workflow, _request(["--remove", "old tag", "--add", "new tag"])
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "masterpiece,\n// #region custom\n// old tag,\nnew tag,\n"
            "// #endregion custom\nbest quality,\n",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_hard_remove_leaves_added_line_in_place(self) -> None:
        """`--hard` 删除该行时，新标签占据原行位置，其余行序不变。"""
        text = (
            "masterpiece,\n"
            "// #region custom\n"
            "old tag,\n"
            "// #endregion custom\n"
            "best quality,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "old tag", "--add", "new tag", "--hard"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "masterpiece,\n// #region custom\nnew tag,\n"
            "// #endregion custom\nbest quality,\n",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_multiple_adds_inserted_in_order_after_anchor(self) -> None:
        """一次多个 --add 按给出顺序依次插在同一锚点之后。"""
        workflow, prompt = _workflow_and_prompt(
            "masterpiece,\nold tag,\nbest quality,\n"
        )
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "old tag", "--add", "first", "--add", "second"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "masterpiece,\n// old tag,\nfirst,\nsecond,\nbest quality,\n",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_multiple_matches_fall_back_to_default_region(self) -> None:
        """命中多行时没有唯一锚点，回落默认 positive 区域。"""
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n"
                        "// #endregion positive\nold tag,\n"
                    ],
                },
                {"id": "2", "type": "CLIPTextEncode", "widgets_values": ["old tag,\n"]},
                {
                    "id": "4",
                    "type": "KSampler",
                    "widgets_values": [12345, "fixed", 20, 8.0],
                },
            ]
        }
        prompt: Dict[str, Any] = {
            "1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,\nold tag,\n"},
            },
            "2": {"class_type": "CLIPTextEncode", "inputs": {"text": "old tag,\n"}},
            "4": {"class_type": "KSampler", "inputs": {"seed": 12345, "cfg": 8.0}},
        }
        ctx = _context(
            prompt, workflow, _request(["--remove", "old tag", "--add", "new tag"])
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\nnew tag,\n// #endregion positive\n"
            "// old tag,\n",
        )
        self._assert_tracks_in_sync(wf_text, pr_text)

    def test_multiple_adds_stay_inside_anchor_region_when_other_region_is_rebuilt(
        self,
    ) -> None:
        """插入之间若有其它 region 被重建（空行折叠），后续标签仍留在锚点 region 内。"""
        text = (
            "// #region a\n"
            "new tag,\n"
            "// #endregion a\n"
            "\n"
            "\n"
            "// #region b\n"
            "anchor line,\n"
            "// #endregion b"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(
                ["--remove", "anchor line", "--add", "new tag", "--add", "second"]
            ),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region a\n// new tag,\n// #endregion a\n\n"
            "// #region b\n// anchor line,\nnew tag,\nsecond,\n// #endregion b",
        )
        # 第一条标签已存在于 region a，清理后两条轨道都只保留 region b 内的一份
        self.assertEqual(str(pr_text).count("new tag"), 1)
        self.assertEqual(str(pr_text).count("second"), 1)
        self.assertNotIn("anchor line", str(pr_text))
        self._assert_tracks_in_sync(wf_text, pr_text)

    def test_add_stays_at_anchor_when_weight_edit_appends_line_above_it(self) -> None:
        """移除后调权向上追加一行，新标签仍紧跟被注释的锚点行。"""
        text = (
            "// #region positive\n"
            "masterpiece,\n"
            "// #endregion positive\n"
            "old tag,\n"
            "tail,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(
                [
                    "--remove",
                    "old tag",
                    "--add",
                    "new tag",
                    "--add",
                    "missing_tag",
                    "1.5",
                ]
            ),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\n(missing_tag:1.5),\n"
            "// #endregion positive\n// old tag,\nnew tag,\ntail,\n",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_add_stays_at_anchor_when_weight_edit_collapses_lines_above_it(
        self,
    ) -> None:
        """移除后调权折叠上方空行（行数 -1），新标签仍紧跟被注释的锚点行。"""
        text = (
            "// #region positive\n"
            "masterpiece,\n"
            "\n"
            "\n"
            "// #endregion positive\n"
            "old tag,\n"
            "tail,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(
                [
                    "--remove",
                    "old tag",
                    "--add",
                    "new tag",
                    "--add",
                    "missing_tag",
                    "1.5",
                ]
            ),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\n(missing_tag:1.5),\n"
            "// #endregion positive\n// old tag,\nnew tag,\ntail,\n",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_hard_add_ignores_same_content_line_in_other_region(self) -> None:
        """hard 移除后依附行在另一 region 有同名行时，仍插在锚点所在 region 内。"""
        text = (
            "// #region positive\n"
            "masterpiece,\n"
            "// #endregion positive\n"
            "// #region negative\n"
            "target,\n"
            "masterpiece,\n"
            "// #endregion negative"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(
                [
                    "--remove",
                    "target",
                    "--hard",
                    "--add",
                    "new tag",
                    "--add",
                    "a1",
                    "1.5",
                    "--add",
                    "a2",
                    "1.5",
                    "--add",
                    "a3",
                    "1.5",
                ]
            ),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\n(a1:1.5),\n(a2:1.5),\n(a3:1.5),\n"
            "// #endregion positive\n// #region negative\nnew tag,\nmasterpiece,\n"
            "// #endregion negative",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_hard_add_ignores_blank_anchor_outside_its_region(self) -> None:
        """hard 移除后依附行为空行时，不能匹配到其它 region 的空行。"""
        text = (
            "// #region positive\n"
            "masterpiece,\n"
            "// #endregion positive\n"
            "\n"
            "// #region negative\n"
            "target,\n"
            "\n"
            "// #endregion negative"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(
                [
                    "--remove",
                    "target",
                    "--hard",
                    "--add",
                    "new tag",
                    "--add",
                    "a1",
                    "1.5",
                    "--add",
                    "a2",
                    "1.5",
                    "--add",
                    "a3",
                    "1.5",
                ]
            ),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\n(a1:1.5),\n(a2:1.5),\n(a3:1.5),\n"
            "// #endregion positive\n\n// #region negative\nnew tag,\n\n"
            "// #endregion negative",
        )
        self.assertEqual(pr_text, strip_comments(wf_text))

    def test_not_found_remove_falls_back_to_default_region(self) -> None:
        """未命中任何行时没有锚点，回落默认 positive 区域。"""
        text = "// #region positive\nmasterpiece,\n// #endregion positive\n"
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "missing tag", "--add", "new tag"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\nnew tag,\n// #endregion positive",
        )
        self._assert_tracks_in_sync(wf_text, pr_text)

    def test_raw_remove_falls_back_to_default_region(self) -> None:
        """`--raw` 精确子串移除不产生锚点，新增仍落到默认 positive 区域。"""
        text = "// #region positive\nmasterpiece,\n// #endregion positive\nold tag,\n"
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "old tag", "--raw", "--add", "new tag"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "// #region positive\nmasterpiece,\nnew tag,\n// #endregion positive\n,\n",
        )
        self._assert_tracks_in_sync(wf_text, pr_text)

    def test_explicit_region_overrides_anchor(self) -> None:
        """显式 --region 始终优先于锚点。"""
        text = (
            "masterpiece,\n"
            "// #region custom\n"
            "best quality,\n"
            "// #endregion custom\n"
            "old tag,\n"
        )
        workflow, prompt = _workflow_and_prompt(text)
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "old tag", "--add", "new tag", "--region", "custom"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(
            wf_text,
            "masterpiece,\n// #region custom\nbest quality,\nnew tag,\n"
            "// #endregion custom\n// old tag,\n",
        )
        self._assert_tracks_in_sync(wf_text, pr_text)

    def test_explicit_node_overrides_anchor(self) -> None:
        """显式 --node 始终优先于锚点：仍追加到目标节点末尾。"""
        workflow, prompt = _workflow_and_prompt("old tag,\nmasterpiece,\n")
        ctx = _context(
            prompt,
            workflow,
            _request(["--remove", "old tag", "--add", "new tag", "--node", "1"]),
        )
        wf_text, pr_text = self._sent_texts(ctx)
        self.assertEqual(wf_text, "// old tag,\nmasterpiece,\nnew tag,")
        self._assert_tracks_in_sync(wf_text, pr_text)


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

    def test_no_edit_flags_submits_when_seed_changes(self) -> None:
        """无编辑旗标但种子策略随机化：种子更新使提交内容与基线不同，照常提交。"""
        workflow, prompt = _workflow_and_prompt(
            "masterpiece,", seed_strategy="randomize"
        )
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


class TestRemixSkipWithoutChange(unittest.TestCase):
    """空转保护：即将提交的 prompt 与基线完全一致时跳过入列，`--no-skip` 例外。"""

    def test_no_edit_with_fixed_seed_is_skipped(self) -> None:
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request([]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)
        self.assertEqual(ctx.skip_reason, "重混未产生任何改动")

    def test_queue_with_fixed_seed_is_skipped(self) -> None:
        """queue 与无编辑项的 remix 共用同一处理器，判定一致。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, RemixRequest())
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)

    def test_no_skip_flag_submits_anyway(self) -> None:
        """`--no-skip` 语义不变：比较结果相等也照常提交。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--no-skip"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_called_once()
        self.assertFalse(ctx.skipped)

    def test_branch_only_with_fixed_seed_is_skipped(self) -> None:
        """`--branch` 不算编辑项：prompt 未变化时仍受空转保护约束。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--branch", "b1"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)
        self.assertEqual(ctx.skip_reason, "重混未产生任何改动")

    def test_branch_only_with_no_skip_submits(self) -> None:
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--branch", "b1", "--no-skip"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_called_once()
        self.assertFalse(ctx.skipped)

    def test_edit_writing_current_value_is_skipped(self) -> None:
        """--cfg 写回与现值相同的值不构成有意义的改动，不再因编辑动作本身而提交。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--cfg", "8"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)

    def test_effective_edit_submits(self) -> None:
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, _request(["--cfg", "6"]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        self.assertEqual(len(captured), 1)
        self.assertEqual(captured[0][0]["4"]["inputs"]["cfg"], 6.0)
        self.assertFalse(ctx.skipped)

    def test_workflow_only_change_is_skipped(self) -> None:
        """只比较 prompt：workflow 侧变化（如禁用节点的计数器自增）不构成有意义的改动。"""
        workflow, prompt = _workflow_and_prompt("masterpiece,")
        ctx = _context(prompt, workflow, RemixRequest())
        changed_workflow = copy.deepcopy(workflow)
        changed_workflow["nodes"].append(
            {"id": "99", "type": "Note", "widgets_values": [1]}
        )
        pair = WorkflowPromptPair(changed_workflow, copy.deepcopy(prompt))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            submitted = _submit_jobs(ctx, pair, None)
        self.assertFalse(submitted)
        submit.assert_not_called()

    def test_date_template_with_fixed_seed_is_skipped(self) -> None:
        """日期模板在预处理阶段求值一次并进入基线，无编辑项 + fixed 仍能正确跳过。"""
        workflow, prompt = _date_workflow_and_prompt()
        _refresh_filenames(prompt, workflow)
        ctx = _context(prompt, workflow, _request([]))
        with patch("comfyui.command_handlers._submit_fn") as submit:
            RemixHandler().run(ctx)
        submit.assert_not_called()
        self.assertTrue(ctx.skipped)

    def test_date_template_jobs_share_filename_prefix(self) -> None:
        """日期求值只发生在预处理阶段：同一张图的多次 job 得到相同的 filename_prefix。"""
        workflow, prompt = _date_workflow_and_prompt(seed_strategy="randomize")
        _refresh_filenames(prompt, workflow)
        ctx = _context(prompt, workflow, _request([]), jobs=2)
        with patch("comfyui.command_handlers._submit_fn") as submit:
            captured = _submissions(submit)
            RemixHandler().run(ctx)
        self.assertEqual(len(captured), 2)
        self.assertEqual(
            [p["9"]["inputs"]["filename_prefix"] for p, _ in captured],
            ["C/D/20260601_120000", "C/D/20260601_120000"],
        )


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
        args = Namespace(model="someModel.safetensors", format="disabled")
        stdout = io.StringIO()
        with tempfile.TemporaryDirectory() as tmp_dir, patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": tmp_dir}
        ):
            from .command_handlers import handle_set_model_format_cmd

            with redirect_stdout(stdout):
                handle_set_model_format_cmd(args)

            reloaded = ModelFormatConfig.load()
            self.assertEqual(reloaded.models.get("someModel.safetensors"), "disabled")
        # 命令的用户可见反馈：报告写入的模型与保存位置
        self.assertIn("someModel.safetensors", stdout.getvalue())

    def test_set_model_format_requires_both_arguments(self) -> None:
        """缺少 model 或 format 参数时快速失败。"""
        from .command_handlers import handle_set_model_format_cmd

        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="", format="anima"))
        with self.assertRaises(ValueError):
            handle_set_model_format_cmd(Namespace(model="m.safetensors", format=""))
