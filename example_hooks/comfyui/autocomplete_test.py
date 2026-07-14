import unittest
import os
import argparse
from typing import Dict, Any
from unittest.mock import patch, MagicMock
from .autocomplete import (
    AutocompleteContext,
    WorkflowPromptProvider,
    DanbooruProvider,
    NodeProvider,
    LoraProvider,
)


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
        provider3 = DanbooruProvider()
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

    def test_autocomplete_add_prompt_danbooru(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
                {
                    "tag": "dunyarzad_(genshin_impact)",
                    "cn_name": "迪娜泽黛",
                    "wiki": "A noble girl.",
                },
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="",
            cwords=["/add"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/search")

        req_data = call_kwargs.get("json")
        self.assertEqual(req_data["query"], "girl")
        self.assertEqual(req_data["top_k"], 20)
        self.assertEqual(req_data["limit"], 20)
        self.assertFalse(req_data["show_nsfw"])

        mock_post.reset_mock()
        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
                "DANBOORU_SEARCH_INCLUDE_NSFW": "true",
            },
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called()
        call_args, call_kwargs = mock_post.call_args
        req_data = call_kwargs.get("json")
        self.assertTrue(req_data["show_nsfw"])

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

        mock_post.reset_mock()
        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": ""}
        ):
            suggestions = list(provider.provide(context))
        self.assertEqual(len(suggestions), 0)

        context2 = self._make_context(
            target_command="add",
            query="pos",
            prev_word="--region",
            cwords=["/add", "--region"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        self.assertFalse(provider.can_provide(context2))

    def test_autocomplete_add_prompt_danbooru_related(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "sailor_collar",
                    "cn_name": "水手领",
                    "wiki": "A collar style.",
                },
                {
                    "tag": "hatsune_miku",
                    "category": "Character",
                    "cn_name": "初音未来",
                    "wiki": "Vocaloid character.",
                },
                {"tag": "skirt", "cn_name": "裙子", "wiki": "A bottom wear."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        context = self._make_context(
            target_command="add",
            query="",
            prev_word="1girl,",
            cwords=["/add", "masterpiece,", "1girl,"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

        req_data = call_kwargs.get("json")
        self.assertEqual(req_data["tags"], ["masterpiece", "1girl"])
        self.assertEqual(req_data["limit"], 100)
        self.assertFalse(req_data["show_nsfw"])

        mock_post.reset_mock()
        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
                "DANBOORU_SEARCH_INCLUDE_NSFW": "true",
            },
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        req_data = call_kwargs.get("json")
        self.assertTrue(req_data["show_nsfw"])

        self.assertEqual(len(suggestions), 2)
        self.assertEqual(suggestions[0].text, "sailor_collar")
        self.assertEqual(suggestions[0].displayText, "sailor_collar (水手领)")
        self.assertEqual(suggestions[0].description, "A collar style.")
        self.assertEqual(suggestions[0].type, "danbooru")

        self.assertEqual(suggestions[1].text, "skirt")
        self.assertEqual(suggestions[1].displayText, "skirt (裙子)")
        self.assertEqual(suggestions[1].description, "A bottom wear.")

    def test_autocomplete_add_prompt_danbooru_related_fallback(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "solo", "cn_name": "单人", "wiki": "Solo."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

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
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

        req_data = call_kwargs.get("json")
        self.assertEqual(sorted(req_data["tags"]), ["1girl", "masterpiece"])

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "solo")

    def test_autocomplete_danbooru_style_muted_from_history(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
                {"tag": "solo", "cn_name": "单人", "wiki": "Solo image."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        with patch(
            "comfyui.autocomplete.get_added_prompts", return_value={"1girl"}
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "mock_root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "mock_rel",
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
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
            provider = DanbooruProvider()

            with patch("comfyui.autocomplete.requests.post", mock_post):
                suggestions1 = list(provider.provide(context1))

            self.assertEqual(len(suggestions1), 2)
            # 1girl 在历史中，style 为 muted
            self.assertEqual(suggestions1[0].text, "1girl")
            self.assertEqual(suggestions1[0].style, "muted")
            # solo 不在历史中，style 保持为空
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

            mock_response_related = MagicMock()
            mock_response_related.json.return_value = {
                "results": [
                    {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
                    {"tag": "solo", "cn_name": "单人", "wiki": "Solo image."},
                ]
            }
            mock_post_related = MagicMock(return_value=mock_response_related)

            with patch("comfyui.autocomplete.requests.post", mock_post_related):
                suggestions2 = list(provider.provide(context2))

            self.assertEqual(len(suggestions2), 2)
            self.assertEqual(suggestions2[0].text, "1girl")
            self.assertEqual(suggestions2[0].style, "muted")
            self.assertEqual(suggestions2[1].text, "solo")
            self.assertEqual(suggestions2[1].style, "")

    def test_autocomplete_add_prompt_with_neg_flag(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="--neg",
            cwords=["/add", "--neg"],
            parsed_args=self._make_parsed_args(command="add"),
        )
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        texts = [s.text for s in suggestions]
        self.assertEqual(len(suggestions), 1)
        self.assertEqual(texts[0], "1girl")

    def test_autocomplete_add_prompt_region_query_search(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/search")

        req_data = call_kwargs.get("json")
        self.assertEqual(req_data["query"], "girl")

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")
        self.assertEqual(suggestions[0].type, "danbooru")

    def test_autocomplete_add_prompt_region_related(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "cute", "cn_name": "可爱", "wiki": "Cute character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

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
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

        req_data = call_kwargs.get("json")
        expected_tags = ["1girl", "cute", "masterpiece", "score_7"]
        self.assertEqual(sorted(req_data["tags"]), expected_tags)

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "cute")

    def test_autocomplete_add_prompt_region_query_search_fallback(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        context = self._make_context(
            target_command="add",
            query="girl",
            prev_word="positive",
            cwords=["/add", "--region", "positive"],
            parsed_args=self._make_parsed_args(command="add", region=["positive"]),
        )
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/search")

        req_data = call_kwargs.get("json")
        self.assertEqual(req_data["query"], "girl")

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].text, "1girl")
        self.assertEqual(suggestions[0].type, "danbooru")

    def test_autocomplete_add_prompt_region_related_fallback(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "cute", "cn_name": "可爱", "wiki": "Cute character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

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
        provider = DanbooruProvider()
        self.assertTrue(provider.can_provide(context))

        with patch("comfyui.autocomplete.requests.post", mock_post), patch.dict(
            os.environ, {"DANBOORU_SEARCH_URL": "https://mock-danbooru.space"}
        ):
            suggestions = list(provider.provide(context))

        mock_post.assert_called_once()
        call_args, call_kwargs = mock_post.call_args
        self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

        req_data = call_kwargs.get("json")
        expected_tags = [
            "1girl",
            "another prompt line",
            "cute",
            "masterpiece",
            "score_7",
        ]
        self.assertEqual(sorted(req_data["tags"]), expected_tags)

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
                    "widgets_values": ["1girl"],
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
        self.assertEqual(node_1_sug.description, "1girl")
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

    @patch("comfyui.autocomplete.requests.post")
    def test_integration_autocomplete_danbooru_related(
        self, mock_post: MagicMock
    ) -> None:
        import io
        import json
        from comfyui.autocomplete import main as autocomplete_main

        # 1. Mock Danbooru API 的返回，包含 1 个 Character 标签，以及超过 25 个其他标签
        mock_response = MagicMock()
        results = [
            {"tag": "1girl", "cn_name": "女孩", "wiki": "A girl."},
            {
                "tag": "hatsune_miku",
                "category": "Character",
                "cn_name": "初音未来",
                "wiki": "A character.",
            },
        ]
        for i in range(30):
            results.append(
                {"tag": f"tag_{i}", "cn_name": f"标签_{i}", "wiki": f"Wiki_{i}"}
            )

        mock_response.json.return_value = {"results": results}
        mock_post.return_value = mock_response

        # 2. 配置补全上下文环境变量
        env_vars = {
            "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
            "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "1girl,",
            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/add", "masterpiece,", "1girl,"]',
            "IMAGE_FUNNEL_IMAGE_PATHS": "[]",
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

        # 验证 limit 为 100
        mock_post.assert_called_once()
        _, call_kwargs = mock_post.call_args
        self.assertEqual(call_kwargs.get("json", {}).get("limit"), 100)

        # 验证结果中不包含 character 标签
        tags = [item["text"] for item in lines]
        self.assertNotIn("hatsune_miku", tags)
        self.assertIn("1girl", tags)
        self.assertIn("tag_0", tags)

        # 验证结果被限制在最前 20 个常规标签内（1girl + tag_0 到 tag_18）
        self.assertEqual(len(lines), 20)
        self.assertEqual(lines[0]["text"], "1girl")
        self.assertEqual(lines[19]["text"], "tag_18")


if __name__ == "__main__":
    unittest.main()
