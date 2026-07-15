# -*- coding: utf-8 -*-
import json
import os
import shutil
import tempfile
import unittest
from unittest.mock import MagicMock, patch
import requests

from .db import SQLiteContext
from .danbooru import (
    AkizukiDanbooruTagProvider,
    DanbooruTag,
    SQLiteDanbooruTagProvider,
)


class TestAkizukiDanbooruTagProvider(unittest.TestCase):

    @patch("comfyui.danbooru.requests.post")
    def test_search_success(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "1girl",
                    "cn_name": "女孩",
                    "wiki": "A girl.",
                    "category": "General",
                },
                {
                    "tag": "solo",
                    "cn_name": "单人",
                    "wiki": "Solo.",
                    "category": "General",
                },
            ]
        }
        mock_post.return_value = mock_response

        provider = AkizukiDanbooruTagProvider("https://mock-api.com")
        results = provider.search("girl")

        self.assertEqual(len(results), 2)
        self.assertEqual(results[0].tag, "1girl")
        self.assertEqual(results[0].cn_name, "女孩")
        self.assertEqual(results[0].category, "General")
        mock_post.assert_called_once()

    @patch("comfyui.danbooru.requests.post")
    def test_search_failure(self, mock_post: MagicMock) -> None:
        mock_post.side_effect = requests.RequestException("Network Error")

        provider = AkizukiDanbooruTagProvider("https://mock-api.com")
        with self.assertRaises(requests.RequestException):
            provider.search("girl")

    @patch("comfyui.danbooru.requests.post")
    def test_related_success(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "masterpiece",
                    "cn_name": "杰作",
                    "wiki": "Masterpiece.",
                    "category": "General",
                },
            ]
        }
        mock_post.return_value = mock_response

        provider = AkizukiDanbooruTagProvider("https://mock-api.com")
        results = provider.related(["1girl", "solo"])

        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].tag, "masterpiece")
        mock_post.assert_called_once()

    @patch("comfyui.danbooru.requests.post")
    def test_search_payload_and_nsfw(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"results": []}
        mock_post.return_value = mock_response

        # 默认情况下 NSFW 为 false
        provider = AkizukiDanbooruTagProvider("https://mock-api.com")
        provider.search("girl")
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertEqual(payload["query"], "girl")
        self.assertEqual(payload["limit"], 20)
        self.assertFalse(payload["show_nsfw"])

        # show_nsfw=True 时，应该反映在 payload 中
        mock_post.reset_mock()
        provider_nsfw = AkizukiDanbooruTagProvider(
            "https://mock-api.com", show_nsfw=True
        )
        provider_nsfw.search("girl")
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertTrue(payload["show_nsfw"])

    @patch("comfyui.danbooru.requests.post")
    def test_related_payload_and_nsfw(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"results": []}
        mock_post.return_value = mock_response

        provider = AkizukiDanbooruTagProvider("https://mock-api.com")
        provider.related(["1girl"])
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertEqual(payload["tags"], ["1girl"])
        self.assertEqual(payload["limit"], 100)
        self.assertFalse(payload["show_nsfw"])

        mock_post.reset_mock()
        provider_nsfw = AkizukiDanbooruTagProvider(
            "https://mock-api.com", show_nsfw=True
        )
        provider_nsfw.related(["1girl"])
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertTrue(payload["show_nsfw"])

    def test_from_env(self) -> None:
        with patch.dict(os.environ, {"DANBOORU_SEARCH_INCLUDE_NSFW": "true"}):
            provider = AkizukiDanbooruTagProvider.from_env("https://mock-api.com")
            self.assertTrue(provider.show_nsfw)

        with patch.dict(os.environ, {"DANBOORU_SEARCH_INCLUDE_NSFW": "false"}):
            provider = AkizukiDanbooruTagProvider.from_env("https://mock-api.com")
            self.assertFalse(provider.show_nsfw)


