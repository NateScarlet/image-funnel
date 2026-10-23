import logging
import unittest
import os
import json
import argparse
import shutil
import tempfile
import sqlite3
from contextlib import contextmanager
from typing import Dict, Any, Generator, Optional, List, Set, Tuple, cast
from unittest.mock import ANY, patch, MagicMock

from PIL import Image
from PIL.PngImagePlugin import PngInfo

from test_support import pin_tempdir

pin_tempdir()

logging.disable(logging.CRITICAL)
# _load_workflow_data 是已见提示词收集的唯一入口，测试直接覆盖该内部接缝
# pyright: reportPrivateUsage=false
from .autocomplete import (
    AutocompleteContext,
    WorkflowPromptProvider,
    DanbooruProvider,
    NodeProvider,
    LoraProvider,
    RegionProvider,
    RegionOptionProvider,
    ModelFormatProvider,
    AutocompleteRequest,
    AutocompleteServices,
    autocomplete,
    build_providers,
    build_request_from_params,
    classify_action,
    collect_options,
    quote_if_needed,
    NullSuggestionTextFormatter,
    PromptTarget,
    _load_workflow_data,
    _parse_args_for_autocomplete,
)
from .__main__ import get_parser
from .cancellable_http import HttpCanceledError, HttpError
from .cancellation import CancelToken
from .danbooru import DanbooruTag, SemanticLayerUnavailable
from .danbooru_embedding import COMPILED_EMBEDDINGS_FILENAME
from .model_format import (
    MissingDataDirError,
    ModelFormatConfig,
    NodeTextFormatter,
    format_workflow_prompt_pair,
)
from .prompt_fragment import PromptFragment
from .workflow_prompt_pair import WorkflowPromptPair


@contextmanager
def _data_dir_env() -> Generator[str, None, None]:
    """把数据目录临时指向空目录（新增标签补全按需加载模型格式配置）。

    同时清空 DANBOORU_DATA_DIR：外部 shell 若指向真实编译产物，本地链路会
    绕过这里的空目录直接加载数据集，使断言依赖本机数据。
    """
    with tempfile.TemporaryDirectory() as tmp_dir:
        with patch.dict(
            os.environ,
            {"IMAGE_FUNNEL_DATA_DIR": tmp_dir, "DANBOORU_DATA_DIR": ""},
        ):
            yield tmp_dir


def _request(
    cwords: Optional[List[str]] = None, target_command: str = "remix"
) -> AutocompleteRequest:
    """构造一个补全请求上下文，默认落在「新增标签」这一编辑动作上。"""
    return AutocompleteRequest(
        target_command=target_command,
        query="",
        prev_word="",
        cwords=cwords if cwords is not None else ["/remix", "--add"],
        image_paths=[],
        root_dir="root",
        directory_rel_path="dir",
        cancel_token=CancelToken(),
    )


def _make_context_with(**kwargs: Any) -> AutocompleteContext:
    """按当前已输入的词推导编辑动作来构造补全上下文（与生产路径同一判定）。"""
    cwords = cast(List[str], kwargs.get("cwords") or [])
    parser = get_parser()
    defaults: Dict[str, Any] = dict(
        target_command="remix",
        query="",
        prev_word="",
        cwords=cwords,
        image_paths=[],
        parsed_args=None,
        seen_prompts={},
        workflow=None,
        prompt_meta=None,
        parser=parser,
        # 用例默认令牌：仅满足调用期签名；断言令牌透传的用例显式传入自己的令牌
        cancel_token=CancelToken(),
        action=classify_action(
            [w for w in cwords if not (w.startswith("<") and w.endswith(">"))],
            collect_options(parser, cwords)[0],
        ),
    )
    defaults.update(kwargs)
    return AutocompleteContext(**defaults)


