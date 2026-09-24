# -*- coding: utf-8 -*-
import json
import os
import shutil
import sqlite3
import tempfile
import unittest
from typing import Any, cast
from unittest.mock import MagicMock, patch, PropertyMock
import requests
import time

from .db import SQLiteContext
from .danbooru import (
    AkizukiDanbooruTagProvider,
    DanbooruTag,
    FileDanbooruTagProvider,
    SQLiteDanbooruTagProvider,
    SQLiteDanbooruTagLoader,
    AkizukiDanbooruTagLoader,
)
from .danbooru_data import (
    COMPILED_COOC_FILENAME,
    COMPILED_TAGS_FILENAME,
    compile_dataset,
)


def write_file_fixture(
    data_dir: str,
    tags: list[dict[str, str]],
    cooc: list[tuple[str, str, int]],
    *,
    csv_encoding: str = "utf-8",
) -> None:
    """写入源数据并编译为 FileDanbooruTagProvider 所需产物的夹具。"""
    import csv as csv_mod
    import pyarrow as pa  # pyright: ignore[reportMissingTypeStubs]
    import pyarrow.parquet as pq  # pyright: ignore[reportMissingTypeStubs]

    pa_mod = cast(Any, pa)
    pq_mod = cast(Any, pq)

    fieldnames = ["name", "cn_name", "wiki", "post_count", "category", "nsfw"]
    csv_path = os.path.join(data_dir, "tags_enhanced.csv")
    with open(csv_path, "w", encoding=csv_encoding, newline="") as f:
        writer = csv_mod.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(tags)

    table = pa_mod.table(
        {
            "tag_a": pa_mod.array([a for a, _, _ in cooc], type=pa_mod.string()),
            "tag_b": pa_mod.array([b for _, b, _ in cooc], type=pa_mod.string()),
            "count": pa_mod.array([c for _, _, c in cooc], type=pa_mod.int32()),
        }
    )
    pq_mod.write_table(table, os.path.join(data_dir, "cooccurrence_clean.parquet"))
    # 测试夹具：源与产物同目录，provider 直接从该目录读取
    compile_dataset(data_dir, data_dir)


