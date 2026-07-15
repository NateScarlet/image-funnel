# -*- coding: utf-8 -*-
import json
import logging
import os
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import List, Protocol, Optional
import requests

from .db import SQLiteContext

_LOGGER = logging.getLogger(__name__)


@dataclass(frozen=True)
class DanbooruTag:
    tag: str
    cn_name: str
    wiki: str
    category: str


class DanbooruTagProvider(Protocol):
    """Danbooru 标签自动补全提供者接口。"""

    def search(self, query: str) -> List[DanbooruTag]:
        """前缀搜索 Danbooru 提示词。"""
        ...

    def related(self, tags: List[str]) -> List[DanbooruTag]:
        """联想与指定 tags 列表相关的提示词。"""
        ...


class AkizukiDanbooruTagProvider:
    """Akizuki Danbooru 服务提供的标签补全实现。"""

    def __init__(self, search_url: str, show_nsfw: bool = False) -> None:
        self.search_url = search_url.rstrip("/")
        self.show_nsfw = show_nsfw

    @classmethod
    def from_env(cls, search_url: str) -> "AkizukiDanbooruTagProvider":
        show_nsfw_env = os.getenv("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower()
        show_nsfw = show_nsfw_env in ("true", "1", "yes", "on")
        return cls(search_url, show_nsfw=show_nsfw)

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
                tag = item.get("tag", "")
                if not tag:
                    continue
                tags.append(
                    DanbooruTag(
                        tag=tag,
                        cn_name=item.get("cn_name", ""),
                        wiki=item.get("wiki", ""),
                        category=item.get("category", ""),
                    )
                )
            return tags
        except requests.RequestException as e:
            _LOGGER.error("Failed to fetch Danbooru suggestions: %s", e, exc_info=True)
            raise

    def related(self, tags: List[str]) -> List[DanbooruTag]:
        if not tags:
            return []

        api_url = f"{self.search_url}/api/related"

        payload = {
            "tags": tags,
            "limit": 100,
            "show_nsfw": self.show_nsfw,
        }

        try:
            _LOGGER.debug(
                "Fetching Danbooru related tags for: %r from URL: %r", tags, api_url
            )
            response = requests.post(api_url, json=payload, timeout=5.0)
            _LOGGER.debug("Danbooru related response status: %d", response.status_code)
            response.raise_for_status()
            res_json = response.json()
            results = res_json.get("results", [])
            _LOGGER.debug("Danbooru related search returned %d items", len(results))

            tags_list: List[DanbooruTag] = []
            for item in results:
                tag = item.get("tag", "")
                if not tag:
                    continue
                tags_list.append(
                    DanbooruTag(
                        tag=tag,
                        cn_name=item.get("cn_name", ""),
                        wiki=item.get("wiki", ""),
                        category=item.get("category", ""),
                    )
                )
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
        """异步拉起子进程更新本地缓存，保证 CLI 的快速响应。"""
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
        except Exception as e:
            _LOGGER.warning("Failed to spawn async cache updater process: %s", e)

    def write_search_cache(self, query: str, results: List[DanbooruTag]) -> None:
        """持久化写入前缀搜索结果至 SQLite 缓存中，并执行过期数据清理。"""
        now = int(time.time())
        try:
            data_str = json.dumps(
                [item.__dict__ for item in results], ensure_ascii=False
            )
            with self.db_ctx.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_search_cache WHERE updated_at < ?",
                    (now - self.ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                    (query, data_str, now),
                )
        except Exception as e:
            _LOGGER.warning("SQLite search cache write error: %s", e)

    def write_related_cache(self, tags: List[str], results: List[DanbooruTag]) -> None:
        """持久化写入联想词结果至 SQLite 缓存中，并执行过期数据清理。"""
        now = int(time.time())
        tags_key = json.dumps(tags)
        try:
            data_str = json.dumps(
                [item.__dict__ for item in results], ensure_ascii=False
            )
            with self.db_ctx.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_related_cache WHERE updated_at < ?",
                    (now - self.ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_related_cache (tags, results, updated_at) VALUES (?, ?, ?)",
                    (tags_key, data_str, now),
                )
        except Exception as e:
            _LOGGER.warning("SQLite related cache write error: %s", e)

    def search(self, query: str) -> List[DanbooruTag]:
        now = int(time.time())
        cached_results = None
        is_stale = False

        # 1. 尝试从 SQLite 读取缓存
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
        except Exception as e:
            _LOGGER.warning("SQLite search cache read error: %s", e)

        # 2. 如果缓存新鲜，直接返回
        if cached_results is not None and not is_stale:
            return cached_results

        # 3. 如果缓存已过期，直接返回已过期的缓存，并在后台异步请求上游拉取更新
        if cached_results is not None and is_stale:
            self._trigger_async_update("search", query)
            return cached_results

        # 4. 如果没有缓存，则同步请求上游（主线程阻塞）
        try:
            results = self.provider.search(query)
        except Exception as e:
            _LOGGER.error("Danbooru search upstream error: %s", e, exc_info=True)
            raise

        # 5. 写入缓存并清理已过期缓存
        self.write_search_cache(query, results)
        return results

    def related(self, tags: List[str]) -> List[DanbooruTag]:
        if not tags:
            return []

        tags_key = json.dumps(tags)
        now = int(time.time())
        cached_results = None
        is_stale = False

        # 1. 尝试从 SQLite 读取缓存
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
        except Exception as e:
            _LOGGER.warning("SQLite related cache read error: %s", e)

        # 2. 如果缓存新鲜，直接返回
        if cached_results is not None and not is_stale:
            return cached_results

        # 3. 如果缓存已过期，直接返回已过期的缓存，并在后台异步请求上游拉取更新
        if cached_results is not None and is_stale:
            self._trigger_async_update("related", tags_key)
            return cached_results

        # 4. 如果没有缓存，则同步请求上游
        try:
            results = self.provider.related(tags)
        except Exception as e:
            _LOGGER.error("Danbooru related upstream error: %s", e, exc_info=True)
            raise

        # 5. 写入缓存并清理已过期缓存
        self.write_related_cache(tags, results)
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

    akizuki = AkizukiDanbooruTagProvider.from_env(search_url)
    provider = SQLiteDanbooruTagProvider(akizuki, db_ctx, search_url)

    with db_ctx:
        if method == "search":
            results = akizuki.search(key_arg)
            provider.write_search_cache(key_arg, results)
        elif method == "related":
            tags = json.loads(key_arg)
            results = akizuki.related(tags)
            provider.write_related_cache(tags, results)


def main() -> None:
    logging.basicConfig(level=logging.INFO)
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
    main()
