import logging
import unittest
import os
import json
import argparse
import tempfile
import requests
import sqlite3
from typing import Dict, Any, Optional, List, Set, Tuple
from unittest.mock import patch, MagicMock

from PIL import Image
from PIL.PngImagePlugin import PngInfo

logging.disable(logging.CRITICAL)
from .autocomplete import (
    AutocompleteContext,
    WorkflowPromptProvider,
    DanbooruProvider,
    NodeProvider,
    LoraProvider,
    RegionOptionProvider,
    ModelFormatProvider,
    AutocompleteRequest,
    AutocompleteServices,
    autocomplete,
    build_providers,
    build_request_from_params,
)
from .__main__ import get_parser
from .danbooru import DanbooruTag
from .model_format import ModelFormatConfig


class TestComfyUIAutocomplete(unittest.TestCase):

    def _make_context(self, **kwargs: Any) -> AutocompleteContext:
        """Helper to build AutocompleteContext with sane defaults."""
        defaults: Dict[str, Any] = dict(
            target_command="",
            query="",
            prev_word="",
            cwords=[],
            image_paths=[],
            parsed_args=None,
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
            parser=get_parser(),
        )
        defaults.update(kwargs)
        return AutocompleteContext(**defaults)

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

    def test_autocomplete_remove_prompt_normal(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remove",
            cwords=["/remove"],
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
            target_command="remove",
            query="cute",
            cwords=["/remove", "--region", "positive"],
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
            target_command="remove",
            prev_word="--region",
            cwords=["/remove", "--region"],
            parsed_args=self._make_parsed_args(),
        )
        provider = WorkflowPromptProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_remove_prompt_skip_on_dash_query(self):
        context = self._make_context(
            target_command="remove",
            query="--r",
            prev_word="",
            cwords=["/remove"],
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
            target_command="remove",
            cwords=["/delete_prompt"],
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
            target_command="remove",
            query="-a",
            prev_word="--",
            cwords=["/remove", "--"],
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
            target_command="adjust",
            prev_word="my_lora",
            cwords=["/adjust", "lora", "my_lora"],
            parsed_args=argparse.Namespace(
                command="adjust", adjust_type="lora", name="my_lora", weight="0.8"
            ),
        )
        provider = LoraProvider()
        self.assertFalse(provider.can_provide(context))
        provider2 = WorkflowPromptProvider()
        self.assertFalse(provider2.can_provide(context))
        provider3 = DanbooruProvider(MagicMock(), self._make_history())
        self.assertFalse(provider3.can_provide(context))

    def test_autocomplete_adjust_prompt_normal(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="adjust",
            cwords=["/adjust", "prompt"],
            parsed_args=self._make_parsed_args(
                command="adjust",
                adjust_type="prompt",
                text="dummy_prompt",
                weight="dummy_weight",
            ),
            seen_prompts=seen_prompts,
        )
        provider = WorkflowPromptProvider()
        self.assertTrue(provider.can_provide(context))
        suggestions = list(provider.provide(context))
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 2)
        self.assertIn('"1girl, masterpiece, score_7, cute"', texts)
        self.assertIn('"another prompt line"', texts)

    def test_autocomplete_adjust_prompt_skip_by_text(self):
        context = self._make_context(
            target_command="adjust",
            prev_word='"another prompt line"',
            cwords=["/adjust", "prompt", '"another prompt line"'],
            parsed_args=None,
            seen_prompts={"1girl, masterpiece, score_7, cute": "区域: positive"},
        )
        provider = WorkflowPromptProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_adjust_prompt_skip_by_weight(self):
        context = self._make_context(
            target_command="adjust",
            query="1",
            prev_word='"another prompt line"',
            cwords=["/adjust", "prompt", '"another prompt line"', "1"],
            parsed_args=None,
            seen_prompts={},
        )
        provider = WorkflowPromptProvider()
        self.assertFalse(provider.can_provide(context))

    def test_autocomplete_remove_prompt_with_double_dash_and_option(self):
        seen_prompts = {
            "1girl, masterpiece, score_7, cute": "区域: positive",
            "another prompt line": "区域: positive",
        }
        context = self._make_context(
            target_command="remove",
            query="a",
            prev_word="--region",
            cwords=["/remove", "--", "--region"],
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

    def test_autocomplete_add_prompt_danbooru(self) -> None:
        mock_provider = MagicMock()
        mock_provider.search.return_value = [
            DanbooruTag("1girl", "女孩", "A female character.", "General"),
            DanbooruTag(
                "dunyarzad_(genshin_impact)", "迪娜泽黛", "A noble girl.", "General"
            ),
        ]

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.search.assert_called_once_with("girl")

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
            target_command="add",
            query="pos",
            prev_word="--region",
            cwords=["/add", "--region"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        self.assertFalse(provider.can_provide(context2))

    def test_autocomplete_add_prompt_danbooru_related(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("sailor_collar", "水手领", "A collar style.", "General"),
            DanbooruTag("skirt", "裙子", "A bottom wear.", "General"),
        ]

        context = self._make_context(
            target_command="add",
            query="",
            prev_word="1girl,",
            cwords=["/add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.related.assert_called_once_with(
            ["masterpiece", "1girl"],
            target_categories=["General", "Artist", "Meta"],
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
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.related.assert_called_once_with(
            ["1girl", "masterpiece"],
            target_categories=["General", "Artist", "Meta"],
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
        provider = DanbooruProvider(mock_provider, history)

        # 场景 1：有 query，执行前缀语义搜索
        context1 = self._make_context(
            target_command="add",
            query="girl",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        suggestions1 = list(provider.provide(context1))

        self.assertEqual(len(suggestions1), 2)
        self.assertEqual(suggestions1[0].text, "1girl")
        self.assertEqual(suggestions1[0].style, "muted")
        self.assertEqual(suggestions1[1].text, "solo")
        self.assertEqual(suggestions1[1].style, "")

        # 场景 2：空 query，执行关联联想
        context2 = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
            MagicMock(), self._make_history(all_added=mock_history)
        )

        # 空 query，无 cwords，无 workflow
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
            MagicMock(), self._make_history(all_added=mock_history)
        )

        # seen_prompts 中包含 "1girl"
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
        provider = DanbooruProvider(MagicMock(), self._make_history())
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
            mock_provider, self._make_history(all_added=mock_history)
        )

        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
            target_command="add",
            query="girl",
            prev_word="--neg",
            cwords=["/add", "--neg"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
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
            target_command="add",
            query="girl",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        mock_provider.search.assert_called_once_with("girl")
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")

    def test_autocomplete_add_prompt_region_related(self) -> None:
        mock_provider = MagicMock()
        mock_provider.related.return_value = [
            DanbooruTag("cute", "可爱", "Cute character.", "General"),
        ]

        seen_prompts = {"1girl, masterpiece, score_7, cute": "区域: positive"}
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
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
            target_command="add",
            query="girl",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
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
            target_command="add",
            query="",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
            seen_prompts=seen_prompts,
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = DanbooruProvider(mock_provider, self._make_history())
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "cute")

    # ---- Danbooru 失败处理：预期错误显示 ⚠，编程错误传播 ----

    def test_autocomplete_danbooru_search_upstream_error_yields_error_item(
        self,
    ) -> None:
        """上游搜索网络失败（requests.RequestException）以 ⚠ 建议项呈现给用户。"""
        mock_provider = MagicMock()
        mock_provider.search.side_effect = requests.RequestException("timeout")

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].type, "error")
        self.assertIn("搜索失败", suggestions[0].displayText)
        self.assertEqual(suggestions[0].text, "")

    def test_autocomplete_danbooru_search_programming_error_propagates(
        self,
    ) -> None:
        """上游搜索的编程错误（非 requests.RequestException）不再被吞掉：直接抛出。"""
        mock_provider = MagicMock()
        mock_provider.search.side_effect = KeyError("tag")

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())

        with self.assertRaises(KeyError):
            list(provider.provide(context))

    def test_autocomplete_danbooru_related_upstream_error_yields_error_item(
        self,
    ) -> None:
        """上游关联联想网络失败（requests.RequestException）以 ⚠ 建议项呈现给用户。"""
        mock_provider = MagicMock()
        mock_provider.related.side_effect = requests.RequestException(
            "connection refused"
        )

        context = self._make_context(
            target_command="add",
            query="",
            prev_word="1girl,",
            cwords=["/add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].type, "error")
        self.assertIn("搜索失败", suggestions[0].displayText)

    def test_autocomplete_danbooru_related_internal_error_propagates(
        self,
    ) -> None:
        """上游关联联想的内部错误（sqlite3.Error，非 requests.RequestException）不再被吞掉。"""
        mock_provider = MagicMock()
        mock_provider.related.side_effect = sqlite3.OperationalError(
            "database is locked"
        )

        context = self._make_context(
            target_command="add",
            query="",
            prev_word="1girl,",
            cwords=["/add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider(mock_provider, self._make_history())

        with self.assertRaises(sqlite3.Error):
            list(provider.provide(context))

    def test_autocomplete_danbooru_history_error_yields_error_item(self) -> None:
        """历史读取失败（sqlite3.Error）以 ⚠ 建议项呈现给用户。"""
        history = self._make_history()
        history.get_all_added_prompts.side_effect = sqlite3.OperationalError(
            "database is locked"
        )
        provider = DanbooruProvider(MagicMock(), history)
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
        provider = DanbooruProvider(MagicMock(), history)
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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

    def test_region_option_provider_suggests_all_regions_on_empty_input(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
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
            target_command="add",
            query="g",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_when_prompt_words_typed(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="1girl,",
            cwords=["/add", "1girl,"],
            parsed_args=self._make_parsed_args(command="add"),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_when_region_specified(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_when_node_specified(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="5",
            cwords=["/add", "--node", "5"],
            parsed_args=self._make_parsed_args(command="add", node=["5"]),
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_without_workflow(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_skips_without_region_markers(self):
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
            workflow={"nodes": []},
            prompt_meta={},
        )
        provider = RegionOptionProvider()
        self.assertFalse(provider.can_provide(context))

    def test_region_option_provider_suggests_regions_with_neg_flag(self):
        """--neg 只改变默认区域，不应阻止显式选择区域的建议。"""
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="--neg",
            cwords=["/add", "--neg"],
            parsed_args=self._make_parsed_args(command="add", neg=True),
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
            target_command="add",
            query="",
            prev_word="",
            cwords=["/add"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertTrue(provider.can_provide(context_plain))

        # 扫描到 --region 视为已指定目标
        context_targeted = self._make_context(
            target_command="add",
            query="",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        self.assertFalse(provider.can_provide(context_targeted))

    def test_region_option_provider_target_scan_respects_end_of_options(self):
        """结束标记 -- 之后的 --region 属于提示词文本，不算指定目标。"""
        context = self._make_context(
            target_command="add",
            query="",
            prev_word="--region",
            cwords=["/add", "--", "--region"],
            parsed_args=None,
            workflow=self._get_mock_region_workflow(),
            prompt_meta=self._get_mock_region_prompt_meta(),
        )
        provider = RegionOptionProvider()
        self.assertTrue(provider.can_provide(context))

    def test_autocomplete_prefers_region_options_over_danbooru_related(self) -> None:
        """空输入且有可用区域时，区域选项独占建议，关联联想与历史回退均不执行。"""
        mock_danbooru = MagicMock()
        history = self._make_history(all_added=[("history_tag", "2026-08-07T10:00:00")])
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(mock_danbooru, history),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample(
                self._get_mock_region_workflow(), tmp_dir
            )
            request = AutocompleteRequest(
                target_command="add",
                query="",
                prev_word="",
                cwords=["/add"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
            )
            suggestions = list(autocomplete(request, services))

        self.assertEqual(
            [s.text for s in suggestions],
            ["--region positive", "--region negative"],
        )
        mock_danbooru.search.assert_not_called()
        mock_danbooru.related.assert_not_called()
        history.get_all_added_prompts.assert_not_called()

    def test_autocomplete_falls_back_to_danbooru_related_without_regions(self) -> None:
        """无区域标记时回落到 Danbooru 提供者（历史回退路径照常执行）。"""
        mock_danbooru = MagicMock()
        history = self._make_history(all_added=[("history_tag", "2026-08-07T10:00:00")])
        services = AutocompleteServices(
            parser=get_parser(),
            providers=[
                RegionOptionProvider(),
                DanbooruProvider(mock_danbooru, history),
            ],
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample({"nodes": []}, tmp_dir)
            request = AutocompleteRequest(
                target_command="add",
                query="",
                prev_word="",
                cwords=["/add"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
            )
            suggestions = list(autocomplete(request, services))

        texts = [s.text for s in suggestions]
        self.assertEqual(texts, ["history_tag"])
        mock_danbooru.search.assert_not_called()
        mock_danbooru.related.assert_not_called()
        history.get_all_added_prompts.assert_called_once()

    def test_build_providers_registers_region_option_before_danbooru(self) -> None:
        """真实 provider 链中区域选项提供者必须注册在 Danbooru 提供者之前。"""
        with patch("comfyui.autocomplete.SQLiteContext"):
            with build_providers(
                "root", "dir", "http://localhost", False, "add"
            ) as providers:
                names = [type(p).__name__ for p in providers]
        self.assertLess(
            names.index("RegionOptionProvider"), names.index("DanbooruProvider")
        )

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
                DanbooruProvider(mock_danbooru, history),
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

        with tempfile.TemporaryDirectory() as tmp_dir:
            sample_path = self._write_workflow_sample(workflow, tmp_dir)
            request = AutocompleteRequest(
                target_command="add",
                query="",
                prev_word="positive",
                cwords=["/add", "--region", "positive"],
                image_paths=[sample_path],
                root_dir="mock_root",
                directory_rel_path="mock_rel",
            )
            suggestions = list(autocomplete(request, services))

        mock_danbooru.related.assert_called_once_with(
            ["1girl"], target_categories=["General", "Artist", "Meta"]
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
            target_command="add",
            prev_word="--node",
            cwords=["/add", "--node"],
            parsed_args=self._make_parsed_args(command="add"),
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
            cwords=["/adjust", "cfg", "--node"],
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
            cwords=["/adjust", "aspect", "--node"],
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
            target_command="add",
            prev_word="--node",
            cwords=["/add", "--node", "node_1", "--node"],
            parsed_args=self._make_parsed_args(command="add", node=["node_1"]),
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
            target_command="add",
            prev_word="--node",
            cwords=["/add", "--node"],
            parsed_args=self._make_parsed_args(command="add"),
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
            target_command="add",
            prev_word="--node",
            cwords=["/add", "--node"],
            parsed_args=self._make_parsed_args(command="add"),
            prompt_meta=prompt_meta,
            workflow=workflow,
        )
        provider = NodeProvider()
        suggestions = list(provider.provide(context))
        node_1_sug = next(s for s in suggestions if s.text == "node_1")
        self.assertEqual(node_1_sug.description, "1girl")

    def test_autocomplete_no_images_robustness(self):
        context = self._make_context(
            target_command="add",
            prev_word="--node",
            cwords=["/add", "--node"],
            parsed_args=self._make_parsed_args(command="add"),
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
            target_command="remove",
            prev_word="--all",
            cwords=["/remove", "--all"],
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

    @patch("comfyui.autocomplete.SQLiteDanbooruTagProvider")
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
            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/add", "masterpiece,", "1girl,"]',
            "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
            "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
        }

        # 3. 拦截 stdout 并运行 main()
        with patch.dict(os.environ, env_vars), patch(
            "sys.argv", ["comfyui.autocomplete", "add"]
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
        mock_provider.related.assert_called_once_with(
            ["masterpiece", "1girl"],
            target_categories=["General", "Artist", "Meta"],
        )

        tags = [item["text"] for item in lines]
        self.assertIn("1girl", tags)
        self.assertIn("tag_0", tags)

        self.assertEqual(len(lines), 20)
        self.assertEqual(lines[0]["text"], "1girl")

    @patch("comfyui.autocomplete.SQLiteDanbooruTagProvider")
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
            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/add"]',
            "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
            "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
        }

        with patch.dict(os.environ, env_vars), patch(
            "sys.argv", ["comfyui.autocomplete", "add"]
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
        defaults: Dict[str, Any] = dict(
            target_command="",
            query="",
            prev_word="",
            cwords=[],
            image_paths=[],
            parsed_args=None,
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
            parser=get_parser(),
        )
        defaults.update(kwargs)
        return AutocompleteContext(**defaults)

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
            }
        )
        with build_providers("", "", "", False, "set-model-format") as providers:
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


if __name__ == "__main__":
    unittest.main()