class TestFileDanbooruTagProvider(unittest.TestCase):
    """本地数据文件 + 容忍笔误字面匹配的离线补全实现。"""

    def setUp(self) -> None:
        self.tmp_dir = tempfile.mkdtemp()
        self.tags = [
            {
                "name": "1girl",
                "cn_name": "一个女孩,女孩",
                "wiki": "A girl.",
                "post_count": "8101306",
                "category": "0",
                "nsfw": "0",
            },
            {
                "name": "solo",
                "cn_name": "单人",
                "wiki": "Solo.",
                "post_count": "3000000",
                "category": "0",
                "nsfw": "0",
            },
            {
                "name": "white_hair",
                "cn_name": "白发",
                "wiki": "White hair.",
                "post_count": "500000",
                "category": "0",
                "nsfw": "0",
            },
            {
                "name": "red_hair",
                "cn_name": "红发",
                "wiki": "Red hair.",
                "post_count": "400000",
                "category": "0",
                "nsfw": "0",
            },
            {
                "name": "hentai",
                "cn_name": "变态",
                "wiki": "NSFW tag.",
                "post_count": "900000",
                "category": "0",
                "nsfw": "1",
            },
            {
                "name": "hatsune_miku",
                "cn_name": "初音未来",
                "wiki": "Miku.",
                "post_count": "200000",
                "category": "4",
                "nsfw": "0",
            },
            {
                "name": "some_artist",
                "cn_name": "某画师",
                "wiki": "Artist.",
                "post_count": "5000",
                "category": "1",
                "nsfw": "0",
            },
        ]
        self.cooc = [
            ("1girl", "white_hair", 100),
            ("white_hair", "1girl", 100),
            ("1girl", "solo", 50),
            ("solo", "1girl", 50),
            ("1girl", "hentai", 80),
            ("1girl", "hatsune_miku", 10),
            ("1girl", "some_artist", 5),
        ]
        write_file_fixture(self.tmp_dir, self.tags, self.cooc)

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    # ---- 缺文件快速失败 ----

    def test_missing_dir_raises_with_compile_hint(self) -> None:
        missing = os.path.join(self.tmp_dir, "no-such-dir")
        with self.assertRaises(FileNotFoundError) as ctx:
            FileDanbooruTagProvider(missing)
        msg = str(ctx.exception)
        self.assertIn(COMPILED_TAGS_FILENAME, msg)
        self.assertIn(COMPILED_COOC_FILENAME, msg)
        self.assertIn("compile_danbooru_data.py", msg)
        # 下载指引只在编译脚本路径，不在运行时
        self.assertNotIn("github.com", msg)

    def test_missing_tags_bin_raises_with_compile_hint(self) -> None:
        os.remove(os.path.join(self.tmp_dir, COMPILED_TAGS_FILENAME))
        with self.assertRaises(FileNotFoundError) as ctx:
            FileDanbooruTagProvider(self.tmp_dir)
        msg = str(ctx.exception)
        self.assertIn(COMPILED_TAGS_FILENAME, msg)
        self.assertIn("compile_danbooru_data.py", msg)

    def test_missing_cooc_bin_raises_with_compile_hint(self) -> None:
        os.remove(os.path.join(self.tmp_dir, COMPILED_COOC_FILENAME))
        with self.assertRaises(FileNotFoundError) as ctx:
            FileDanbooruTagProvider(self.tmp_dir)
        msg = str(ctx.exception)
        self.assertIn(COMPILED_COOC_FILENAME, msg)
        self.assertIn("compile_danbooru_data.py", msg)

    # ---- 懒加载：构造不读内容，search/related 分别触发 ----

    def test_construct_does_not_load_content(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        dataset = cast(Any, provider)._dataset
        self.assertIsNone(dataset._tag_index)
        self.assertIsNone(dataset._cooc_index)

    def test_search_lazy_loads_tags_only(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        provider.search("solo")
        dataset = cast(Any, provider)._dataset
        self.assertIsNotNone(dataset._tag_index)
        self.assertIsNone(dataset._cooc_index)

    def test_related_lazy_loads_tags_and_cooc(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        provider.related(["1girl"])
        dataset = cast(Any, provider)._dataset
        self.assertIsNotNone(dataset._tag_index)
        self.assertIsNotNone(dataset._cooc_index)

    # ---- search：分层字面匹配 + 笔误容忍 ----

    def test_search_exact_match_ranks_first(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("solo")
        self.assertGreaterEqual(len(results), 1)
        self.assertEqual(results[0].tag, "solo")
        self.assertEqual(results[0].cn_name, "单人")
        self.assertEqual(results[0].category, "General")

    def test_search_prefix_match(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("white")
        tags = [r.tag for r in results]
        self.assertIn("white_hair", tags)
        self.assertNotIn("red_hair", tags)

    def test_search_substring_match_underscore_or_space(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        by_space = {r.tag for r in provider.search("white hair")}
        by_underscore = {r.tag for r in provider.search("white_hair")}
        self.assertIn("white_hair", by_space)
        self.assertIn("white_hair", by_underscore)

    def test_search_tolerates_typo(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("solo")  # 精确
        self.assertEqual(results[0].tag, "solo")
        # 一处笔误仍应命中
        typo_results = provider.search("solo ")
        self.assertTrue(any(r.tag == "solo" for r in typo_results))
        fuzzy = provider.search("whit hair")  # 缺 e + 空格
        self.assertTrue(any(r.tag == "white_hair" for r in fuzzy))

    def test_search_chinese_via_cn_name(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("白发")
        self.assertGreaterEqual(len(results), 1)
        self.assertEqual(results[0].tag, "white_hair")

    def test_search_chinese_second_alias(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("女孩")
        tags = [r.tag for r in results]
        self.assertIn("1girl", tags)

    def test_search_filters_nsfw_by_default(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=False)
        tags = [r.tag for r in provider.search("hentai")]
        self.assertNotIn("hentai", tags)

    def test_search_includes_nsfw_when_enabled(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        tags = [r.tag for r in provider.search("hentai")]
        self.assertIn("hentai", tags)

    def test_search_empty_query_returns_empty(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        self.assertEqual(provider.search(""), [])
        self.assertEqual(provider.search("   "), [])

    def test_search_maps_numeric_category(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("hatsune_miku")
        self.assertEqual(results[0].tag, "hatsune_miku")
        self.assertEqual(results[0].category, "Character")
        artist = provider.search("some_artist")
        self.assertEqual(artist[0].category, "Artist")

    def test_search_rank_popularity_tiebreak(self) -> None:
        """同层匹配按 post_count 降序：1girl 应排在同为 General 的低热度前缀前。"""
        provider = FileDanbooruTagProvider(self.tmp_dir)
        results = provider.search("g")
        # 前缀含 g 的：1girl / solo? 无；至少 1girl 命中
        tags = [r.tag for r in results]
        self.assertIn("1girl", tags)

    def test_search_gbk_encoded_csv(self) -> None:
        """CSV 编码回退：GBK 文件也能加载。"""
        gbk_dir = tempfile.mkdtemp()
        try:
            write_file_fixture(
                gbk_dir,
                [
                    {
                        "name": "1girl",
                        "cn_name": "一个女孩",
                        "wiki": "A girl.",
                        "post_count": "100",
                        "category": "0",
                        "nsfw": "0",
                    }
                ],
                [],
                csv_encoding="gbk",
            )
            provider = FileDanbooruTagProvider(gbk_dir)
            results = provider.search("1girl")
            self.assertEqual(results[0].cn_name, "一个女孩")
        finally:
            shutil.rmtree(gbk_dir, ignore_errors=True)

    # ---- related：共现计数联想 ----

    def test_related_aggregates_cooccurrence(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        results = provider.related(["1girl"])
        tags = [r.tag for r in results]
        self.assertIn("white_hair", tags)
        self.assertIn("solo", tags)
        # white_hair 计数 100 > solo 50
        self.assertLess(tags.index("white_hair"), tags.index("solo"))

    def test_related_excludes_seed_tags(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        results = provider.related(["1girl", "white_hair"])
        tags = {r.tag for r in results}
        self.assertNotIn("1girl", tags)
        self.assertNotIn("white_hair", tags)

    def test_related_filters_target_categories(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        results = provider.related(
            ["1girl"], target_categories=["General", "Artist", "Meta"]
        )
        tags = {r.tag for r in results}
        self.assertIn("white_hair", tags)  # General
        self.assertIn("some_artist", tags)  # Artist
        self.assertNotIn("hatsune_miku", tags)  # Character 被过滤

    def test_related_filters_nsfw(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=False)
        results = provider.related(["1girl"])
        tags = {r.tag for r in results}
        self.assertNotIn("hentai", tags)

        provider_nsfw = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        results_nsfw = provider_nsfw.related(["1girl"])
        self.assertIn("hentai", {r.tag for r in results_nsfw})

    def test_related_empty_tags_returns_empty(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        self.assertEqual(provider.related([]), [])

    def test_related_unknown_seed_returns_empty(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir)
        self.assertEqual(provider.related(["no_such_tag_xyz"]), [])

    def test_related_returns_danbooru_tag_fields(self) -> None:
        provider = FileDanbooruTagProvider(self.tmp_dir, show_nsfw=True)
        results = provider.related(["1girl"])
        white = next(r for r in results if r.tag == "white_hair")
        self.assertEqual(white.cn_name, "白发")
        self.assertEqual(white.wiki, "White hair.")
        self.assertEqual(white.category, "General")


class TestAkizukiDanbooruTagProvider(unittest.TestCase):

    def setUp(self) -> None:
        self.loader = MagicMock()
        self.loader.load.return_value = None

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

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
        results = provider.search("girl")

        self.assertEqual(len(results), 2)
        self.assertEqual(results[0].tag, "1girl")
        self.assertEqual(results[0].cn_name, "女孩")
        self.assertEqual(results[0].category, "General")
        mock_post.assert_called_once()

    @patch("comfyui.danbooru.requests.post")
    def test_search_failure(self, mock_post: MagicMock) -> None:
        mock_post.side_effect = requests.RequestException("Network Error")

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
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

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
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
        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
        provider.search("girl")
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertEqual(payload["query"], "girl")
        self.assertEqual(payload["limit"], 20)
        self.assertFalse(payload["show_nsfw"])

        # show_nsfw=True 时，应该反映在 payload 中
        mock_post.reset_mock()
        provider_nsfw = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader, show_nsfw=True
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

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
        provider.related(["1girl"], target_categories=["General", "Artist"])
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertEqual(payload["tags"], ["1girl"])
        self.assertEqual(payload["limit"], 100)
        self.assertFalse(payload["show_nsfw"])
        self.assertEqual(payload["target_categories"], ["General", "Artist"])

        mock_post.reset_mock()
        provider_nsfw = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader, show_nsfw=True
        )
        provider_nsfw.related(["1girl"])
        _, call_kwargs = mock_post.call_args
        payload = call_kwargs.get("json", {})
        self.assertTrue(payload["show_nsfw"])
        self.assertNotIn("target_categories", payload)

    def test_from_env(self) -> None:
        with patch.dict(os.environ, {"DANBOORU_SEARCH_INCLUDE_NSFW": "true"}):
            provider = AkizukiDanbooruTagProvider.from_env(
                "https://mock-api.com", loader=self.loader
            )
            self.assertTrue(provider.show_nsfw)

        with patch.dict(os.environ, {"DANBOORU_SEARCH_INCLUDE_NSFW": "false"}):
            provider = AkizukiDanbooruTagProvider.from_env(
                "https://mock-api.com", loader=self.loader
            )
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

    def test_search_cache_read_error_falls_back_to_upstream(self) -> None:
        """缓存读取失败（sqlite3.Error）回退上游（SWR 语义），写缓存不受影响。"""
        tags = [
            DanbooruTag("1girl", "女孩", "wiki", "General"),
        ]
        self.mock_inner.search.return_value = tags
        real_conn = self.db_ctx.connection

        class _FlakyReadConn:
            """仅对 search 缓存读取失败，其余语句转发真实连接。"""

            def execute(self, sql: str, *args: Any, **kwargs: Any) -> Any:
                if sql.startswith("SELECT") and "danbooru_search_cache" in sql:
                    raise sqlite3.OperationalError("database is locked")
                return real_conn.execute(sql, *args, **kwargs)

            def commit(self) -> Any:
                return real_conn.commit()

            def rollback(self) -> Any:
                return real_conn.rollback()

        with patch.object(
            SQLiteContext,
            "connection",
            new_callable=PropertyMock,
            return_value=_FlakyReadConn(),
        ):
            results = self.provider.search("girl")

        self.assertEqual(results, tags)
        self.mock_inner.search.assert_called_once_with("girl")

    def test_search_cache_corrupt_json_falls_back_to_upstream(self) -> None:
        """缓存数据损坏（JSONDecodeError）回退上游（SWR 语义）。"""
        now = int(time.time())
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                ("solo", "not-json{{{", now),
            )
        tags = [
            DanbooruTag("solo", "单人", "wiki", "General"),
        ]
        self.mock_inner.search.return_value = tags

        results = self.provider.search("solo")
        self.assertEqual(results, tags)
        self.mock_inner.search.assert_called_once_with("solo")

    def test_search_cache_wrong_shape_propagates(self) -> None:
        """缓存数据形状错误（TypeError）不再被吞掉：直接抛出。"""
        now = int(time.time())
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                ("solo", json.dumps([{"unexpected": "field"}]), now),
            )

        with self.assertRaises(TypeError):
            self.provider.search("solo")

    def test_related_cache_corrupt_json_falls_back_to_upstream(self) -> None:
        """related 缓存数据损坏（JSONDecodeError）回退上游（SWR 语义）。"""
        now = int(time.time())
        tags_key = json.dumps(["1girl"])
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_related_cache (tags, results, updated_at) VALUES (?, ?, ?)",
                (tags_key, "not-json{{{", now),
            )
        mock_res = [DanbooruTag("solo", "单人", "Solo.", "General")]
        self.mock_inner.related.return_value = mock_res

        results = self.provider.related(["1girl"])
        self.assertEqual(results, mock_res)
        self.mock_inner.related.assert_called_once_with(
            ["1girl"], target_categories=None
        )

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

    def test_update_cache_related_invalid_json_propagates(self) -> None:
        """related 缓存 key 解析失败不再回退为空数据：JSONDecodeError 直接抛出。"""
        from comfyui.danbooru import update_cache

        with self.assertRaises(json.JSONDecodeError):
            update_cache(
                "related", "{not-json", "https://mock-api.com", db_ctx=self.db_ctx
            )

    @patch("sys.argv", ["comfyui.danbooru", "search", "girl", "https://mock-api.com"])
    @patch("comfyui.danbooru.update_cache")
    def test_danbooru_main_success(self, mock_update: MagicMock) -> None:
        from comfyui.danbooru import main as danbooru_main

        try:
            danbooru_main()
        except SystemExit as e:
            self.assertEqual(e.code, 0)
        mock_update.assert_called_once_with("search", "girl", "https://mock-api.com")


class TestDanbooruTagLoader(unittest.TestCase):

    def setUp(self) -> None:
        self.db_ctx = SQLiteContext(":memory:")
        # 激活建表迁移
        _ = self.db_ctx.connection
        self.mock_inner = MagicMock()
        self.loader = SQLiteDanbooruTagLoader(self.mock_inner, self.db_ctx, ttl=3600)

    def tearDown(self) -> None:
        self.db_ctx.close()

    def test_load_cache_hit(self) -> None:
        now = int(time.time())
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_tag_cache (tag, cn_name, wiki, category, updated_at) VALUES (?, ?, ?, ?, ?)",
                ("1girl", "女孩", "A girl.", "General", now),
            )

        res = self.loader.load("1girl")
        assert res is not None
        self.assertEqual(res.tag, "1girl")
        self.assertEqual(res.cn_name, "女孩")
        self.assertEqual(res.category, "General")
        self.mock_inner.load.assert_not_called()

    def test_load_cache_miss_calls_inner(self) -> None:
        mock_tag = DanbooruTag("solo", "单人", "Solo.", "General")
        self.mock_inner.load.return_value = mock_tag

        res = self.loader.load("solo")
        self.assertEqual(res, mock_tag)
        self.mock_inner.load.assert_called_once_with("solo")

        row = self.db_ctx.connection.execute(
            "SELECT cn_name, wiki, category FROM danbooru_tag_cache WHERE tag = ?",
            ("solo",),
        ).fetchone()
        self.assertIsNotNone(row)
        self.assertEqual(row[0], "单人")
        self.assertEqual(row[2], "General")

    def test_load_cache_expired(self) -> None:
        now = int(time.time())
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_tag_cache (tag, cn_name, wiki, category, updated_at) VALUES (?, ?, ?, ?, ?)",
                ("1girl", "女孩", "A girl.", "General", now - 7200),
            )

        mock_tag = DanbooruTag("1girl", "女孩新", "A girl.", "General")
        self.mock_inner.load.return_value = mock_tag

        res = self.loader.load("1girl")
        self.assertEqual(res, mock_tag)
        self.mock_inner.load.assert_called_once_with("1girl")

    def test_load_cache_read_error_falls_back_to_inner(self) -> None:
        """缓存读取失败（sqlite3.Error）回退到 inner loader（SWR 语义）。"""
        broken_conn = MagicMock()
        broken_conn.execute.side_effect = sqlite3.OperationalError("database is locked")
        self.mock_inner.load.return_value = None

        with patch.object(
            SQLiteContext,
            "connection",
            new_callable=PropertyMock,
            return_value=broken_conn,
        ):
            res = self.loader.load("solo")

        self.assertIsNone(res)
        self.mock_inner.load.assert_called_once_with("solo")

    def test_load_cache_read_unexpected_error_propagates(self) -> None:
        """缓存读取的意外错误（非 sqlite3.Error）不再被吞掉：直接抛出。"""
        broken_conn = MagicMock()
        broken_conn.execute.side_effect = KeyError("boom")

        with patch.object(
            SQLiteContext,
            "connection",
            new_callable=PropertyMock,
            return_value=broken_conn,
        ):
            with self.assertRaises(KeyError):
                self.loader.load("solo")

    def test_write_cache_error_propagates(self) -> None:
        """缓存写入失败不再静默吞掉：直接抛出（快速失败）。"""
        broken_conn = MagicMock()
        broken_conn.execute.side_effect = sqlite3.OperationalError("disk I/O error")

        with patch.object(
            SQLiteContext,
            "connection",
            new_callable=PropertyMock,
            return_value=broken_conn,
        ):
            with self.assertRaises(sqlite3.Error):
                self.loader.write_cache(DanbooruTag("solo", "单人", "Solo.", "General"))

    @patch("requests.post")
    def test_akizuki_loader_load_success(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "solo_focus",
                    "cn_name": "单人焦点",
                    "wiki": "",
                    "category": "General",
                },
                {
                    "tag": "solo",
                    "cn_name": "单人",
                    "wiki": "wiki",
                    "category": "General",
                },
            ]
        }
        mock_post.return_value = mock_response

        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        res = raw_loader.load("solo")

        assert res is not None
        self.assertEqual(res.tag, "solo")
        self.assertEqual(res.cn_name, "单人")
        self.assertEqual(res.category, "General")

    @patch("requests.post")
    def test_akizuki_loader_network_failure_propagates(
        self, mock_post: MagicMock
    ) -> None:
        """上游网络失败不再被吞掉并返回 None：直接抛出（快速失败）。"""
        mock_post.side_effect = requests.RequestException("Network Error")

        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        with self.assertRaises(requests.RequestException):
            raw_loader.load("solo")

    @patch("requests.post")
    def test_akizuki_loader_http_error_propagates(self, mock_post: MagicMock) -> None:
        """HTTP 非 2xx 不再被吞掉：直接抛出。"""
        mock_response = MagicMock()
        mock_response.raise_for_status.side_effect = requests.HTTPError(
            "500 Server Error"
        )
        mock_post.return_value = mock_response

        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        with self.assertRaises(requests.HTTPError):
            raw_loader.load("solo")

    @patch("requests.post")
    def test_akizuki_loader_missing_field_propagates(
        self, mock_post: MagicMock
    ) -> None:
        """结果项缺字段（编程错误）不再被吞掉：直接抛出。"""
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [{"cn_name": "缺 tag 字段", "category": "General"}]
        }
        mock_post.return_value = mock_response

        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        with self.assertRaises(KeyError):
            raw_loader.load("solo")

    @patch("requests.post")
    def test_akizuki_loader_not_found_returns_none(self, mock_post: MagicMock) -> None:
        """无精确匹配视为「未找到」返回 None，而非错误。"""
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "solo_focus",
                    "cn_name": "单人焦点",
                    "wiki": "",
                    "category": "General",
                },
            ]
        }
        mock_post.return_value = mock_response

        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        self.assertIsNone(raw_loader.load("solo"))

    def test_akizuki_loader_blank_tag_returns_none(self) -> None:
        """空白 tag 视为「未找到」返回 None，不发起请求。"""
        raw_loader = AkizukiDanbooruTagLoader("https://mock-api.com")
        self.assertIsNone(raw_loader.load("   "))
        self.assertIsNone(raw_loader.load(""))

    @patch("requests.post")
    def test_provider_related_consumes_loader(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {"tag": "solo", "cn_name": "单人", "category": "General"},
                {"tag": "1girl", "cn_name": "女孩", "category": "General"},
            ]
        }
        mock_post.return_value = mock_response

        now = int(time.time())
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO danbooru_tag_cache (tag, cn_name, wiki, category, updated_at) VALUES (?, ?, ?, ?, ?)",
                ("1girl", "女孩", "A girl.", "General", now),
            )

        solo_tag = DanbooruTag("solo", "单人", "Solo.", "General")
        self.mock_inner.load.return_value = solo_tag

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
        results = provider.related(["dummy"])

        self.assertEqual(len(results), 2)
        tag_map = {item.tag: item for item in results}
        self.assertIn("1girl", tag_map)
        self.assertEqual(tag_map["1girl"].cn_name, "女孩")
        self.assertEqual(tag_map["1girl"].category, "General")

        self.assertIn("solo", tag_map)
        self.assertEqual(tag_map["solo"].cn_name, "单人")
        self.assertEqual(tag_map["solo"].category, "General")

    @patch("requests.post")
    def test_provider_prepopulates_loader_cache(self, mock_post: MagicMock) -> None:
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [
                {
                    "tag": "red_hair",
                    "cn_name": "红发",
                    "wiki": "Red.",
                    "category": "Copyright",
                }
            ]
        }
        mock_post.return_value = mock_response

        provider = AkizukiDanbooruTagProvider(
            "https://mock-api.com", loader=self.loader
        )
        results = provider.search("red")

        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].tag, "red_hair")
        row = self.db_ctx.connection.execute(
            "SELECT cn_name, category FROM danbooru_tag_cache WHERE tag = ?",
            ("red_hair",),
        ).fetchone()
        self.assertIsNotNone(row)
        self.assertEqual(row[0], "红发")
        self.assertEqual(row[1], "Copyright")
