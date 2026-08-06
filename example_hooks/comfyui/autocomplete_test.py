import logging
import unittest
import os
import argparse
from typing import Dict, Any
from unittest.mock import patch, MagicMock

logging.disable(logging.CRITICAL)
from .autocomplete import (
    AutocompleteContext,
    WorkflowPromptProvider,
    DanbooruProvider,
    NodeProvider,
    LoraProvider,
)
from .danbooru import DanbooruTag


class TestComfyUIAutocomplete(unittest.TestCase):

    def _make_context(self, **kwargs: Any) -> AutocompleteContext:
        """Helper to build AutocompleteContext with sane defaults."""
        defaults: Dict[str, Any] = dict(
            target_command=None,
            query="",
            prev_word="",
            cwords=[],
            image_paths=[],
            parsed_args=None,
            seen_prompts={},
            workflow=None,
            prompt_meta=None,
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
        provider3 = DanbooruProvider(MagicMock())
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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

        with patch(
            "comfyui.autocomplete.get_added_prompts", return_value={"1girl"}
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
            },
        ):
            # 场景 1：有 query，执行前缀语义搜索
            context1 = self._make_context(
                target_command="add",
                query="girl",
                prev_word="",
                cwords=["/add"],
                parsed_args=self._make_parsed_args(command="add"),
            )
            provider = DanbooruProvider(mock_provider)
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

        with patch(
            "comfyui.autocomplete.get_all_added_prompts", return_value=mock_history
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
            },
        ):
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
            provider = DanbooruProvider(MagicMock())
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

        with patch(
            "comfyui.autocomplete.get_all_added_prompts", return_value=mock_history
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
            },
        ):
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
            provider = DanbooruProvider(MagicMock())
            suggestions = list(provider.provide(context))

            # 应只返回 "solo"（"1girl" 已在提示词中）
            self.assertEqual(len(suggestions), 1)
            self.assertEqual(suggestions[0].text, "solo")

    def test_autocomplete_danbooru_history_fallback_no_env(self) -> None:
        """没有环境变量时，历史回退不应生效"""
        with patch.dict(os.environ, clear=True):
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
            provider = DanbooruProvider(MagicMock())
            suggestions = list(provider.provide(context))

            # 无历史环境变量，应返回空
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

        with patch(
            "comfyui.autocomplete.get_all_added_prompts", return_value=mock_history
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
            },
        ):
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
            provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
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
        provider = DanbooruProvider(mock_provider)
        self.assertTrue(provider.can_provide(context))

        suggestions = list(provider.provide(context))

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "cute")

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


if __name__ == "__main__":
    unittest.main()
