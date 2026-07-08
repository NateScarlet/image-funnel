# /// script
# dependencies = [
#   "requests",
#   "Pillow",
# ]
# ///

#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import unittest
import json
from unittest.mock import patch, MagicMock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))


class TestComfyUIAutocomplete(unittest.TestCase):

    def setUp(self):
        # 每次测试执行前清空模块缓存，确保 PIL.Image.open 的 mock 绝对生效
        sys.modules.pop("comfyui_autocomplete", None)
        sys.modules.pop("comfyui", None)

    def _get_mock_image(self):
        mock_prompt = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {
                    "text": "1girl, masterpiece, score_7, cute\n// this is a comment\n\n  another prompt line , "
                },
            }
        }
        mock_workflow = {
            "nodes": [
                {
                    "id": "node_1",
                    "type": "CLIPTextEncode",
                    "widgets_values": [
                        "1girl, masterpiece, score_7, cute\n// this is a comment\n\n  another prompt line , "
                    ],
                }
            ]
        }
        mock_image = MagicMock()
        mock_image.info = {
            "prompt": json.dumps(mock_prompt),
            "workflow": json.dumps(mock_workflow),
        }
        mock_image.__enter__.return_value = mock_image
        return mock_image

    def test_autocomplete_remove_prompt_normal(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/remove"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            texts = [s.text for s in suggestions]

            self.assertEqual(len(suggestions), 2)
            self.assertIn('"1girl, masterpiece, score_7, cute"', texts)
            self.assertIn('"another prompt line"', texts)
            self.assertEqual(suggestions[0].type, "prompt")
            self.assertEqual(suggestions[0].style, "")
            self.assertTrue("positive" in suggestions[0].description)

    def test_autocomplete_remove_prompt_by_region(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "cute",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove --region positive cu",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(
                    ["/remove", "--region", "positive"]
                ),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 1)
            self.assertEqual(texts[0], '"1girl, masterpiece, score_7, cute"')

    def test_autocomplete_remove_prompt_skip_on_option(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "--region",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove --region ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/remove", "--region"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            for s in suggestions:
                self.assertNotEqual(s.type, "prompt")

    def test_autocomplete_remove_prompt_skip_on_dash_query(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "--r",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove --r",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/remove"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            for s in suggestions:
                self.assertNotEqual(s.type, "prompt")

    def test_autocomplete_remove_prompt_by_custom_command(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/delete_prompt ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/delete_prompt"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 2)
            self.assertIn('"1girl, masterpiece, score_7, cute"', texts)

    def test_autocomplete_remove_prompt_with_double_dash(self):
        mock_prompt_dash = {
            "node_1": {
                "class_type": "CLIPTextEncode",
                "inputs": {"text": "-a_cute_girl, masterpiece\n"},
            }
        }
        mock_workflow_dash = {
            "nodes": [
                {
                    "id": "node_1",
                    "type": "CLIPTextEncode",
                    "widgets_values": ["-a_cute_girl\nmasterpiece\n"],
                }
            ]
        }
        mock_image_dash = MagicMock()
        mock_image_dash.info = {
            "prompt": json.dumps(mock_prompt_dash),
            "workflow": json.dumps(mock_workflow_dash),
        }
        mock_image_dash.__enter__.return_value = mock_image_dash

        with patch("PIL.Image.open", return_value=mock_image_dash), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "-a",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "--",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove -- -a",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/remove", "--"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 1)
            self.assertEqual(texts[0], "-a_cute_girl")

    def test_autocomplete_adjust_prompt_normal(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/adjust prompt ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/adjust", "prompt"]),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("adjust"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 2)
            self.assertIn('"1girl, masterpiece, score_7, cute"', texts)
            self.assertIn('"another prompt line"', texts)

    def test_autocomplete_adjust_prompt_skip_by_text(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": '"another prompt line"',
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": '/adjust prompt "another prompt line" ',
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(
                    ["/adjust", "prompt", '"another prompt line"']
                ),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("adjust"))
            self.assertEqual(len(suggestions), 0)

    def test_autocomplete_adjust_prompt_skip_by_weight(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "1",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": '"another prompt line"',
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": '/adjust prompt "another prompt line" 1',
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(
                    ["/adjust", "prompt", '"another prompt line"', "1"]
                ),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("adjust"))
            self.assertEqual(len(suggestions), 0)

    def test_autocomplete_remove_prompt_with_double_dash_and_option(self):
        mock_image = self._get_mock_image()
        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "a",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "--region",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/remove -- --region a",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(
                    ["/remove", "--", "--region"]
                ),
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("remove"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 2)
            self.assertIn('"1girl, masterpiece, score_7, cute"', texts)

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

        with patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/add"]),
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))

            mock_post.assert_called_once()
            call_args, call_kwargs = mock_post.call_args
            self.assertEqual(call_args[0], "https://mock-danbooru.space/api/search")

            req_data = call_kwargs.get("json")
            self.assertEqual(req_data["query"], "girl")
            self.assertEqual(req_data["top_k"], 20)
            self.assertEqual(req_data["limit"], 20)

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
        with patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/add"]),
                "DANBOORU_SEARCH_URL": "",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))
            mock_post.assert_not_called()
            self.assertEqual(len(suggestions), 0)

        mock_post.reset_mock()
        with patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "pos",
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "--region",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add --region pos",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/add", "--region"]),
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))
            mock_post.assert_not_called()

    def test_autocomplete_add_prompt_danbooru_related(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "sailor_collar",
                    "cn_name": "水手领",
                    "wiki": "A collar style.",
                },
                {"tag": "skirt", "cn_name": "裙子", "wiki": "A bottom wear."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        with patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "1girl,",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add masterpiece, 1girl, ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(
                    ["/add", "masterpiece,", "1girl,"]
                ),
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))

            mock_post.assert_called_once()
            call_args, call_kwargs = mock_post.call_args
            self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

            req_data = call_kwargs.get("json")
            self.assertEqual(req_data["tags"], ["masterpiece", "1girl"])
            self.assertEqual(req_data["limit"], 20)

            self.assertEqual(len(suggestions), 2)
            self.assertEqual(suggestions[0].text, "sailor_collar")
            self.assertEqual(suggestions[0].displayText, "sailor_collar (水手领)")
            self.assertEqual(suggestions[0].description, "A collar style.")
            self.assertEqual(suggestions[0].type, "danbooru")

            self.assertEqual(suggestions[1].text, "skirt")
            self.assertEqual(suggestions[1].displayText, "skirt (裙子)")
            self.assertEqual(suggestions[1].description, "A bottom wear.")

    def test_autocomplete_add_prompt_danbooru_related_fallback(self):
        mock_prompt = {
            "6": {
                "inputs": {"text": "masterpiece, 1girl"},
                "class_type": "CLIPTextEncode",
            }
        }
        mock_workflow = {
            "nodes": [
                {
                    "id": 6,
                    "type": "CLIPTextEncode",
                    "widgets_values": ["masterpiece, 1girl"],
                }
            ]
        }

        mock_image = MagicMock()
        mock_image.info = {
            "prompt": json.dumps(mock_prompt),
            "workflow": json.dumps(mock_workflow),
        }
        mock_image.__enter__.return_value = mock_image

        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "solo", "cn_name": "单人", "wiki": "Solo."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        with patch("PIL.Image.open", return_value=mock_image), patch(
            "os.path.isfile", return_value=True
        ), patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(["dummy.png"]),
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add ",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/add"]),
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))

            mock_post.assert_called_once()
            call_args, call_kwargs = mock_post.call_args
            self.assertEqual(call_args[0], "https://mock-danbooru.space/api/related")

            req_data = call_kwargs.get("json")
            self.assertEqual(sorted(req_data["tags"]), ["1girl", "masterpiece"])

            self.assertEqual(len(suggestions), 1)
            self.assertEqual(suggestions[0].text, "solo")

    def test_autocomplete_add_prompt_with_neg_flag(self):
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "1girl", "cn_name": "女孩", "wiki": "A female character."},
            ]
        }
        mock_post = MagicMock(return_value=mock_response)

        with patch("comfyui_autocomplete.requests.post", mock_post), patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "--neg",
                "IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX": "/add --neg girl",
                "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": json.dumps(["/add", "--neg"]),
                "DANBOORU_SEARCH_URL": "https://mock-danbooru.space",
            },
        ):
            from comfyui_autocomplete import autocomplete

            suggestions = list(autocomplete("add"))
            texts = [s.text for s in suggestions]
            self.assertEqual(len(suggestions), 1)
            self.assertEqual(texts[0], "1girl")
            mock_post.assert_called_once()


if __name__ == "__main__":
    unittest.main()
