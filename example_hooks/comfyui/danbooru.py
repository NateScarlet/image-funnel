# -*- coding: utf-8 -*-
import json
import logging
import os
import sqlite3
import subprocess
import sys
import time
import threading
from dataclasses import dataclass
from typing import List, Protocol, Optional, Any, Dict, cast
import requests

from .db import SQLiteContext

_LOGGER = logging.getLogger(__name__)


@dataclass(frozen=True)
class DanbooruTag:
    tag: str
    cn_name: str
    wiki: str
    category: str


class DanbooruTagLoader(Protocol):
    """支持按精确名称查询/加载单个 Danbooru 标签详情的接口。"""

    def load(self, tag: str) -> Optional[DanbooruTag]:
        """精确按 tag 名称加载详情。"""
        ...

    def write_cache(self, item: DanbooruTag) -> None:
        """回填写入缓存数据（可选）。"""
        ...


class AkizukiDanbooruTagLoader:
    """通过 Akizuki 在线接口精确匹配并加载单个 DanbooruTag 的实体。"""

    def __init__(self, search_url: str) -> None:
        self.search_url = search_url

    @classmethod
    def from_env(cls, search_url: str) -> "AkizukiDanbooruTagLoader":
        return cls(search_url)

    def write_cache(self, item: DanbooruTag) -> None:
        """AkizukiDanbooruTagLoader 本身无缓存写操作，此处直接 pass。"""
        pass

    def load(self, tag: str) -> Optional[DanbooruTag]:
        if not tag.strip():
            return None

        api_url = f"{self.search_url}/api/search"
        payload = {
            "query": tag,
            "top_k": 5,
            "limit": 5,
            "popularity_weight": 0.0,
            "show_nsfw": True,
            "use_segmentation": False,
        }

        # 快速失败：网络/HTTP/响应解析错误一律抛出，只有「未找到」返回 None
        response = requests.post(api_url, json=payload, timeout=5.0)
        response.raise_for_status()
        res_json = response.json()
        results = res_json.get("results", [])

        for item in results:
            name = item["tag"]
            if name == tag:
                return DanbooruTag(
                    tag=name,
                    cn_name=item["cn_name"],
                    wiki=item.get("wiki", ""),
                    category=item["category"],
                )
        return None


class SQLiteDanbooruTagLoader:
    """带 SQLite 缓存的 DanbooruTagLoader 装饰器。"""

    def __init__(
        self,
        loader: DanbooruTagLoader,
        db_ctx: SQLiteContext,
        ttl: int = 2592000,  # 30天
    ) -> None:
        self.loader = loader
        self.db_ctx = db_ctx
        self.ttl = ttl
        self._lock = threading.Lock()

    def load(self, tag: str) -> Optional[DanbooruTag]:
        if not tag.strip():
            return None

        now = int(time.time())
        # 1. 尝试从 SQLite 中读取精确匹配的缓存；读取失败（sqlite3.Error）回退上游（SWR 语义）
        try:
            with self._lock:
                row = self.db_ctx.connection.execute(
                    "SELECT cn_name, wiki, category, updated_at FROM danbooru_tag_cache WHERE tag = ?",
                    (tag,),
                ).fetchone()
            if row:
                cn_name, wiki, category, updated_at = row
                if now - updated_at < self.ttl:
                    return DanbooruTag(
                        tag=tag, cn_name=cn_name, wiki=wiki, category=category
                    )
        except sqlite3.Error as e:
            _LOGGER.warning("SQLite tag cache read error: %s", e)

        # 2. 缓存未命中或已过期，同步调用底层 loader 获取最新信息
        result = self.loader.load(tag)
        if result:
            self.write_cache(result)
        return result

    def write_cache(self, item: DanbooruTag) -> None:
        """保存/更新单个 Tag 的详情至缓存。缓存写入失败直接抛出（快速失败）。"""
        now = int(time.time())
        with self._lock:
            with self.db_ctx.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_tag_cache WHERE updated_at < ?",
                    (now - self.ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_tag_cache (tag, cn_name, wiki, category, updated_at) VALUES (?, ?, ?, ?, ?)",
                    (item.tag, item.cn_name, item.wiki, item.category, now),
                )