class TestComfyUIAutocomplete(unittest.TestCase):

    def _make_context(self, **kwargs: Any) -> AutocompleteContext:
        """Helper to build AutocompleteContext with sane defaults."""
        return _make_context_with(**kwargs)

    def _make_parsed_args(self, **kwargs: Any) -> argparse.Namespace:
        """Helper to build a Namespace simulating parsed CLI args."""
        base = dict(
            command="remove",
            neg=False,
            all=False,
            region=None,
            node=None,
            raw=False,
            hard=False,
            no_skip=False,
        )
        base.update(kwargs)
        return argparse.Namespace(**base)

    def _make_history(
        self,
        added: Optional[Set[str]] = None,
        added_times: Optional[Dict[str, str]] = None,
        all_added: Optional[List[Tuple[str, str]]] = None,
    ) -> MagicMock:
        """构造注入 DanbooruProvider 的历史依赖 mock。"""
        history = MagicMock()
        history.get_added_prompts.return_value = set(added or [])
        history.get_added_prompt_times.return_value = dict(added_times or {})
        history.get_all_added_prompts.return_value = list(all_added or [])
        return history

    # ---- WorkflowPromptProvider tests ----

    def test_quote_if_needed_escapes_for_split_args(self):
        r"""quote_if_needed 的转义约定必须与 Go 端 splitArgs 闭环。

        splitArgs 双引号内还原 \\ 与 \"，因此这里必须把原始值中的
        \ 加倍为 \\、" 转义为 \"，再用双引号包裹；否则指令执行时
        参数会保留双反斜杠，导致 /remove 等匹配失败。
        """
        # 含空格的 ComfyUI 转义括号提示词
        raw = r"elysia \(herrscher of human: ego\) \(honkai impact\)"
        quoted = quote_if_needed(raw)
        self.assertEqual(
            quoted,
            r'"elysia \\(herrscher of human: ego\\) \\(honkai impact\\)"',
        )
        # 反向：splitArgs 还原后应等于原始值
        # （等价于 Go 端 TestSplitArgs_AutocompleteRoundTrip 的对端断言）
        unescaped = quoted[1:-1].replace(r"\\", "\\").replace(r"\"", '"')
        self.assertEqual(unescaped, raw)

        # 含双引号的值
        self.assertEqual(quote_if_needed('say "hi"'), r'"say \"hi\""')
        # 无空格无需包裹
        self.assertEqual(quote_if_needed("masterpiece"), "masterpiece")

    def test_autocomplete_remove_prompt_normal(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            cwords=["/remix", "--remove"],
            parsed_args=self._make_parsed_args(),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 2)
        self.assertIn('"1girl, masterpiece, score_7, cute"', texts)
        self.assertIn('"another prompt line"', texts)
        self.assertEqual(suggestions[0].type, "prompt")
        self.assertEqual(suggestions[0].style, "")
        self.assertTrue("positive" in suggestions[0].description)

    def test_autocomplete_remove_prompt_by_region(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            query="cute",
            cwords=["/remix", "--remove", "--region", "positive"],
            parsed_args=self._make_parsed_args(region=["positive"]),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(texts[0], '"1girl, masterpiece, score_7, cute"')

    def test_autocomplete_remove_prompt_skip_on_option(self):
        context = self._make_context(
            target_command="remix",
            prev_word="--region",
            cwords=["/remix", "--remove", "--region"],
            parsed_args=self._make_parsed_args(),
        )
        provider = WorkflowPromptProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_remove_prompt_skip_on_dash_query(self):
        context = self._make_context(
            target_command="remix",
            query="--r",
            prev_word="",
            cwords=["/remix", "--remove"],
            parsed_args=self._make_parsed_args(),
        )
        provider = WorkflowPromptProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_remove_prompt_by_custom_command(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            cwords=["/delete_prompt", "--remove"],
            parsed_args=self._make_parsed_args(),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 2)
        self.assertIn('"1girl, masterpiece, score_7, cute"', texts)

    def test_autocomplete_remove_prompt_with_double_dash(self):
        seen_prompts = {
            "-a_cute_girl": "区域: positive",
            "masterpiece": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            query="-a",
            prev_word="--",
            cwords=["/remix", "--remove", "--"],
            parsed_args=self._make_parsed_args(),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(texts[0], "-a_cute_girl")

    def test_autocomplete_adjust_lora_no_prompt_or_danbooru(self):
        context = self._make_context(
            target_command="remix",
            prev_word="my_lora",
            cwords=["/remix", "--lora", "my_lora"],
            parsed_args=argparse.Namespace(command="remix"),
        )
        provider = LoraProvider()
        self.assertFalse(provider.can_provide(context))
        provider2 = WorkflowPromptProvider()
        self.assertFalse(provider2.can_provide(context))
        provider3 = DanbooruProvider(
            MagicMock(), self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertFalse(provider3.can_provide(context))

    def test_add_never_suggests_existing_prompts(self):
        """`--add` 后面填的始终是标签：末尾 token 是不是权重表达式只在提交时判定，
        因此补全不该在第二个值位置切换成「工作流中已有提示词」——那里填的是数字或
        表达式，给出提示词名反而会让人误选成一个新标签。"""
        for cwords in (
            ["/remix", "--add", "1girl, masterpiece, score_7, cute"],
            ["/remix", "--add", '"another prompt line"', "0.8"],
        ):
            with self.subTest(cwords=cwords):
                context = self._make_context(
                    target_command="remix",
                    cwords=cwords,
                    parsed_args=self._make_parsed_args(command="remix"),
                    seen_prompts={
                        "1girl, masterpiece, score_7, cute": "区域: positive"
                    },
                )
                self.assertFalse(WorkflowPromptProvider().can_provide(context))

    def test_add_keeps_tag_completion_after_one_tag(self):
        """已经给了一个标签后仍应继续补标签，而不是切走。"""
        mock_provider = MagicMock()
        mock_provider.related.return_value = []
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="masterpiece,",
            cwords=["/remix", "--add", "masterpiece,"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))
        list(provider.provide(context))
        mock_provider.related.assert_called_once_with(
            ["masterpiece"],
            target_categories=["General", "Artist", "Meta"],
            cancel_token=context.cancel_token,
        )

    def test_remove_prompt_suggestions_work_before_its_value_is_typed(self):
        """`/remix --remove bad` 时旗标后面还没有值，argparse 会校验失败；
        补上占位值重试后仍要能给出工作流中已有提示词的建议。"""
        seen_prompts = {"bad hands": "节点: 2"}
        parsed = _parse_args_for_autocomplete(
            get_parser(), "remix", ["/remix", "--remove"]
        )
        context = self._make_context(
            target_command="remix",
            query="bad",
            prev_word="--remove",
            cwords=["/remix", "--remove"],
            parsed_args=parsed,
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertIsNotNone(parsed)
        self.assertTrue(provider.can_provide(context))
        self.assertEqual([s.text for s in provider.provide(context)], ['"bad hands"'])

    def test_autocomplete_remove_prompt_with_double_dash_and_option(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            query="a",
            prev_word="--region",
            cwords=["/remix", "--remove", "--", "--region"],
            parsed_args=self._make_parsed_args(),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 2)
        self.assertIn('"1girl, masterpiece, score_7, cute"', texts)

    # ---- DanbooruProvider tests ----

    def test_add_flag_value_is_still_completed(self) -> None:
        """`/remix --add blue` 的前一个词就是取值型旗标 `--add`，
        但它的值正是要补全的标签，不能被当成「正在输入选项参数」而全部拒绝。"""
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("blue_eyes", "蓝眼", "Blue eyes.", "General"),
        ]
        context = self._make_context(
            target_command="remix",
            query="blue",
            prev_word="--add",
            cwords=["/remix", "--add"],
            parsed_args=_parse_args_for_autocomplete(
                get_parser(), "remix", ["/remix", "--add"]
            ),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        mock_provider.search.assert_called_once_with(
            "blue", cancel_token=context.cancel_token
        )
        self.assertEqual([s.text for s in suggestions], ["blue_eyes"])

    def test_add_flag_value_completed_on_empty_query(self) -> None:
        """刚打完 `--add ` 还没输入时同样应进入新增标签的补全。"""
        mock_provider = MagicMock()
        mock_provider.related.return_value = []
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="--add",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

    def test_autocomplete_add_prompt_danbooru(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
            DanbooruTag(
                "dunyarzad_(genshin_impact)", "迪娜泽黛", "A noble girl.", "General"
            ),
        ]

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.search.assert_called_once_with(
            "girl", cancel_token=context.cancel_token
        )

        self.assertEqual(len(suggestions), 2)
        self.assertEqual(suggestions[0].text, "1girl")
        self.assertEqual(suggestions[0].displayText, "1girl (女孩)")
        self.assertEqual(suggestions[0].description, "A female character.")
        self.assertEqual(suggestions[0].type, "danbooru")

        self.assertEqual(suggestions[1].text, r"dunyarzad_\(genshin_impact\)")
        self.assertEqual(
            suggestions[1].displayText, "dunyarzad_(genshin_impact) (迪娜泽黛)"
        )
        self.assertEqual(suggestions[1].description, "A noble girl.")

        # 选项不满足的情况
        context2 = self._make_context(
            target_command="remix",
            query="pos",
            prev_word="--region",
            cwords=["/remix", "--add", "--region"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
        )
        self.assertFalse(provider.can_provide(context2))

    def test_autocomplete_add_prompt_danbooru_related(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("sailor_collar", "水手领", "A collar style.", "General"),
            DanbooruTag("skirt", "裙子", "A bottom wear.", "General"),
        ]

        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="1girl,",
            cwords=["/remix", "--add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.related.assert_called_once_with(
            ["masterpiece", "1girl"],
            target_categories=["General", "Artist", "Meta"],
            cancel_token=context.cancel_token,
        )

        self.assertEqual(len(suggestions), 2)
        self.assertEqual(suggestions[0].text, "sailor_collar")
        self.assertEqual(suggestions[1].text, "skirt")

    def test_autocomplete_add_prompt_danbooru_related_fallback(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("solo", "单人", "Solo.", "General"),
        ]

        seen_prompts = {"masterpiece, 1girl": "区域: positive"}
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.related.assert_called_once_with(
            ["1girl", "masterpiece"],
            target_categories=["General", "Artist", "Meta"],
            cancel_token=context.cancel_token,
        )
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "solo")

    def test_autocomplete_danbooru_style_muted_from_history(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
            DanbooruTag("solo", "单人", "Solo image.", "General"),
        ]
        mock_provider.related.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
            DanbooruTag("solo", "单人", "Solo image.", "General"),
        ]
        history = self._make_history(added={"1girl"})
        provider = DanbooruProvider(
            mock_provider, history, NullSuggestionTextFormatter()
        )

        # 场景 1：有 query，执行前缀语义搜索
        context1 = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        suggestions1 = list(provider.provide(context1))

        self.assertEqual(len(suggestions1), 2)
        self.assertEqual(suggestions1[0].text, "1girl")
        self.assertEqual(suggestions1[0].style, "muted")
        self.assertEqual(suggestions1[1].text, "solo")
        self.assertEqual(suggestions1[1].style, "")

        # 场景 2：空 query，执行关联联想
        context2 = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={"portrait": "区域: positive"},
            workflow={"nodes": []},
            prompt_meta={},
        )
        suggestions2 = list(provider.provide(context2))

        self.assertEqual(len(suggestions2), 2)
        self.assertEqual(suggestions2[0].text, "1girl")
        self.assertEqual(suggestions2[0].style, "muted")
        self.assertEqual(suggestions2[1].text, "solo")
        self.assertEqual(suggestions2[1].style, "")

    def test_autocomplete_danbooru_history_fallback(self) -> None:
        """空 query 且无关联标签时，应回退到历史添加的标签"""
        # 模拟历史数据
        mock_history = [
            ("1girl", "2026-08-07T10:00:00"),
            ("solo", "2026-08-06T10:00:00"),
        ]
        provider = DanbooruProvider(
            MagicMock(),
            self._make_history(all_added=mock_history),
            NullSuggestionTextFormatter(),
        )

        # 空 query，无 cwords，无 workflow
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
        )
        suggestions = list(provider.provide(context))

        # 应返回所有历史标签，按时间倒序
        self.assertEqual(len(suggestions), 2)
        self.assertEqual(suggestions[0].text, "1girl")
        self.assertEqual(suggestions[0].style, "muted")
        self.assertIn("历史添加", suggestions[0].description)
        self.assertEqual(suggestions[1].text, "solo")
        self.assertEqual(suggestions[1].style, "muted")

    def test_autocomplete_danbooru_history_fallback_filter_seen(self) -> None:
        """历史回退时，应过滤掉当前提示词中已存在的标签"""
        mock_history = [
            ("1girl", "2026-08-07T10:00:00"),
            ("solo", "2026-08-06T10:00:00"),
        ]
        provider = DanbooruProvider(
            MagicMock(),
            self._make_history(all_added=mock_history),
            NullSuggestionTextFormatter(),
        )

        # seen_prompts 中包含 "1girl"
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={"1girl": "区域: positive"},
            workflow=None,
            prompt_meta=None,
        )
        suggestions = list(provider.provide(context))

        # 应只返回 "solo"（"1girl" 已在提示词中）
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "solo")

    def test_autocomplete_danbooru_history_empty(self) -> None:
        """历史为空时，历史回退不产生建议"""
        provider = DanbooruProvider(
            MagicMock(), self._make_history(), NullSuggestionTextFormatter()
        )
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
        )
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 0)

    def test_autocomplete_danbooru_history_supplement_related(self) -> None:
        """有关联标签时，历史标签应作为补充追加"""
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
            DanbooruTag("solo", "单人", "Solo image.", "General"),
        ]

        mock_history = [
            ("1girl", "2026-08-07T10:00:00"),
            ("solo", "2026-08-06T10:00:00"),
            ("masterpiece", "2026-08-05T10:00:00"),
        ]
        provider = DanbooruProvider(
            mock_provider,
            self._make_history(all_added=mock_history),
            NullSuggestionTextFormatter(),
        )

        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={"portrait": "区域: positive"},
            workflow={"nodes": []},
            prompt_meta={},
        )
        suggestions = list(provider.provide(context))

        # 应包含关联联想结果（2个）
        # 加上历史补充中不重复的（只有 masterpiece 不重复）
        self.assertEqual(len(suggestions), 3)
        # 关联联想在前
        self.assertEqual(suggestions[0].text, "1girl")
        self.assertEqual(suggestions[1].text, "solo")
        # 历史补充在后，且不应重复关联联想已有的标签
        self.assertEqual(suggestions[2].text, "masterpiece")
        self.assertEqual(suggestions[2].style, "muted")

    def test_autocomplete_add_prompt_with_neg_flag(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
        ]

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="--neg",
            cwords=["/remix", "--add", "--neg"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")

    def test_autocomplete_add_prompt_region_query_search(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
        ]

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="positive",
            cwords=["/remix", "--add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.search.assert_called_once_with(
            "girl", cancel_token=context.cancel_token
        )
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")

    def test_autocomplete_add_prompt_region_related(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("cute", "可爱", "Cute character.", "General"),
        ]

        seen_prompts = {"1girl, masterpiece, score_7, cute": "区域: positive"}
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="positive",
            cwords=["/remix", "--add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "cute")

    def test_autocomplete_add_prompt_region_query_search_fallback(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
        ]

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="positive",
            cwords=["/remix", "--add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")

    def test_autocomplete_add_prompt_region_related_fallback(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("cute", "可爱", "Cute character.", "General"),
        ]

        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="positive",
            cwords=["/remix", "--add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "cute")

    # ---- Danbooru 失败处理：预期错误显示 ⚠，编程错误传播 ----

    def test_autocomplete_danbooru_search_upstream_error_yields_error_item(
        self,
    ) -> None:
        """上游搜索网络失败（HttpError）以 ⚠ 建议项呈现给用户。"""
        mock_provider = MagicMock()
        mock_provider.search.side_effect = HttpError("timeout")

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].type, "error")
        self.assertIn("搜索失败", suggestions[0].displayText)
        self.assertEqual(suggestions[0].text, "")

    def test_autocomplete_danbooru_search_programming_error_propagates(
        self,
    ) -> None:
        """上游搜索的编程错误（非 HttpError）不再被吞掉：直接抛出。"""
        mock_provider = MagicMock()
        mock_provider.search.side_effect = KeyError("tag")

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        with self.assertRaises(KeyError):
            list(provider.provide(context))

    def test_autocomplete_danbooru_search_cancel_propagates(self) -> None:
        """取消掐断在途请求（HttpCanceledError）不是上游失败：直接传播，不渲染 ⚠ 项。"""
        mock_provider = MagicMock()
        mock_provider.search.side_effect = HttpCanceledError(
            "请求已取消，在途请求已被掐断"
        )

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        with self.assertRaises(HttpCanceledError):
            list(provider.provide(context))

    def test_autocomplete_danbooru_semantic_unavailable_yields_error_item(
        self,
    ) -> None:
        """语义层降级提示项渲染为 ⚠ 错误建议项，字面标签建议仍照常输出。"""
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A girl.", "General"),
            SemanticLayerUnavailable(reason="嵌入接口不可用: 超时"),
        ]

        context = self._make_context(
            target_command="remix",
            query="girl",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        suggestions = list(provider.provide(context))

        self.assertEqual([s.text for s in suggestions], ["1girl", ""])
        self.assertEqual(suggestions[-1].type, "error")
        self.assertIn("语义搜索不可用", suggestions[-1].displayText)
        self.assertEqual(suggestions[-1].description, "嵌入接口不可用: 超时")

    def test_autocomplete_danbooru_related_upstream_error_yields_error_item(
        self,
    ) -> None:
        """上游关联联想网络失败（HttpError）以 ⚠ 建议项呈现给用户。"""
        mock_provider = MagicMock()
        mock_provider.related.side_effect = HttpError("connection refused")

        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="1girl,",
            cwords=["/remix", "--add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].type, "error")
        self.assertIn("搜索失败", suggestions[0].displayText)

    def test_autocomplete_danbooru_related_internal_error_propagates(
        self,
    ) -> None:
        """上游关联联想的内部错误（sqlite3.Error，非 HttpError）不再被吞掉。"""
        mock_provider = MagicMock()
        mock_provider.related.side_effect = sqlite3.OperationalError(
            "database is locked"
        )

        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="1girl,",
            cwords=["/remix", "--add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = DanbooruProvider(
            mock_provider, self._make_history(), NullSuggestionTextFormatter()
        )

        with self.assertRaises(sqlite3.Error):
            list(provider.provide(context))

    def test_autocomplete_danbooru_history_error_yields_error_item(self) -> None:
        """历史读取失败（sqlite3.Error）以 ⚠ 建议项呈现给用户。"""
        history = self._make_history()
        history.get_all_added_prompts.side_effect = sqlite3.OperationalError(
            "database is locked"
        )
        provider = DanbooruProvider(MagicMock(), history, NullSuggestionTextFormatter())
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
        )

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].type, "error")
        self.assertIn("历史标签", suggestions[0].displayText)

    def test_autocomplete_danbooru_history_programming_error_propagates(
        self,
    ) -> None:
        """历史读取的编程错误（非 sqlite3.Error）不再被吞掉。"""
        history = self._make_history()
        history.get_all_added_prompts.side_effect = KeyError("boom")
        provider = DanbooruProvider(MagicMock(), history, NullSuggestionTextFormatter())
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
        )

        with self.assertRaises(KeyError):
            list(provider.provide(context))

    # ---- RegionOptionProvider tests ----

    def _get_mock_region_workflow(self) -> Dict[str, Any]:
        """含 positive/negative 两个区域标记的工作流夹具"""
        return {
            "nodes": [
                {
                    "id": "node_1",
                    "type": "CLIPTextEncode",
                    "title": "Prompt",
                    "widgets_values": [
                        "// #region positive\n1girl, masterpiece,\n// #endregion\n"
                        "// #region negative\nworst quality\n// #endregion\n"
                    ],
                }
            ]
        }

    def _get_mock_region_prompt_meta(self) -> Dict[str, Any]:
        return {"node_1": {"class_type": "CLIPTextEncode", "inputs": {"text": "1girl"}}}

    def _write_workflow_sample(self, workflow: Dict[str, Any], tmp_dir: str) -> str:
        """生成带 ComfyUI prompt/workflow 元数据的临时 PNG 样本，返回文件路径"""
        img = Image.new("RGB", (1, 1), "white")
        meta = PngInfo()
        meta.add_text(
            "prompt",
            json.dumps(
                {
                    "node_1": {
                        "class_type": "CLIPTextEncode",
                        "inputs": {"text": ""},
                    }
                }
            ),
        )
        meta.add_text("workflow", json.dumps(workflow))
        path = os.path.join(tmp_dir, "sample.png")
        img.save(path, pnginfo=meta)
        return path

    def test_seen_prompts_ignore_comments(self):
        """已见提示词只收集非注释内容：行内注释与注释行都不算已见标签"""
        workflow = self._get_mock_region_workflow()
        workflow["nodes"][0]["widgets_values"] = [
            "// #region positive\nmasterpiece // keep the style\n/* 1girl */\n"
            "// #endregion\nworst quality\n"
        ]
        tmp_dir = tempfile.mkdtemp()
        try:
            path = self._write_workflow_sample(workflow, tmp_dir)
            seen_prompts, _, _, _ = _load_workflow_data(
                [path], self._make_parsed_args(region=["positive"]), "add"
            )
        finally:
            shutil.rmtree(tmp_dir, ignore_errors=True)

        self.assertEqual(seen_prompts, {"masterpiece": "区域: positive"})

    def test_region_option_provider_suggests_all_regions_on_empty_input(self):
        """`/remix ` 空位尚未进入任何编辑动作，给出完整形式的区域选项。"""
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="/remix",
            cwords=["/remix"],
            parsed_args=self._make_parsed_args(command="remix"),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(
            [s.text for s in suggestions],
            ["--region positive", "--region negative"],
        )
        self.assertEqual([s.displayText for s in suggestions], ["positive", "negative"])
        self.assertEqual(suggestions[0].type, "region")
        self.assertEqual(suggestions[0].description, "区域: positive")

    def test_region_option_provider_skips_when_query_typed(self):
        context = self._make_context(
            target_command="remix",
            query="g",
            prev_word="/remix",
            cwords=["/remix"],
            parsed_args=self._make_parsed_args(command="remix"),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_after_add_flag(self):
        """`/remix --add ` 是标签位置：跟 `--region` 会让 argparse 拒绝整条指令。"""
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="--add",
            cwords=["/remix", "--add"],
            parsed_args=self._make_parsed_args(command="remix"),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertFalse(RegionOptionProvider().can_provide(context))

    def test_region_option_provider_skips_in_option_value_position(self):
        """取值型选项的参数位（如 `-j `）只接受值，此处不能建议 `--region`。"""
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="-j",
            cwords=["/remix", "-j"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertFalse(RegionOptionProvider().can_provide(context))

    def test_region_option_provider_skips_when_region_specified(self):
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="positive",
            cwords=["/remix", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="remix", region=["positive"]),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_when_node_specified(self):
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="5",
            cwords=["/remix", "--node", "5"],
            parsed_args=self._make_parsed_args(command="remix", node=["5"]),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_without_workflow(self):
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="/remix",
            cwords=["/remix"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_without_region_markers(self):
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="/remix",
            cwords=["/remix"],
            parsed_args=self._make_parsed_args(command="remix"),
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_suggests_regions_with_neg_flag(self):
        """--neg 只改变默认区域，不应阻止显式选择区域的建议。"""
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="--neg",
            cwords=["/remix", "--neg"],
            parsed_args=self._make_parsed_args(command="remix", neg=True),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertTrue(provider.can_provide(context))

    def test_region_option_provider_target_scan_fallback(self):
        """解析失败（parsed_args=None）时回退扫描已完成词列表中的目标选项。"""
        provider = RegionOptionProvider()

        # 无任何目标选项时正常建议区域
        context_plain = self._make_context(
            target_command="remix",
            query="",
            prev_word="/remix",
            cwords=["/remix"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertTrue(provider.can_provide(context_plain))

        # 扫描到 --region 视为已指定目标
        context_targeted = self._make_context(
            target_command="remix",
            query="",
            prev_word="positive",
            cwords=["/remix", "--region", "positive"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertFalse(provider.can_provide(context_targeted))

    def test_region_option_provider_skips_after_end_of_options(self):
        """`--` 之后写入的是位置值而非旗标，不能再建议区域选项。"""
        context = self._make_context(
            target_command="remix",
            query="",
            prev_word="--region",
            cwords=["/remix", "--", "--region"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_prefers_region_options_over_danbooru_related(self) -> None:
        """空输入且有可用区域时，区域选项独占建议，关联联想与历史回退均不执行。"""
        mock_danbooru = MagicMock()
        history = self._make_history(all_added=[("history_tag", "2026-08-07T10:00:00")])
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(mock_danbooru, history, NullSuggestionTextFormatter()),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample(
                self._get_mock_region_workflow(), tmp_dir
            )
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="/remix",
                cwords=["/remix"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=CancelToken(),
            )
            suggestions = list(autocomplete(request, services))

        self.assertEqual(
            [s.text for s in suggestions],
            ["--region positive", "--region negative"],
        )
        mock_danbooru.search.assert_not_called()
        mock_danbooru.related.assert_not_called()
        history.get_all_added_prompts.assert_not_called()

    def test_autocomplete_empty_input_without_regions_yields_nothing(self) -> None:
        """`/remix ` 空位在没有区域标记时为空：该位置不产出其他类型的建议。"""
        mock_danbooru = MagicMock()
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(
                    mock_danbooru, self._make_history(), NullSuggestionTextFormatter()
                ),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample({"nodes": []}, tmp_dir)
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="/remix",
                cwords=["/remix"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=CancelToken(),
            )
            suggestions = list(autocomplete(request, services))

        self.assertEqual(suggestions, [])
        mock_danbooru.related.assert_not_called()

    def _write_related_data_dir(self, data_dir: str) -> None:
        """写入真实数据构成的共现产物：只有多词标签，共现边也只连接多词标签。

        真实共现数据不含 ``1girl`` / ``solo`` 这类覆盖过广的标签，因此夹具里不提供
        单词标签作为种子，避免掩盖「多词标签命中不了」的问题。
        """
        from .danbooru_test import write_file_fixture

        write_file_fixture(
            data_dir,
            [
                {
                    "name": "pink_hair",
                    "cn_name": "粉发",
                    "wiki": "Pink hair.",
                    "post_count": "90",
                    "category": "0",
                    "nsfw": "0",
                },
                {
                    "name": "very_long_hair",
                    "cn_name": "超长发",
                    "wiki": "Very long hair.",
                    "post_count": "80",
                    "category": "0",
                    "nsfw": "0",
                },
                {
                    "name": "floating_hair",
                    "cn_name": "飘发",
                    "wiki": "Floating hair.",
                    "post_count": "70",
                    "category": "0",
                    "nsfw": "0",
                },
            ],
            [("pink_hair", "floating_hair", 100)],
        )

    def test_autocomplete_add_related_from_real_region_text(self) -> None:
        """真实链路：区域文本 → 本地共现 → 关联联想，且关联结果排在历史补充之前。

        夹具按真实数据构成只保留多词标签：共现索引按标签名（下划线）建表，而
        anima 提示词用空格分隔标签，命中必须依赖调用前的归一化；sdxl 提示词本来
        就是标签名形态，同一断言保证归一化对它幂等。
        """
        from .danbooru import FileDanbooruTagProvider, NullSemanticTagSearcher

        with tempfile.TemporaryDirectory() as data_dir:
            self._write_related_data_dir(data_dir)
            real_provider = FileDanbooruTagProvider(
                data_dir, False, semantic=NullSemanticTagSearcher()
            )
            services = AutocompleteServices(
                parser=get_parser(),
                providers=[
                    RegionOptionProvider(),
                    DanbooruProvider(
                        real_provider,
                        self._make_history(
                            all_added=[("history_only", "2026-08-07T10:00:00")]
                        ),
                        NullSuggestionTextFormatter(),
                    ),
                ],
            )

            for region_text in (
                "pink hair, very long hair",
                "pink_hair, very_long_hair",
            ):
                with self.subTest(region_text=region_text):
                    with tempfile.TemporaryDirectory() as tmp_dir:
                        sample_path = self._write_workflow_sample(
                            {
                                "nodes": [
                                    {
                                        "id": "node_1",
                                        "type": "CLIPTextEncode",
                                        "title": "Prompt",
                                        "widgets_values": [
                                            "// #region positive\n"
                                            f"{region_text}\n"
                                            "// #endregion"
                                        ],
                                    }
                                ]
                            },
                            tmp_dir,
                        )
                        request = AutocompleteRequest(
                            target_command="remix",
                            query="",
                            prev_word="--add",
                            cwords=["/remix", "--region", "positive", "--add"],
                            image_paths=[sample_path],
                            root_dir="mock_root",
                            directory_rel_path="mock_rel",
                            cancel_token=CancelToken(),
                        )
                        suggestions = list(autocomplete(request, services))

                    texts = [s.text for s in suggestions]
                    self.assertIn("floating_hair", texts)
                    self.assertLess(
                        texts.index("floating_hair"), texts.index("history_only")
                    )

    def test_autocomplete_add_related_normalizes_typed_uppercase_seed(self) -> None:
        """用户键入的 `--add` 值可能带大写：种子同样要归一化成小写标签名才能命中共现索引。"""
        from .danbooru import FileDanbooruTagProvider, NullSemanticTagSearcher

        with tempfile.TemporaryDirectory() as data_dir:
            self._write_related_data_dir(data_dir)
            services = AutocompleteServices(
                parser=get_parser(),
                providers=[
                    RegionOptionProvider(),
                    DanbooruProvider(
                        FileDanbooruTagProvider(
                            data_dir, False, semantic=NullSemanticTagSearcher()
                        ),
                        self._make_history(),
                        NullSuggestionTextFormatter(),
                    ),
                ],
            )
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="Pink Hair,",
                cwords=["/remix", "--add", "Pink Hair,"],
                image_paths=[],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=CancelToken(),
            )
            suggestions = list(autocomplete(request, services))

        self.assertIn("floating_hair", [s.text for s in suggestions])

    def test_autocomplete_add_position_never_suggests_region(self) -> None:
        """`/remix --add ` 是标签位置：区域选项不得抢占，关联联想照常产出。"""
        mock_danbooru = MagicMock()
        mock_danbooru.related.return_value = [
            DanbooruTag("cute", "可爱", "Cute character.", "General")
        ]
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(
                    mock_danbooru, self._make_history(), NullSuggestionTextFormatter()
                ),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample(
                self._get_mock_region_workflow(), tmp_dir
            )
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="--add",
                cwords=["/remix", "--add"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=CancelToken(),
            )
            suggestions = list(autocomplete(request, services))

        self.assertEqual([s.text for s in suggestions], ["cute"])
        mock_danbooru.related.assert_called_once()

    def test_autocomplete_modifier_value_positions_ignore_editing_action(self) -> None:
        """`--region` / `--node` 的值位置只看选项位置，不因当前编辑动作收窄。"""
        workflow = self._get_mock_region_workflow()
        prompt_meta: Dict[str, Any] = {
            "node_1": {"class_type": "CLIPTextEncode", "inputs": {"text": "1girl"}},
            "node_2": {"class_type": "KSampler", "inputs": {"cfg": 7.0}},
        }

        region_positions = [
            ["/remix", "--region"],
            ["/remix", "--remove", "foo", "--region"],
            ["/remix", "--add", "blue", "--region"],
        ]
        for cwords in region_positions:
            with self.subTest(cwords=cwords):
                context = self._make_context(
                    target_command="remix",
                    query="",
                    prev_word="--region",
                    cwords=cwords,
                    parsed_args=None,
                    workflow=workflow,
                    prompt_meta=prompt_meta,
                )
                provider = RegionProvider()
                self.assertTrue(provider.can_provide(context))
                self.assertEqual(
                    [s.text for s in provider.provide(context)],
                    ["positive", "negative"],
                )

        node_positions = [
            ["/remix", "--node"],
            ["/remix", "--cfg", "--node"],
            ["/remix", "--remove", "foo", "--node"],
        ]
        expected_nodes = [["node_1"], ["node_2"], ["node_1"]]
        for cwords, expected in zip(node_positions, expected_nodes):
            with self.subTest(cwords=cwords):
                context = self._make_context(
                    target_command="remix",
                    query="",
                    prev_word="--node",
                    cwords=cwords,
                    parsed_args=None,
                    workflow=workflow,
                    prompt_meta=prompt_meta,
                )
                provider = NodeProvider()
                self.assertTrue(provider.can_provide(context))
                self.assertEqual([s.text for s in provider.provide(context)], expected)

    def test_autocomplete_falls_back_to_danbooru_related_without_regions(self) -> None:
        """无区域标记时回落到 Danbooru 提供者（历史回退路径照常执行）。"""
        mock_danbooru = MagicMock()
        history = self._make_history(all_added=[("history_tag", "2026-08-07T10:00:00")])
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(mock_danbooru, history, NullSuggestionTextFormatter()),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample({"nodes": []}, tmp_dir)
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="",
                cwords=["/remix", "--add"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=CancelToken(),
            )
            suggestions = list(autocomplete(request, services))

        texts = [s.text for s in suggestions]
        self.assertEqual(texts, ["history_tag"])
        mock_danbooru.search.assert_not_called()
        mock_danbooru.related.assert_not_called()
        history.get_all_added_prompts.assert_called_once()

    def test_build_providers_registers_region_option_before_danbooru(self) -> None:
        """真实 provider 链中区域选项提供者必须注册在 Danbooru 提供者之前。"""
        with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
            with build_providers(
                _request(), get_parser(), "http://localhost", False
            ) as providers:
                names = [type(p).__name__ for p in providers]
        self.assertLess(
            names.index("RegionOptionProvider"), names.index("DanbooruProvider")
        )

    def test_build_providers_file_mode_when_data_dir_set(self) -> None:
        """DANBOORU_DATA_DIR 非空时启用 FileDanbooruTagProvider，且优先于在线 URL。"""
        from .danbooru import FileDanbooruTagProvider
        from .danbooru_test import write_file_fixture

        with tempfile.TemporaryDirectory() as data_dir:
            write_file_fixture(
                data_dir,
                [
                    {
                        "name": "1girl",
                        "cn_name": "女孩",
                        "wiki": "A girl.",
                        "post_count": "100",
                        "category": "0",
                        "nsfw": "0",
                    }
                ],
                [],
            )
            with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
                with build_providers(
                    _request(),
                    get_parser(),
                    "http://should-be-ignored",
                    False,
                    data_dir,
                ) as providers:
                    danbooru = next(
                        p for p in providers if type(p).__name__ == "DanbooruProvider"
                    )
                    self.assertIsInstance(
                        cast(Any, danbooru).provider, FileDanbooruTagProvider
                    )

    def test_build_providers_file_mode_missing_files_fails_fast(self) -> None:
        """数据目录缺编译产物时 build_providers 直接抛出（快速失败，不降级在线链路）。"""
        with tempfile.TemporaryDirectory() as data_dir:
            with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
                with self.assertRaises(FileNotFoundError) as ctx:
                    with build_providers(
                        _request(), get_parser(), "http://localhost", False, data_dir
                    ):
                        pass
                self.assertIn("compile_danbooru_data.py", str(ctx.exception))

    def _write_single_tag_fixture(self, data_dir: str) -> None:
        from .danbooru_test import write_file_fixture

        write_file_fixture(
            data_dir,
            [
                {
                    "name": "1girl",
                    "cn_name": "女孩",
                    "wiki": "A girl.",
                    "post_count": "100",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )

    def test_build_providers_injects_semantic_layer_when_artifacts_exist(
        self,
    ) -> None:
        """配置端点且向量产物存在时，本地 provider 注入向量语义层。"""
        from .danbooru import FileDanbooruTagProvider
        from .danbooru_embedding import VectorSemanticTagSearcher

        # 语义层常驻 mmap 住向量产物（Windows 下不可删除），故用 mkdtemp + 忽略错误清理
        data_dir = tempfile.mkdtemp()
        try:
            self._write_single_tag_fixture(data_dir)
            with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
                with build_providers(
                    _request(),
                    get_parser(),
                    "",
                    False,
                    data_dir,
                    "http://127.0.0.1:1#model=stub",
                ) as providers:
                    danbooru = next(
                        p for p in providers if type(p).__name__ == "DanbooruProvider"
                    )
                    provider = cast(
                        FileDanbooruTagProvider, cast(Any, danbooru).provider
                    )
                    self.assertIsInstance(provider.semantic, VectorSemanticTagSearcher)
        finally:
            shutil.rmtree(data_dir, ignore_errors=True)

    def test_build_providers_semantic_inactive_without_artifacts(self) -> None:
        """配置端点但缺向量产物时注入空语义层，补全退回纯字面行为。"""
        from .danbooru import FileDanbooruTagProvider
        from .danbooru_embedding import NullSemanticTagSearcher

        with tempfile.TemporaryDirectory() as data_dir:
            self._write_single_tag_fixture(data_dir)
            os.remove(os.path.join(data_dir, COMPILED_EMBEDDINGS_FILENAME))
            with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
                with build_providers(
                    _request(),
                    get_parser(),
                    "",
                    False,
                    data_dir,
                    "http://127.0.0.1:1#model=stub",
                ) as providers:
                    danbooru = next(
                        p for p in providers if type(p).__name__ == "DanbooruProvider"
                    )
                    provider = cast(
                        FileDanbooruTagProvider, cast(Any, danbooru).provider
                    )
                    self.assertIsInstance(provider.semantic, NullSemanticTagSearcher)
                    self.assertEqual(
                        [
                            r.tag
                            for r in provider.search(
                                "1girl", cancel_token=CancelToken()
                            )
                        ],
                        ["1girl"],
                    )

    def test_build_providers_semantic_inactive_without_endpoint_url(self) -> None:
        """未配置端点时即便向量产物存在也不启用语义层。"""
        from .danbooru import FileDanbooruTagProvider
        from .danbooru_embedding import NullSemanticTagSearcher

        with tempfile.TemporaryDirectory() as data_dir:
            self._write_single_tag_fixture(data_dir)
            with _data_dir_env(), patch("comfyui.autocomplete.SQLiteContext"):
                with build_providers(
                    _request(), get_parser(), "", False, data_dir
                ) as providers:
                    danbooru = next(
                        p for p in providers if type(p).__name__ == "DanbooruProvider"
                    )
                    provider = cast(
                        FileDanbooruTagProvider, cast(Any, danbooru).provider
                    )
                    self.assertIsInstance(provider.semantic, NullSemanticTagSearcher)

    def test_build_providers_without_url_or_data_dir_skips_danbooru(self) -> None:
        """URL 与数据目录均为空时不注册 DanbooruProvider。"""
        with patch("comfyui.autocomplete.SQLiteContext"):
            with build_providers(_request(), get_parser(), "", False) as providers:
                names = [type(p).__name__ for p in providers]
        self.assertNotIn("DanbooruProvider", names)

    def test_autocomplete_after_region_selection_shows_related_tags(self) -> None:
        """闭环：选定区域后，下一次空输入补全应进入该区域的关联标签模式。"""
        mock_danbooru = MagicMock()
        mock_danbooru.related.return_value = [
            DanbooruTag("cute", "可爱", "Cute character.", "General")
        ]
        history = self._make_history()
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(mock_danbooru, history, NullSuggestionTextFormatter()),
            ],
        )
        # 区域标记写在节点文本中：既能被 extract_region_names 识别，
        # 也能被 locate_prompts 定位到该区域已有提示词
        workflow: Dict[str, Any] = {
            "nodes": [
                {
                    "id": "node_1",
                    "type": "CLIPTextEncode",
                    "title": "Prompt",
                    "widgets_values": ["// #region positive\n1girl\n// #endregion"],
                }
            ]
        }

        # 本次请求的令牌：断言它随请求一路传到 provider
        token = CancelToken()
        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample(workflow, tmp_dir)
            request = AutocompleteRequest(
                target_command="remix",
                query="",
                prev_word="positive",
                cwords=["/remix", "--add", "--region", "positive"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
                cancel_token=token,
            )
            suggestions = list(autocomplete(request, services))

        mock_danbooru.related.assert_called_once_with(
            ["1girl"],
            target_categories=["General", "Artist", "Meta"],
            cancel_token=token,
        )
        self.assertEqual([s.text for s in suggestions], ["cute"])

    # ---- NodeProvider tests ----

    def _get_mock_node_prompt_meta(self) -> Dict[str, Any]:
        return {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "1girl"},
                "_meta": {"title": "Positive Prompt"},
            },
            "node_2": {
                "class_type": "KSampler",
                "inputs": {"cfg": 8.0, "seed": 42},
                "_meta": {"title": "Main Sampler"},
            },
            "node_3": {
                "class_type": "EmptyLatentImage",
                "inputs": {"width": 512, "height": 512},
            },
            "node_4:child_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "subgraph prompt"},
            },
        }

    def _get_mock_node_workflow(self) -> Dict[str, Any]:
        return {
            "nodes": [
                {
                    "id": "node_1",
                    "type": "CLIPTextEncode",
                    "title": "Positive Prompt",
                    "widgets_values": ["// positive prompt\n1girl, masterpiece,"],
                },
                {
                    "id": "node_2",
                    "type": "KSampler",
                    "title": "Main Sampler",
                    "widgets_values": [42, "fixed", 8.0],
                },
                {
                    "id": "node_3",
                    "type": "EmptyLatentImage",
                    "widgets_values": [512, 512],
                },
                {
                    "id": "node_4",
                    "type": "MySubgraph",
                    "title": "Parent Subgraph",
                    "widgets_values": [],
                },
            ],
            "definitions": {
                "subgraphs": [
                    {
                        "id": "MySubgraph",
                        "nodes": [
                            {
                                "id": "child_1",
                                "type": "CLIPTextEncode",
                                "title": "Inner Prompt",
                            }
                        ],
                    }
                ]
            },
        }

    def test_autocomplete_node_clip_text_encode(self):
        prompt_meta = self._get_mock_node_prompt_meta()
        workflow = self._get_mock_node_workflow()
        context = self._make_context(
            target_command="remix",
            prev_word="--node",
            cwords=["/remix", "--add", "--node"],
            parsed_args=self._make_parsed_args(command="remix"),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 2)

        node_1_sug = next(s for s in suggestions if s.text == "node_1")
        # workflow 文本含注释，应优先展示
        self.assertEqual(
            node_1_sug.description, "// positive prompt\n1girl, masterpiece,"
        )
        self.assertEqual(
            node_1_sug.displayText, "#node_1 Positive Prompt (CLIPTextEncode)"
        )
        self.assertEqual(node_1_sug.type, "node")
        self.assertEqual(node_1_sug.style, "")

        sub_sug = next(s for s in suggestions if s.text == "node_4:child_1")
        self.assertEqual(sub_sug.description, "subgraph prompt")
        self.assertEqual(
            sub_sug.displayText,
            "#node_4:child_1 Parent Subgraph -> Inner Prompt (CLIPTextEncode)",
        )

    def test_autocomplete_node_adjust_cfg(self):
        prompt_meta = self._get_mock_node_prompt_meta()
        workflow = self._get_mock_node_workflow()
        context = self._make_context(
            target_command="adjust",
            prev_word="--node",
            cwords=["/remix", "--cfg", "--node"],
            parsed_args=self._make_parsed_args(command="adjust"),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "node_2")
        self.assertEqual(suggestions[0].description, "")
        self.assertEqual(suggestions[0].displayText, "#node_2 Main Sampler (KSampler)")

    def test_autocomplete_node_adjust_aspect(self):
        prompt_meta = self._get_mock_node_prompt_meta()
        workflow = self._get_mock_node_workflow()
        context = self._make_context(
            target_command="adjust",
            prev_word="--node",
            cwords=["/remix", "--aspect", "--node"],
            parsed_args=self._make_parsed_args(command="adjust"),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "node_3")
        self.assertEqual(suggestions[0].description, "")
        self.assertEqual(
            suggestions[0].displayText,
            "#node_3 EmptyLatentImage (EmptyLatentImage)",
        )

    def test_autocomplete_node_muted_style(self):
        prompt_meta = self._get_mock_node_prompt_meta()
        workflow = self._get_mock_node_workflow()
        context = self._make_context(
            target_command="remix",
            prev_word="--node",
            cwords=["/remix", "--add", "--node", "node_1", "--node"],
            parsed_args=self._make_parsed_args(command="remix", node=["node_1"]),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 2)

        node_1_sug = next(s for s in suggestions if s.text == "node_1")
        self.assertEqual(node_1_sug.style, "muted")

        sub_sug = next(s for s in suggestions if s.text == "node_4:child_1")
        self.assertEqual(sub_sug.style, "")

    def test_autocomplete_node_fallback_prompt_text_when_no_workflow(self):
        """workflow 为 None 时，description 应 fallback 到 prompt 文本"""
        prompt_meta = self._get_mock_node_prompt_meta()
        context = self._make_context(
            target_command="remix",
            prev_word="--node",
            cwords=["/remix", "--add", "--node"],
            parsed_args=self._make_parsed_args(command="remix"),
            prompt_meta=prompt_meta,
            workflow=None,
        )
        provider = NodeProvider()
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 2)
        node_1_sug = next(s for s in suggestions if s.text == "node_1")
        self.assertEqual(node_1_sug.description, "1girl")

    def test_autocomplete_node_fallback_prompt_text_when_no_workflow_text(self):
        """workflow 节点无 widgets_values 时，description 应 fallback 到 prompt 文本"""
        prompt_meta = self._get_mock_node_prompt_meta()
        workflow = self._get_mock_node_workflow()
        # 清空 node_1 的 widgets_values 模拟无 workflow 文本的场景
        workflow["nodes"][0]["widgets_values"] = []
        context = self._make_context(
            target_command="remix",
            prev_word="--node",
            cwords=["/remix", "--add", "--node"],
            parsed_args=self._make_parsed_args(command="remix"),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        suggestions = list(provider.provide(context))
        node_1_sug = next(s for s in suggestions if s.text == "node_1")
        self.assertEqual(node_1_sug.description, "1girl")

    def test_autocomplete_no_images_robustness(self):
        context = self._make_context(
            target_command="remix",
            prev_word="--node",
            cwords=["/remix", "--add", "--node"],
            parsed_args=self._make_parsed_args(command="remix"),
        )
        provider = NodeProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 0)

    def test_autocomplete_remove_all_freeze(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "节点: node_1",
            "another prompt line": "节点: node_1",
        }
        context = self._make_context(
            target_command="remix",
            prev_word="--all",
            cwords=["/remix", "--remove", "--all"],
            parsed_args=self._make_parsed_args(all=True),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        self.assertIsNotNone(suggestions)
        self.assertTrue(len(suggestions) > 0)
        self.assertTrue(
            any(s.text == '"1girl, masterpiece, score_7, cute"' for s in suggestions)
        )


class TestAutocompleteIntegration(unittest.TestCase):

    @patch("comfyui.danbooru.CachedDanbooruTagProvider")
    def test_integration_autocomplete_danbooru_related(
        self, mock_sqlite_provider_class: MagicMock
    ) -> None:
        import io
        import json
        from comfyui.autocomplete import main as autocomplete_main

        # 1. Mock Danbooru provider 的返回，包含 1 个 Character 标签，以及超过 25 个其他标签
        mock_provider = MagicMock()
        results = [
            DanbooruTag("1girl", "女孩", "A girl.", "General"),
            DanbooruTag("hatsune_miku", "初音未来", "A character.", "Character"),
        ]
        for i in range(30):
            results.append(DanbooruTag(f"tag_{i}", f"标签_{i}", f"Wiki_{i}", "General"))

        mock_provider.related.return_value = results
        mock_sqlite_provider_class.return_value = mock_provider

        # 2. 配置补全上下文环境变量
        env_vars = {
            "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
            "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "1girl,",
            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/remix", "--add", "masterpiece,", "1girl,"]',
            "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
            "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
        }

        # 3. 拦截 stdout 并运行 main()
        with _data_dir_env(), patch.dict(os.environ, env_vars), patch(
            "sys.argv", ["comfyui.autocomplete", "remix"]
        ), patch("sys.stdout", new_io := io.StringIO()):
            try:
                autocomplete_main()
            except SystemExit as e:
                self.assertEqual(e.code, 0)
            output = new_io.getvalue()

        # 4. 解析输出的 JSONL 并验证
        lines = [
            json.loads(line) for line in output.strip().splitlines() if line.strip()
        ]

        mock_sqlite_provider_class.assert_called_once()
        # 单次 main() 调用没有外部取消通道：令牌由入口自建，这里只断言它被一路传下来
        mock_provider.related.assert_called_once_with(
            ["masterpiece", "1girl"],
            target_categories=["General", "Artist", "Meta"],
            cancel_token=ANY,
        )

        tags = [item["text"] for item in lines]
        self.assertIn("1girl", tags)
        self.assertIn("tag_0", tags)

        self.assertEqual(len(lines), 20)
        self.assertEqual(lines[0]["text"], "1girl")

    @patch("comfyui.danbooru.CachedDanbooruTagProvider")
    def test_single_shot_programming_error_propagates(
        self, mock_sqlite_provider_class: MagicMock
    ) -> None:
        """单次模式下编程错误不再被吞掉：异常直接传播（进程以非零退出）。"""
        import io
        from comfyui.autocomplete import main as autocomplete_main

        mock_provider = MagicMock()
        mock_provider.search.side_effect = KeyError("boom")
        mock_sqlite_provider_class.return_value = mock_provider

        env_vars = {
            "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "girl",
            "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/remix", "--add"]',
            "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
            "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
        }

        with _data_dir_env(), patch.dict(os.environ, env_vars), patch(
            "sys.argv", ["comfyui.autocomplete", "remix"]
        ), patch("sys.stdout", io.StringIO()):
            with self.assertRaises(KeyError):
                autocomplete_main()


class TestModelFormatAutocomplete(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = tempfile.mkdtemp()
        self._env_patch = patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": self.tmp_dir}
        )
        self._env_patch.start()
        self.config = ModelFormatConfig.load()

    def tearDown(self) -> None:
        self._env_patch.stop()

    def _make_context(self, **kwargs: Any) -> AutocompleteContext:
        """Helper to build AutocompleteContext with sane defaults."""
        return _make_context_with(**kwargs)

    def _suggest(self, context: AutocompleteContext) -> List[Any]:
        provider = ModelFormatProvider(self.config)
        self.assertTrue(provider.can_provide(context))
        return list(provider.provide(context))

    def test_suggests_checkpoint_names_on_model_position(self) -> None:
        """生产 cwords（首词带斜杠、当前词在 query 中）：第一个参数位置应推荐 Checkpoint 模型名。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format"],
            prev_word="/set-model-format",
            query="",
            prompt_meta={
                "4": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
                }
            },
        )
        texts = [s.text for s in self._suggest(ctx)]
        self.assertIn("animaPencilXL_v10.safetensors", texts)

    def test_model_suggestions_filtered_by_query(self) -> None:
        """输入模型名片段时按 query 过滤模型候选。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format"],
            prev_word="/set-model-format",
            query="anima",
            prompt_meta={
                "4": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
                },
                "5": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "otherModel.safetensors"},
                },
            },
        )
        texts = [s.text for s in self._suggest(ctx)]
        self.assertIn("animaPencilXL_v10.safetensors", texts)
        self.assertNotIn("otherModel.safetensors", texts)

    def test_suggests_format_options_on_format_position(self) -> None:
        """第二个参数位置应推荐格式类型 anima/sdxl/disabled。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format", "animaModel.safetensors"],
            prev_word="animaModel.safetensors",
            query="",
            prompt_meta={},
        )
        suggestions = self._suggest(ctx)
        self.assertEqual([s.text for s in suggestions], ["anima", "sdxl", "disabled"])
        self.assertEqual(suggestions[0].type, "format")

    def test_format_options_filtered_by_query(self) -> None:
        """输入格式片段时按 query 过滤格式候选。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format", "animaModel.safetensors"],
            prev_word="animaModel.safetensors",
            query="dis",
            prompt_meta={},
        )
        texts = [s.text for s in self._suggest(ctx)]
        self.assertIn("disabled", texts)
        self.assertNotIn("anima", texts)

    def test_full_flow_suggests_model_name(self) -> None:
        """端到端：后端产出的请求参数经 autocomplete() 后应给出 Checkpoint 模型名。"""
        tmp_dir = tempfile.mkdtemp()
        workflow = {
            "nodes": [
                {
                    "id": "6",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n// #endregion"
                    ],
                }
            ]
        }
        prompt_meta = {
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
            },
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece", "clip": ["4", 0]},
            },
        }
        path = os.path.join(tmp_dir, "sample.png")
        img = Image.new("RGB", (1, 1), "white")
        meta = PngInfo()
        meta.add_text("prompt", json.dumps(prompt_meta))
        meta.add_text("workflow", json.dumps(workflow))
        img.save(path, pnginfo=meta)

        request = build_request_from_params(
            {
                "cwords": ["/set-model-format"],
                "cwordIdx": 1,
                "prevWord": "/set-model-format",
                "linePrefix": "/set-model-format ",
                "query": "",
                "imagePaths": [path],
                "rootDir": "/",
                "directoryRelPath": "",
            },
            CancelToken(),
        )
        with build_providers(
            _request(["/set-model-format"], "set-model-format"), get_parser(), "", False
        ) as providers:
            services = AutocompleteServices(parser=get_parser(), providers=providers)
            suggestions = list(autocomplete(request, services))
        texts = [s.text for s in suggestions]
        self.assertIn("animaPencilXL_v10.safetensors", texts)

    def test_model_format_description_shows_config_source(self) -> None:
        """显式映射的模型应展示配置来源的生效格式值。"""
        self.config.models["xyModel.safetensors"] = "anima"
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format"],
            prev_word="/set-model-format",
            query="",
            prompt_meta={
                "4": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "xyModel.safetensors"},
                }
            },
        )
        suggestions = self._suggest(ctx)
        self.assertEqual(len(suggestions), 1)
        desc = suggestions[0].description
        self.assertIn("当前格式: anima (配置)", desc)

    def test_model_format_description_shows_inference_source(self) -> None:
        """无显式映射、可从提示词推理的模型应展示推理来源的格式值，且不写回配置。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format"],
            prev_word="/set-model-format",
            query="",
            prompt_meta={
                "4": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
                },
                "6": {
                    "class_type": "CLIPTextEncode",
                    "inputs": {
                        "text": "blue_hair, cat_ears, detailed_eyes",
                        "clip": ["4", 0],
                    },
                },
            },
        )
        suggestions = self._suggest(ctx)
        self.assertEqual(len(suggestions), 1)
        self.assertIn("当前格式: sdxl (推理)", suggestions[0].description)
        # 展示阶段不应把推理结果持久化到配置（只读）
        self.assertNotIn("animaPencilXL_v10.safetensors", self.config.models)

    def test_model_format_description_falls_back_to_default_source(self) -> None:
        """无显式映射且无法推理（无可追溯提示词文本）时展示默认格式来源。"""
        ctx = self._make_context(
            target_command="set-model-format",
            cwords=["/set-model-format"],
            prev_word="/set-model-format",
            query="",
            prompt_meta={
                "4": {
                    "class_type": "CheckpointLoaderSimple",
                    "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
                }
            },
        )
        suggestions = self._suggest(ctx)
        self.assertEqual(len(suggestions), 1)
        self.assertIn("当前格式: sdxl (默认)", suggestions[0].description)


class TestAutocompleteTaskCoocRelease(unittest.TestCase):
    """请求完成后：无待处理请求才释放 cooc，排队中的请求继续复用已加载索引。"""

    def _run_task(self, data_dir: str, extra_active: Optional[Any] = None) -> MagicMock:
        """以 serve 的方式跑一个请求任务，返回 release_cooc_cache 的 mock。"""
        import io
        import threading

        from comfyui import autocomplete as autocomplete_module

        # 与 danbooru_test 同惯例：cast(Any) 访问模块私有实现
        task_cls = cast(Any, autocomplete_module)._AutocompleteTask

        active: Dict[Any, Any] = {}
        request = AutocompleteRequest(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            image_paths=[],
            root_dir="",
            directory_rel_path="",
            cancel_token=CancelToken(),
        )
        active_lock = threading.Lock()
        task = task_cls(
            1,
            request,
            get_parser(),
            "",
            False,
            io.StringIO(),
            active,
            active_lock,
            data_dir,
        )
        active[1] = task
        if extra_active is not None:
            active[99] = extra_active
        with patch("comfyui.autocomplete.build_providers") as mock_build, patch(
            "comfyui.autocomplete.release_cooc_cache"
        ) as mock_release:
            mock_build.return_value.__enter__.return_value = []
            task.start()
            task.join()
        return cast(MagicMock, mock_release)

    def test_releases_cooc_when_no_pending_request_left(self) -> None:
        release = self._run_task("data-dir")
        release.assert_called_once_with("data-dir")

    def test_keeps_cooc_while_other_request_pending(self) -> None:
        release = self._run_task("data-dir", extra_active=object())
        release.assert_not_called()

    def test_no_release_when_local_data_dir_disabled(self) -> None:
        release = self._run_task("")
        release.assert_not_called()


class TestAutocompleteTaskFailureReporting(unittest.TestCase):
    """取消不是失败：取消掐断的调用不记错误，与取消无关的错误必须可见。"""

    def _run_task(self, error: BaseException, canceled: bool) -> Tuple[str, MagicMock]:
        """以 serve 的方式跑一个请求任务，返回 (写出到 stdout 的内容, _LOGGER mock)。

        error 代表 `autocomplete()` 抛出的异常；canceled 表示取消信号先于异常到达。
        """
        import io
        import threading

        from comfyui import autocomplete as autocomplete_module

        task_cls = cast(Any, autocomplete_module)._AutocompleteTask
        token = CancelToken()
        request = AutocompleteRequest(
            target_command="remix",
            query="",
            prev_word="",
            cwords=["/remix", "--add"],
            image_paths=[],
            root_dir="",
            directory_rel_path="",
            cancel_token=token,
        )
        out = io.StringIO()
        task = task_cls(
            1, request, get_parser(), "", False, out, {}, threading.Lock(), ""
        )
        if canceled:
            token.cancel()
        with patch("comfyui.autocomplete.build_providers") as mock_build, patch(
            "comfyui.autocomplete.autocomplete", side_effect=error
        ), patch("comfyui.autocomplete._LOGGER") as mock_logger:
            mock_build.return_value.__enter__.return_value = []
            task.start()
            task.join()
        return out.getvalue(), cast(MagicMock, mock_logger)

    def test_canceled_network_abort_is_not_logged_as_failure(self) -> None:
        out, logger = self._run_task(
            HttpCanceledError("请求已取消，在途请求已被掐断"), canceled=True
        )
        logger.error.assert_not_called()
        self.assertEqual(out, "", "已取消的请求不应返回响应")

    def test_canceled_request_still_logs_unrelated_error(self) -> None:
        """取消只免去回传结果，不免去报错：与取消无关的错误仍要留下可排查的记录。"""
        out, logger = self._run_task(KeyError("missing tag"), canceled=True)
        logger.error.assert_called_once()
        self.assertEqual(out, "", "已取消的请求不应返回响应")


class TestAddSuggestionModelFormat(unittest.TestCase):
    """`/add` 建议按目标节点模型的提示词格式输出，使其与提交后落盘形态逐字符一致。"""

    def setUp(self) -> None:
        self.tmp_dir = tempfile.mkdtemp()
        self._env_patch = patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": self.tmp_dir}
        )
        self._env_patch.start()

    def tearDown(self) -> None:
        self._env_patch.stop()
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def _set_model_format(self, ckpt_name: str, fmt: str) -> None:
        config = ModelFormatConfig.load()
        config.models[ckpt_name] = fmt
        config.save()

    def _make_workflow_data(
        self, ckpt_name: str, node_text: str
    ) -> Tuple[Dict[str, Any], Dict[str, Any]]:
        """构造带检查点格式化链路的 CLIPTextEncode 工作流夹具。"""
        workflow: Dict[str, Any] = {
            "nodes": [
                {"id": "6", "type": "CLIPTextEncode", "widgets_values": [node_text]}
            ]
        }
        prompt_meta: Dict[str, Any] = {
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": node_text, "clip": ["4", 0]},
            },
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": ckpt_name},
            },
        }
        return workflow, prompt_meta

    def _make_context(
        self,
        *,
        ckpt_name: Optional[str] = "animaPencilXL_v10.safetensors",
        node_text: str = "masterpiece, Blue_Hair",
        inference_text: str = "",
        query: str = "",
        prev_word: str = "",
        cwords: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> AutocompleteContext:
        """构造带目标节点的新增标签补全上下文。"""
        defaults: Dict[str, Any] = dict(
            target_command="remix",
            query=query,
            prev_word=prev_word,
            cwords=cwords if cwords is not None else ["/remix", "--add"],
            image_paths=[],
            parsed_args=argparse.Namespace(
                command="remix",
                neg=False,
                all=False,
                region=None,
                node=None,
                raw=False,
                hard=False,
                no_skip=False,
            ),
            seen_prompts={},
        )
        if ckpt_name is not None:
            workflow, prompt_meta = self._make_workflow_data(ckpt_name, node_text)
            defaults["workflow"] = workflow
            defaults["prompt_meta"] = prompt_meta
            defaults["prompt_target"] = PromptTarget(
                node_id="6", inference_text=inference_text or node_text
            )
        # kwargs 最后覆盖，便于个别用例替换工作流元数据等夹具
        return _make_context_with(**{**defaults, **kwargs})

    def _make_provider(
        self,
        mock_provider: Any,
        history: Any,
        *,
        formatter: Optional[Any] = None,
    ) -> DanbooruProvider:
        """构造注入了真实格式化接缝的 DanbooruProvider。"""
        return DanbooruProvider(
            mock_provider,
            history,
            (
                formatter
                if formatter is not None
                else NodeTextFormatter(ModelFormatConfig.load())
            ),
        )

    def _make_history(
        self,
        added: Optional[Set[str]] = None,
        added_times: Optional[Dict[str, str]] = None,
        all_added: Optional[List[Tuple[str, str]]] = None,
    ) -> MagicMock:
        history = MagicMock()
        history.get_added_prompts.return_value = set(added or [])
        history.get_added_prompt_times.return_value = dict(added_times or {})
        history.get_all_added_prompts.return_value = list(all_added or [])
        return history

    # ---- 搜索建议 ----

    def test_search_suggestions_formatted_as_anima(self) -> None:
        """anima 格式模型：建议插入文本小写、下划线转空格，含空格者加引号。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("Blue_Eyes", "蓝眼", "Blue eyes.", "General"),
            DanbooruTag("score_9", "评分", "Score.", "Meta"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        suggestions = list(provider.provide(self._make_context(query="blue")))

        self.assertEqual(suggestions[0].text, '"blue eyes"')
        # displayText 保持原文（含中文名），仅用于浮层展示
        self.assertEqual(suggestions[0].displayText, "Blue_Eyes (蓝眼)")
        # score_* 评分标签在 anima 下保留下划线，无需引号
        self.assertEqual(suggestions[1].text, "score_9")

    def test_search_suggestions_formatted_as_sdxl(self) -> None:
        """sdxl 格式模型：建议插入文本空格转下划线。"""
        self._set_model_format("sdxlModel.safetensors", "sdxl")
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("blue eyes", "蓝眼", "Blue eyes.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        suggestions = list(provider.provide(self._make_context(query="blue")))

        self.assertEqual(suggestions[0].text, "blue_eyes")

    def test_suggestions_keep_original_when_model_format_disabled(self) -> None:
        """模型为 disabled（opt-out）时建议保持原文。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "disabled")
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("Blue_Eyes", "蓝眼", "Blue eyes.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        suggestions = list(provider.provide(self._make_context(query="blue")))

        self.assertEqual(suggestions[0].text, "Blue_Eyes")

    def test_suggestions_keep_original_when_model_not_traceable(self) -> None:
        """目标节点已解析但模型无法追溯（如 clip 链终止于普通 CLIPLoader）时保持原文。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("Blue_Eyes", "蓝眼", "Blue eyes.", "General"),
        ]
        prompt_meta: Dict[str, Any] = {
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece", "clip": ["5", 0]},
            },
            # 普通 CLIPLoader 的 clip_name 不是格式键，clip 链到此终止
            "5": {
                "class_type": "CLIPLoader",
                "inputs": {"clip_name": "sdxl15.safetensors"},
            },
        }

        provider = self._make_provider(mock_provider, self._make_history())
        context = self._make_context(
            query="blue", prompt_meta=prompt_meta, workflow={"nodes": []}
        )
        suggestions = list(provider.provide(context))

        self.assertEqual(suggestions[0].text, "Blue_Eyes")

    def test_suggestions_keep_original_without_workflow_metadata(self) -> None:
        """图片无 ComfyUI 元数据（未解析出目标节点）时保持原文。"""
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("Blue_Eyes", "蓝眼", "Blue eyes.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        suggestions = list(
            provider.provide(self._make_context(ckpt_name=None, query="blue"))
        )

        self.assertEqual(suggestions[0].text, "Blue_Eyes")

    def test_inferred_model_format_is_persisted_then_reused(self) -> None:
        """补全沿用提交路径的格式推导（ADR 0006）：首次推理写回配置，之后不再落盘。"""
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("Blue_Eyes", "蓝眼", "Blue eyes.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        context = self._make_context(node_text="masterpiece, blue hair", query="blue")
        self.assertEqual(list(provider.provide(context))[0].text, '"blue eyes"')
        self.assertEqual(
            ModelFormatConfig.load().models, {"animaPencilXL_v10.safetensors": "anima"}
        )

        # 之后命中「配置」来源：格式一致且不再写文件
        config_file = os.path.join(self.tmp_dir, "comfyui_model_formats.toml")
        mtime_before = os.path.getmtime(config_file)
        self.assertEqual(list(provider.provide(context))[0].text, '"blue eyes"')
        self.assertEqual(os.path.getmtime(config_file), mtime_before)

    # ---- 关联联想与操作历史 ----

    def test_related_suggestions_formatted(self) -> None:
        """关联联想来源的建议同样按目标模型格式重排。"""
        self._set_model_format("sdxlModel.safetensors", "sdxl")
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("blue eyes", "蓝眼", "Blue eyes.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        context = self._make_context(
            prev_word="masterpiece,",
            cwords=["/remix", "--add", "1girl,", "masterpiece,"],
        )
        suggestions = list(provider.provide(context))

        self.assertEqual(suggestions[0].text, "blue_eyes")

    def test_history_suggestions_formatted(self) -> None:
        """操作历史来源的建议同样按目标模型格式重排，展示文本保持原文。"""
        self._set_model_format("sdxlModel.safetensors", "sdxl")
        history = self._make_history(all_added=[("blue eyes", "2026-08-07T10:00:00")])
        provider = self._make_provider(MagicMock(), history)

        suggestions = list(provider.provide(self._make_context()))

        self.assertEqual(suggestions[0].text, "blue_eyes")
        self.assertEqual(suggestions[0].displayText, "blue eyes")
        self.assertIn("blue eyes", suggestions[0].description)

    def test_formatted_suggestion_still_marked_as_already_in_workflow(self) -> None:
        """已见提示词集合与建议文本在同一形态下比对，「(已有)」标记不失效。

        工作流里还是旧的下划线形态（目标模型是 anima）时也必须能命中。
        """
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("Blue_Hair", "蓝发", "Blue hair.", "General"),
        ]

        provider = self._make_provider(mock_provider, self._make_history())
        context = self._make_context(
            node_text="masterpiece,\nBlue_Hair",
            seen_prompts={
                "masterpiece,": "区域: positive",
                "Blue_Hair": "区域: positive",
            },
        )
        suggestions = list(provider.provide(context))

        self.assertEqual(suggestions[0].text, '"blue hair"')
        self.assertEqual(suggestions[0].style, "muted")
        self.assertTrue(suggestions[0].description.startswith("(已有) "))

    # ---- 其它指令不受影响 ----

    def test_remove_suggestions_keep_original_text(self) -> None:
        """移除列出工作流中已有标签文本，保持原文，不按目标模型格式重排。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        seen_prompts = {"Blue_Hair": "区域: positive"}
        provider = WorkflowPromptProvider()
        context = _make_context_with(
            target_command="remix",
            cwords=["/remix", "--remove"],
            parsed_args=argparse.Namespace(command="remix", neg=False),
            seen_prompts=seen_prompts,
        )
        self.assertTrue(provider.can_provide(context))
        self.assertEqual([s.text for s in provider.provide(context)], ["Blue_Hair"])

    # ---- 目标解析 ----

    def _write_sample(
        self, workflow: Dict[str, Any], prompt_meta: Dict[str, Any]
    ) -> str:
        img = Image.new("RGB", (1, 1), "white")
        meta = PngInfo()
        meta.add_text("prompt", json.dumps(prompt_meta))
        meta.add_text("workflow", json.dumps(workflow))
        path = os.path.join(
            self.tmp_dir, f"sample_{abs(hash(json.dumps(prompt_meta)))}.png"
        )
        img.save(path, pnginfo=meta)
        return path

    def _run_autocomplete(
        self,
        image_paths: List[str],
        cwords: List[str],
        query: str = "",
        tag: str = "blue eyes",
    ) -> List[Any]:
        """以真实图片元数据跑一遍 autocomplete，返回建议列表。"""
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag(tag, "蓝眼", "Blue eyes.", "General"),
        ]
        request = AutocompleteRequest(
            target_command="remix",
            query=query,
            prev_word="",
            cwords=cwords,
            image_paths=image_paths,
            root_dir="",
            directory_rel_path="",
            cancel_token=CancelToken(),
        )
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[self._make_provider(mock_provider, self._make_history())],
        )
        return list(autocomplete(request, services))

    def test_default_region_resolves_target_model_format(self) -> None:
        """未指定 --region/--node 时按默认 positive 区域解析目标模型格式。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        workflow, prompt_meta = self._make_workflow_data(
            "animaPencilXL_v10.safetensors",
            "// #region positive\nmasterpiece,\n// #endregion\n"
            "// #region negative\nworst quality\n// #endregion\n",
        )
        image_path = self._write_sample(workflow, prompt_meta)

        suggestions = self._run_autocomplete(
            [image_path], ["/remix", "--add"], query="blue", tag="Blue_Eyes"
        )

        self.assertEqual([s.text for s in suggestions], ['"blue eyes"'])

    def test_first_target_wins_when_multiple_targets_specified(self) -> None:
        """多目标时按第一个目标解析格式。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        self._set_model_format("sdxlModel.safetensors", "sdxl")
        workflow = {
            "nodes": [
                {
                    "id": "6",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["masterpiece,"],
                },
                {
                    "id": "7",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["worst quality,"],
                },
            ]
        }
        prompt_meta: Dict[str, Any] = {
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,", "clip": ["4", 0]},
            },
            "7": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "worst quality,", "clip": ["5", 0]},
            },
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
            },
            "5": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "sdxlModel.safetensors"},
            },
        }
        image_path = self._write_sample(workflow, prompt_meta)

        suggestions = self._run_autocomplete(
            [image_path],
            ["/remix", "--add", "--node", "6", "--node", "7"],
            query="blue",
            tag="Blue_Eyes",
        )

        self.assertEqual([s.text for s in suggestions], ['"blue eyes"'])

    def test_neg_flag_resolves_negative_region_format(self) -> None:
        """--neg 时按 negative 默认区域解析目标模型格式。"""
        self._set_model_format("sdxlModel.safetensors", "sdxl")
        workflow = {
            "nodes": [
                {
                    "id": "6",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "// #region positive\nmasterpiece,\n// #endregion\n"
                        "// #region negative\nblue eyes\n// #endregion\n"
                    ],
                }
            ]
        }
        prompt_meta: Dict[str, Any] = {
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "masterpiece,", "clip": ["4", 0]},
            },
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "sdxlModel.safetensors"},
            },
        }
        image_path = self._write_sample(workflow, prompt_meta)

        suggestions = self._run_autocomplete(
            [image_path], ["/remix", "--add", "--neg"], query="blue"
        )

        self.assertEqual([s.text for s in suggestions], ["blue_eyes"])

    # ---- 建议与落盘一致 ----

    def test_suggestion_text_matches_text_written_on_submit(self) -> None:
        """补全给出的插入文本与同一输入提交后落盘的文本逐字符一致。"""
        self._set_model_format("animaPencilXL_v10.safetensors", "anima")
        workflow, prompt_meta = self._make_workflow_data(
            "animaPencilXL_v10.safetensors", "masterpiece, Blue_Hair"
        )
        image_path = self._write_sample(workflow, prompt_meta)

        suggestions = self._run_autocomplete(
            [image_path], ["/remix", "--add"], query="cat", tag="Cat_Ears"
        )
        self.assertEqual(suggestions[0].text, '"cat ears"')

        # 用 Go 端 splitArgs 的还原规则去掉引号后提交，再走提交路径的格式化
        submitted = suggestions[0].text[1:-1]
        pair = WorkflowPromptPair(json.loads(json.dumps(workflow)), prompt_meta)
        format_workflow_prompt_pair(pair)
        fragment = PromptFragment(pair, "6")
        self.assertTrue(fragment.add(submitted))
        # 落盘文本逐字符等于建议文本：已有标签与新增标签同属一种形态
        self.assertEqual(
            pair.get_prompt_input("6", "text"), "masterpiece, blue hair\ncat ears,"
        )
        self.assertEqual(
            pair.get_workflow_node_text("6"), "masterpiece, blue hair\ncat ears,"
        )

    # ---- 依赖构建 ----

    def test_build_providers_injects_model_formatter_for_add_only(self) -> None:
        """格式化依赖仅注入 /add；其余指令显式注入空实现。"""
        with patch("comfyui.autocomplete.SQLiteContext"):
            with build_providers(
                _request(), get_parser(), "http://localhost", False
            ) as add_providers:
                with build_providers(
                    _request(cwords=["/remix", "--remove"]),
                    get_parser(),
                    "http://localhost",
                    False,
                ) as remove_providers:
                    add_formatter = cast(
                        Any,
                        next(
                            p for p in add_providers if isinstance(p, DanbooruProvider)
                        ),
                    ).text_formatter
                    remove_formatter = cast(
                        Any,
                        next(
                            p
                            for p in remove_providers
                            if isinstance(p, DanbooruProvider)
                        ),
                    ).text_formatter
        self.assertIsInstance(add_formatter, NodeTextFormatter)
        self.assertIsInstance(remove_formatter, NullSuggestionTextFormatter)

    def test_build_providers_add_missing_data_dir_fails_fast(self) -> None:
        """`/add` 缺 IMAGE_FUNNEL_DATA_DIR 时快速失败，不静默输出原文建议。"""
        with patch("comfyui.autocomplete.SQLiteContext"):
            with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": ""}):
                with self.assertRaises(MissingDataDirError):
                    with build_providers(
                        _request(), get_parser(), "http://localhost", False
                    ):
                        pass


if __name__ == "__main__":
    unittest.main()