class TestSQLiteDanbooruTagProvider(unittest.TestCase):

    def setUp(self) -> None:
        self.tmp_dir = tempfile.mkdtemp()
        self.db_path = os.path.join(self.tmp_dir, "test.db")
        self.db_ctx = SQLiteContext(self.db_path)
        self.mock_inner = MagicMock()
        self.provider = SQLiteDanbooruTagProvider(
            self.mock_inner, self.db_ctx, "https://mock-api.com", ttl=100
        )

    def tearDown(self) -> None:
        self.db_ctx.close()
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def test_search_no_cache_sync_fetch(self) -> None:
        tags = [
            DanbooruTag("1girl", "女孩", "wiki", "General"),
        ]
        self.mock_inner.search.return_value = tags

        # 首次查询，无缓存，应该同步拉取
        results = self.provider.search("girl")
        self.assertEqual(results, tags)
        self.mock_inner.search.assert_called_once_with("girl")

        # 验证是否写入了缓存
        row = self.db_ctx.connection.execute(
            "SELECT results FROM danbooru_search_cache WHERE query = ?",
            ("girl",),
        ).fetchone()
        self.assertIsNotNone(row)
        data = json.loads(row[0])
        self.assertEqual(data[0]["tag"], "1girl")

    def test_search_fresh_cache_returns_directly(self) -> None:
        import time

        now = int(time.time())
        data_str = json.dumps(
            [{"tag": "solo", "cn_name": "单人", "wiki": "wiki", "category": "General"}]
        )
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                ("solo", data_str, now - 10),
            )

        results = self.provider.search("solo")
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].tag, "solo")
        self.mock_inner.search.assert_not_called()

    @patch("comfyui.danbooru.SQLiteDanbooruTagProvider._trigger_async_update")
    def test_search_stale_cache_returns_and_triggers_swr(
        self, mock_trigger: MagicMock
    ) -> None:
        import time

        now = int(time.time())
        data_str = json.dumps(
            [{"tag": "solo", "cn_name": "单人", "wiki": "wiki", "category": "General"}]
        )
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                ("solo", data_str, now - 200),
            )

        results = self.provider.search("solo")
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].tag, "solo")
        self.mock_inner.search.assert_not_called()
        mock_trigger.assert_called_once_with("search", "solo")

    def test_related_preserves_order(self) -> None:
        tags1 = ["1girl", "solo"]
        tags2 = ["solo", "1girl"]

        mock_res1 = [DanbooruTag("tag1", "cn1", "w1", "c1")]
        mock_res2 = [DanbooruTag("tag2", "cn2", "w2", "c2")]

        self.mock_inner.related.side_effect = [mock_res1, mock_res2]

        res1 = self.provider.related(tags1)
        res2 = self.provider.related(tags2)

        self.assertEqual(res1, mock_res1)
        self.assertEqual(res2, mock_res2)

        rows = self.db_ctx.connection.execute(
            "SELECT tags, results FROM danbooru_related_cache"
        ).fetchall()
        self.assertEqual(len(rows), 2)
        keys = [row[0] for row in rows]
        self.assertIn(json.dumps(tags1), keys)
        self.assertIn(json.dumps(tags2), keys)

    def test_update_cache_search(self) -> None:
        from comfyui.danbooru import update_cache

        with patch(
            "comfyui.danbooru.AkizukiDanbooruTagProvider.from_env"
        ) as mock_from_env:
            mock_provider = MagicMock()
            mock_provider.search.return_value = [
                DanbooruTag("1girl", "女孩", "A girl.", "General")
            ]
            mock_from_env.return_value = mock_provider

            update_cache("search", "girl", "https://mock-api.com", db_ctx=self.db_ctx)

            row = self.db_ctx.connection.execute(
                "SELECT results FROM danbooru_search_cache WHERE query = ?",
                ("girl",),
            ).fetchone()
            self.assertIsNotNone(row)
            data = json.loads(row[0])
            self.assertEqual(data[0]["tag"], "1girl")

    def test_update_cache_related(self) -> None:
        from comfyui.danbooru import update_cache

        with patch(
            "comfyui.danbooru.AkizukiDanbooruTagProvider.from_env"
        ) as mock_from_env:
            mock_provider = MagicMock()
            mock_provider.related.return_value = [
                DanbooruTag("solo", "单人", "Solo.", "General")
            ]
            mock_from_env.return_value = mock_provider

            tags_arg = json.dumps(["1girl"])
            update_cache(
                "related", tags_arg, "https://mock-api.com", db_ctx=self.db_ctx
            )

            row = self.db_ctx.connection.execute(
                "SELECT results FROM danbooru_related_cache WHERE tags = ?",
                (tags_arg,),
            ).fetchone()
            self.assertIsNotNone(row)
            data = json.loads(row[0])
            self.assertEqual(data[0]["tag"], "solo")

    @patch("sys.argv", ["comfyui.danbooru", "search", "girl", "https://mock-api.com"])
    @patch("comfyui.danbooru.update_cache")
    def test_danbooru_main_success(self, mock_update: MagicMock) -> None:
        from comfyui.danbooru import main as danbooru_main

        try:
            danbooru_main()
        except SystemExit as e:
            self.assertEqual(e.code, 0)
        mock_update.assert_called_once_with("search", "girl", "https://mock-api.com")
