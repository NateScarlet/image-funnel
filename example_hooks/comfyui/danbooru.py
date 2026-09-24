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
from difflib import SequenceMatcher
from pathlib import Path
from typing import List, Protocol, Optional, Any, Dict, Tuple, cast
import requests

from .db import SQLiteContext
from .danbooru_data import (
    COMPILE_SCRIPT_HINT,
    COMPILED_COOC_FILENAME,
    COMPILED_TAGS_FILENAME,
    CompiledCoocIndex,
    CompiledTagRecord,
    load_compiled_cooc,
    load_compiled_tags,
    normalize_literal,
)

_LOGGER = logging.getLogger(__name__)


@dataclass(frozen=True)
class DanbooruTag:
    tag: str
    cn_name: str
    wiki: str
    category: str


# #region 本地数据文件 provider（FileDanbooruTagProvider）


# 模块级数据集缓存：serve 每请求构建 provider，避免重复建索引（内容仍懒加载）
_DATASET_CACHE: Dict[str, "_FileDataset"] = {}
_DATASET_LOCK = threading.Lock()


# 编译产物记录与运行时标签记录同构；类型别名便于搜索/联想共用打分路径
_FileTagRecord = CompiledTagRecord


@dataclass
class _FileTagIndex:
    """编译 tags 产物加载结果；按 name_norm 长度分桶供模糊匹配取候选。"""

    records: List[CompiledTagRecord]
    by_name: Dict[str, CompiledTagRecord]
    # length(name_norm) -> 该长度的记录列表（模糊匹配只扫长度邻域）
    by_norm_len: Dict[int, List[CompiledTagRecord]]


class _FileDataset:
    """按目录缓存的数据集：tags 与 cooc 分别在首次使用时懒加载。"""

    def __init__(self, tags_path: Path, cooc_path: Path) -> None:
        self.tags_path = tags_path
        self.cooc_path = cooc_path
        self._load_lock = threading.Lock()
        self._tag_index: Optional[_FileTagIndex] = None
        # 双向 CSR 共现索引（节点 id = tags 数组下标）
        self._cooc_index: Optional[CompiledCoocIndex] = None

    def tag_index(self) -> _FileTagIndex:
        if self._tag_index is None:
            with self._load_lock:
                if self._tag_index is None:
                    self._tag_index = _load_tag_index(self.tags_path)
        return self._tag_index

    def cooc_index(self) -> CompiledCoocIndex:
        if self._cooc_index is None:
            with self._load_lock:
                if self._cooc_index is None:
                    self._cooc_index = load_compiled_cooc(self.cooc_path)
        return self._cooc_index


def _load_tag_index(tags_path: Path) -> _FileTagIndex:
    """加载编译 tags 产物并建立 by_name / 长度分桶索引。"""
    records = load_compiled_tags(tags_path)
    by_name: Dict[str, CompiledTagRecord] = {}
    by_norm_len: Dict[int, List[CompiledTagRecord]] = {}
    for record in records:
        by_name[record.tag] = record
        by_norm_len.setdefault(len(record.name_norm), []).append(record)
    return _FileTagIndex(records=records, by_name=by_name, by_norm_len=by_norm_len)


def _get_or_create_dataset(data_dir: str) -> _FileDataset:
    """取（或建）目录对应的数据集；缺编译产物时抛出含编译脚本指引的错误。"""
    cache_key = os.path.abspath(data_dir)
    with _DATASET_LOCK:
        cached = _DATASET_CACHE.get(cache_key)
        if cached is not None:
            return cached

        tags_path = Path(data_dir) / COMPILED_TAGS_FILENAME
        cooc_path = Path(data_dir) / COMPILED_COOC_FILENAME
        missing = [str(p) for p in (tags_path, cooc_path) if not p.is_file()]
        if missing:
            missing_names = ", ".join(Path(m).name for m in missing)
            raise FileNotFoundError(
                f"Danbooru 编译产物缺失: {missing_names}（目录: {data_dir}）\n"
                f"{COMPILE_SCRIPT_HINT}"
            )

        dataset = _FileDataset(tags_path=tags_path, cooc_path=cooc_path)
        _DATASET_CACHE[cache_key] = dataset
        return dataset