class DanbooruTagProvider(Protocol):
    """Danbooru 标签自动补全提供者接口。"""

    def search(self, query: str) -> List[DanbooruTag]:
        """前缀搜索 Danbooru 提示词。"""
        ...

    def related(
        self,
        tags: List[str],
        target_categories: Optional[List[str]] = None,
    ) -> List[DanbooruTag]:
        """联想与指定 tags 列表相关的提示词。"""
        ...


class AkizukiDanbooruTagProvider:
    """Akizuki Danbooru 服务提供的标签补全实现。"""

    def __init__(
        self,
        search_url: str,
        loader: DanbooruTagLoader,
        show_nsfw: bool = False,
    ) -> None:
        self.search_url = search_url.rstrip("/")
        self.loader = loader
        self.show_nsfw = show_nsfw

    @classmethod
    def from_env(
        cls,
        search_url: str,
        loader: DanbooruTagLoader,
    ) -> "AkizukiDanbooruTagProvider":
        show_nsfw_env = os.getenv("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower()
        show_nsfw = show_nsfw_env in ("true", "1", "yes", "on")
        return cls(search_url, loader=loader, show_nsfw=show_nsfw)

    def search(self, query: str) -> List[DanbooruTag]:
        if not query.strip():
            return []

        api_url = f"{self.search_url}/api/search"

        payload = {
            "query": query,
            "top_k": 20,
            "limit": 20,
            "popularity_weight": 0.15,
            "show_nsfw": self.show_nsfw,
            "use_segmentation": False,
        }

        _LOGGER.debug(
            "Fetching Danbooru suggestions for query: %r from URL: %r",
            query,
            api_url,
        )
        try:
            response = requests.post(api_url, json=payload, timeout=5.0)
            _LOGGER.debug("Danbooru response status: %d", response.status_code)
            response.raise_for_status()
            res_json = response.json()
            results = res_json.get("results", [])
            _LOGGER.debug("Danbooru search returned %d items", len(results))

            tags: List[DanbooruTag] = []
            for item in results:
                tag = item["tag"]
                tag_item = DanbooruTag(
                    tag=tag,
                    cn_name=item["cn_name"],
                    wiki=item.get("wiki", ""),
                    category=item["category"],
                )
                tags.append(tag_item)
                self.loader.write_cache(tag_item)
            return tags
        except requests.RequestException as e:
            _LOGGER.error("Failed to fetch Danbooru suggestions: %s", e, exc_info=True)
            raise

    def related(
        self,
        tags: List[str],
        target_categories: Optional[List[str]] = None,
    ) -> List[DanbooruTag]:
        if not tags:
            return []

        api_url = f"{self.search_url}/api/related"

        payload: Dict[str, Any] = {
            "tags": tags,
            "limit": 100,
            "show_nsfw": self.show_nsfw,
        }
        if target_categories is not None:
            payload["target_categories"] = target_categories

        try:
            _LOGGER.debug(
                "Fetching Danbooru related tags for: %r from URL: %r", tags, api_url
            )
            response = requests.post(api_url, json=payload, timeout=5.0)
            _LOGGER.debug("Danbooru related response status: %d", response.status_code)
            response.raise_for_status()
            res_json = cast(Dict[str, Any], response.json())
            results = cast(List[Dict[str, Any]], res_json.get("results", []))
            _LOGGER.debug("Danbooru related search returned %d items", len(results))

            tags_list: List[DanbooruTag] = []
            for item in results:
                tag = cast(str, item["tag"])
                cn_name = cast(str, item.get("cn_name", ""))
                wiki = cast(str, item.get("wiki", ""))
                category = cast(str, item.get("category", ""))
                tag_item = DanbooruTag(
                    tag=tag,
                    cn_name=cn_name,
                    wiki=wiki,
                    category=category,
                )
                tags_list.append(tag_item)
                self.loader.write_cache(tag_item)

            return tags_list
        except requests.RequestException as e:
            _LOGGER.error("Failed to fetch Danbooru related tags: %s", e, exc_info=True)
            raise


class SQLiteDanbooruTagProvider:
    """带 SQLite 缓存装饰的 DanbooruTagProvider 包装器，支持 SWR (Stale-While-Revalidate) 机制。"""

    def __init__(
        self,
        provider: DanbooruTagProvider,
        db_ctx: SQLiteContext,
        search_url: str,
        ttl: int = 86400,
    ) -> None:
        self.provider = provider
        self.db_ctx = db_ctx
        self.search_url = search_url
        self.ttl = ttl

    def _trigger_async_update(self, method: str, key_arg: str) -> None:
        """异步拉起子进程更新本地缓存，保证 CLI 的快速响应。

        spawn 失败即记录日志（fire-and-forget：本进程明确不等待其结果）。
        """
        cmd = [
            sys.executable,
            "-m",
            "comfyui.danbooru",
            method,
            key_arg,
            self.search_url,
        ]
        try:
            if os.name == "nt":
                subprocess.Popen(
                    cmd,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                    creationflags=0x00000008,  # DETACHED_PROCESS
                )
            else:
                subprocess.Popen(
                    cmd,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                )
        except OSError as e:
            _LOGGER.warning("Failed to spawn async cache updater process: %s", e)

    def write_search_cache(self, query: str, results: List[DanbooruTag]) -> None:
        """持久化写入前缀搜索结果至 SQLite 缓存中，并执行过期数据清理。

        缓存写入失败直接抛出（快速失败），调用方可见失败状态。
        """
        now = int(time.time())
        data_str = json.dumps([item.__dict__ for item in results], ensure_ascii=False)
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "DELETE FROM danbooru_search_cache WHERE updated_at < ?",
                (now - self.ttl,),
            )
            conn.execute(
                "INSERT OR REPLACE INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                (query, data_str, now),
            )

    def _make_related_cache_key(
        self, tags: List[str], target_categories: Optional[List[str]] = None
    ) -> str:
        if target_categories is None:
            return json.dumps(tags)
        return json.dumps({"tags": tags, "target_categories": target_categories})

    def write_related_cache(
        self,
        tags: List[str],
        results: List[DanbooruTag],
        target_categories: Optional[List[str]] = None,
    ) -> None:
        """持久化写入联想词结果至 SQLite 缓存中，并执行过期数据清理。

        缓存写入失败直接抛出（快速失败），调用方可见失败状态。
        """
        now = int(time.time())
        tags_key = self._make_related_cache_key(tags, target_categories)
        data_str = json.dumps([item.__dict__ for item in results], ensure_ascii=False)
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "DELETE FROM danbooru_related_cache WHERE updated_at < ?",
                (now - self.ttl,),
            )
            conn.execute(
                "INSERT OR REPLACE INTO danbooru_related_cache (tags, results, updated_at) VALUES (?, ?, ?)",
                (tags_key, data_str, now),
            )

    def search(self, query: str) -> List[DanbooruTag]:
        now = int(time.time())
        cached_results = None
        is_stale = False

        # 1. 尝试从 SQLite 读取缓存；读取失败（sqlite3/JSON 损坏）回退上游（SWR 语义）
        try:
            row = self.db_ctx.connection.execute(
                "SELECT results, updated_at FROM danbooru_search_cache WHERE query = ?",
                (query,),
            ).fetchone()
            if row:
                results_str, updated_at = row
                data = json.loads(results_str)
                cached_results = [DanbooruTag(**item) for item in data]
                if now - updated_at >= self.ttl:
                    is_stale = True
        except (sqlite3.Error, json.JSONDecodeError) as e:
            _LOGGER.warning("SQLite search cache read error: %s", e)

        # 2. 如果缓存新鲜，直接返回
        if cached_results is not None and not is_stale:
            return cached_results

        # 3. 如果缓存已过期，直接返回已过期的缓存，并在后台异步请求上游拉取更新
        if cached_results is not None and is_stale:
            self._trigger_async_update("search", query)
            return cached_results

        # 4. 如果没有缓存，则同步请求上游（主线程阻塞）；上游错误直接抛出
        try:
            results = self.provider.search(query)
        except requests.RequestException as e:
            _LOGGER.error("Danbooru search upstream error: %s", e, exc_info=True)
            raise

        # 5. 写入缓存并清理已过期缓存
        self.write_search_cache(query, results)
        return results

    def related(
        self,
        tags: List[str],
        target_categories: Optional[List[str]] = None,
    ) -> List[DanbooruTag]:
        if not tags:
            return []

        tags_key = self._make_related_cache_key(tags, target_categories)
        now = int(time.time())
        cached_results = None
        is_stale = False

        # 1. 尝试从 SQLite 读取缓存；读取失败（sqlite3/JSON 损坏）回退上游（SWR 语义）
        try:
            row = self.db_ctx.connection.execute(
                "SELECT results, updated_at FROM danbooru_related_cache WHERE tags = ?",
                (tags_key,),
            ).fetchone()
            if row:
                results_str, updated_at = row
                data = json.loads(results_str)
                cached_results = [DanbooruTag(**item) for item in data]
                if now - updated_at >= self.ttl:
                    is_stale = True
        except (sqlite3.Error, json.JSONDecodeError) as e:
            _LOGGER.warning("SQLite related cache read error: %s", e)

        # 2. 如果缓存新鲜，直接返回
        if cached_results is not None and not is_stale:
            return cached_results

        # 3. 如果缓存已过期，直接返回已过期的缓存，并在后台异步请求上游拉取更新
        if cached_results is not None and is_stale:
            self._trigger_async_update("related", tags_key)
            return cached_results

        # 4. 如果没有缓存，则同步请求上游；上游错误直接抛出
        try:
            results = self.provider.related(tags, target_categories=target_categories)
        except requests.RequestException as e:
            _LOGGER.error("Danbooru related upstream error: %s", e, exc_info=True)
            raise

        # 5. 写入缓存并清理已过期缓存
        self.write_related_cache(tags, results, target_categories=target_categories)
        return results


def update_cache(
    method: str,
    key_arg: str,
    search_url: str,
    db_ctx: Optional[SQLiteContext] = None,
) -> None:
    """供异步子进程调用的接口，用来执行真实的后台缓存更新。"""
    if db_ctx is None:
        db_ctx = SQLiteContext.from_env()

    raw_loader = AkizukiDanbooruTagLoader(search_url)
    cache_loader = SQLiteDanbooruTagLoader(raw_loader, db_ctx)
    akizuki = AkizukiDanbooruTagProvider.from_env(search_url, loader=cache_loader)
    provider = SQLiteDanbooruTagProvider(akizuki, db_ctx, search_url)

    with db_ctx:
        if method == "search":
            results = akizuki.search(key_arg)
            provider.write_search_cache(key_arg, results)
        elif method == "related":
            # 缓存 key 解析失败不再回退为空数据：JSONDecodeError 直接抛出（快速失败）
            parsed = json.loads(key_arg)
            if isinstance(parsed, dict):
                parsed_dict = cast(Dict[str, Any], parsed)
                tags = cast(List[str], parsed_dict.get("tags", []))
                target_categories = cast(
                    Optional[List[str]], parsed_dict.get("target_categories")
                )
            else:
                tags = cast(List[str], parsed)
                target_categories = None

            results = akizuki.related(tags, target_categories=target_categories)
            provider.write_related_cache(
                tags, results, target_categories=target_categories
            )


def main() -> None:
    try:
        method = sys.argv[1]
        key_arg = sys.argv[2]
        search_url = sys.argv[3]
        update_cache(method, key_arg, search_url)
    except Exception as e:
        _LOGGER.error("Failed to async update cache: %s", e, exc_info=True)
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    main()