# 模糊层仅对短查询启用：长查询长度邻域候选爆炸且几乎不可能命中
_FUZZY_MAX_QUERY_LEN = 12


def _normalize_literal(text: str) -> str:
    """字面匹配用归一化（编译期与运行时共用同一实现）。"""
    return normalize_literal(text)


def _score_literal_match(
    query_norm: str, record: _FileTagRecord, *, allow_fuzzy: bool = True
) -> float:
    """分层打分：精确 > 中文精确 > 前缀 > 子串 > 模糊；不匹配返回 0。

    allow_fuzzy=False 时只跑无 SequenceMatcher 的分层（search 热路径第一趟）。
    首字符快速拒绝：子串/别名层先查 query[0] 是否出现，避免对 5 万条全量扫长别名。
    """
    if not query_norm:
        return 0.0
    q0 = query_norm[0]
    name = record.name_norm
    if name == query_norm:
        return 100.0
    if query_norm in record.cn_aliases_norm:
        return 90.0
    if name.startswith(query_norm):
        return 80.0
    for alias in record.cn_aliases_norm:
        if alias.startswith(query_norm):
            return 75.0
    if q0 in name and query_norm in name:
        return 70.0
    for alias in record.cn_aliases_norm:
        if q0 in alias and query_norm in alias:
            return 65.0
    if not allow_fuzzy:
        return 0.0
    if len(query_norm) > _FUZZY_MAX_QUERY_LEN:
        return 0.0
    return _score_fuzzy_match(query_norm, record)


def _fuzzy_ratio(query: str, candidate: str) -> float:
    """SequenceMatcher 相似度；quick 上界不足 0.75 时跳过全量 ratio。"""
    matcher = SequenceMatcher(None, query, candidate)
    # real_quick_ratio / quick_ratio 均为 ratio 上界，可安全短路
    if matcher.real_quick_ratio() < 0.75 or matcher.quick_ratio() < 0.75:
        return 0.0
    ratio = matcher.ratio()
    return ratio if ratio >= 0.75 else 0.0


def _fuzzy_maybe_overlap(query_norm: str, candidate: str) -> bool:
    """模糊前的首字符粗筛：ratio≥0.75 的命中几乎总在前 3 字符内共享查询首字符。

    无此过滤时长度邻域可达 2 万+候选，每条 SequenceMatcher 构造即拖垮热路径；
    有此过滤后候选降到千级，typo 查询可进 100ms。
    """
    if not candidate:
        return False
    q0 = query_norm[0]
    if candidate[0] == q0:
        return True
    # 首字符笔误/插入时，查询首字符常出现在候选前 3 字符内
    return q0 in candidate[:3]


def _score_fuzzy_match(query_norm: str, record: _FileTagRecord) -> float:
    """仅模糊层：英文 name_norm 与中文别名，长度邻域 + 首字符粗筛 + quick 上界。"""
    max_delta = max(3, len(query_norm) // 2)
    name = record.name_norm
    if abs(len(query_norm) - len(name)) <= max_delta and _fuzzy_maybe_overlap(
        query_norm, name
    ):
        ratio = _fuzzy_ratio(query_norm, name)
        if ratio > 0:
            return 50.0 + ratio * 15.0
    for alias in record.cn_aliases_norm:
        if abs(len(query_norm) - len(alias)) <= max_delta and _fuzzy_maybe_overlap(
            query_norm, alias
        ):
            alias_ratio = _fuzzy_ratio(query_norm, alias)
            if alias_ratio > 0:
                return 45.0 + alias_ratio * 15.0
    return 0.0


class FileDanbooruTagProvider:
    """基于本地编译产物的 Danbooru 标签补全（字面匹配，容忍笔误，不依赖在线服务）。

    数据目录由入口注入；构造时校验编译产物存在（快速失败），内容懒加载：
    search 首次调用读 tags 产物，related 首次调用读共现 CSR 产物。
    """

    def __init__(self, data_dir: str, show_nsfw: bool = False) -> None:
        if not data_dir:
            raise ValueError("data_dir 不能为空")
        self.data_dir = data_dir
        self.show_nsfw = show_nsfw
        self._dataset = _get_or_create_dataset(data_dir)

    def search(self, query: str) -> List[DanbooruTag]:
        if not query.strip():
            return []
        query_norm = _normalize_literal(query)
        if not query_norm:
            return []

        index = self._dataset.tag_index()
        show_nsfw = self.show_nsfw

        # 第一趟：无模糊（精确/前缀/子串/中文别名），避免全表 SequenceMatcher
        scored: List[Tuple[float, int, _FileTagRecord]] = []
        strong_count = 0
        has_top = False
        for record in index.records:
            if record.nsfw and not show_nsfw:
                continue
            score = _score_literal_match(query_norm, record, allow_fuzzy=False)
            if score > 0:
                scored.append((-score, -record.post_count, record))
                # 模糊分 <=65，子串(65)及以上已能占满 top20 时无需模糊
                if score >= 65:
                    strong_count += 1
                # 精确/中文别名精确已到顶，再补模糊无益于排序
                if score >= 90:
                    has_top = True

        # 第二趟：仅当强匹配不足且非精确命中、查询足够短时，在长度邻域补模糊
        if (
            strong_count < 20
            and not has_top
            and len(query_norm) <= _FUZZY_MAX_QUERY_LEN
        ):
            max_delta = max(3, len(query_norm) // 2)
            seen_ids = {id(r) for _, _, r in scored}
            for length in range(
                max(0, len(query_norm) - max_delta),
                len(query_norm) + max_delta + 1,
            ):
                for record in index.by_norm_len.get(length, ()):
                    if record.nsfw and not show_nsfw:
                        continue
                    if id(record) in seen_ids:
                        continue
                    fuzzy = _score_fuzzy_match(query_norm, record)
                    if fuzzy > 0:
                        scored.append((-fuzzy, -record.post_count, record))
                        seen_ids.add(id(record))

        # 分高优先，同分按热度（post_count 降序）
        scored.sort(key=lambda item: (item[0], item[1]))
        return [
            DanbooruTag(
                tag=record.tag,
                cn_name=record.cn_name,
                wiki=record.wiki,
                category=record.category,
            )
            for _, _, record in scored[:20]
        ]

    def related(
        self,
        tags: List[str],
        target_categories: Optional[List[str]] = None,
    ) -> List[DanbooruTag]:
        if not tags:
            return []
        index = self._dataset.tag_index()
        seed_set = set(tags)
        # 种子名 → 节点 id（rec_id 即 CSR 下标）
        seed_ids = {
            index.by_name[name].rec_id for name in seed_set if name in index.by_name
        }
        neighbor_scores = self._dataset.cooc_index().aggregate(seed_ids)
        if not neighbor_scores:
            return []

        category_filter = (
            set(target_categories) if target_categories is not None else None
        )
        scored: List[Tuple[int, _FileTagRecord]] = []
        for nid, score in neighbor_scores.items():
            # 共现边两端在编译期已保证存在于 tags 表；仍用下标防御越界
            if nid < 0 or nid >= len(index.records):
                continue
            record = index.records[nid]
            if record.nsfw and not self.show_nsfw:
                continue
            if category_filter is not None and record.category not in category_filter:
                continue
            scored.append((score, record))
        scored.sort(key=lambda item: (-item[0], -item[1].post_count))
        return [
            DanbooruTag(
                tag=record.tag,
                cn_name=record.cn_name,
                wiki=record.wiki,
                category=record.category,
            )
            for _, record in scored[:100]
        ]


# #endregion


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
