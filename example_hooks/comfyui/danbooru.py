# -*- coding: utf-8 -*-
import json
import logging
import math
import os
import sqlite3
import subprocess
import sys
import time
import threading
from dataclasses import dataclass
from difflib import SequenceMatcher
from pathlib import Path
from typing import List, Protocol, Optional, Any, Dict, Set, Tuple, cast
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
from .danbooru_embedding import (
    CEILING_COSINE,
    MIN_COSINE,
    EmbeddingError,
    NullSemanticTagSearcher,
    OpenAIEmbeddingClient,
    SemanticTagSearcher,
    cached_semantic_searcher,
    has_compiled_embeddings,
    parse_embedding_endpoint,
)

_LOGGER = logging.getLogger(__name__)


@dataclass(frozen=True)
class DanbooruTag:
    tag: str
    cn_name: str
    wiki: str
    category: str


@dataclass(frozen=True)
class SemanticLayerUnavailable(DanbooruTag):
    """语义层不可用的降级提示项。

    嵌入接口超时或失败时以补全建议项形式告知用户，字面结果不受影响；
    继承 DanbooruTag 以复用既有 search 返回通道，调用方按类型分流出提示项。
    标签字段全部为空：该条目不对应任何真实标签。
    """

    tag: str = ""
    cn_name: str = ""
    wiki: str = ""
    category: str = ""
    reason: str = ""


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
    # 语义层热度分（log1p(post_count) 归一化），仅在语义层激活时惰性计算
    _pop_scores: Optional[List[float]] = None

    @property
    def pop_scores(self) -> List[float]:
        """每条标签的热度分，供语义层做 post_count 加权排序。"""
        if self._pop_scores is None:
            logs = [math.log1p(record.post_count) for record in self.records]
            # post_count 全为 0 时 max_log 为 0，除法无意义：直接给全 0 分
            max_log = max(logs, default=0.0)
            self._pop_scores = (
                [value / max_log for value in logs]
                if max_log > 0
                else [0.0] * len(logs)
            )
        return self._pop_scores


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

    def release_cooc(self) -> None:
        """释放共现索引（下次 related 按需重载）；与懒加载共用锁避免竞态。"""
        with self._load_lock:
            self._cooc_index = None


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


def release_cooc_cache(data_dir: str) -> None:
    """按目录释放已缓存的共现索引；目录未缓存或尚未加载时为 no-op。

    供常驻 serve 在无待处理请求时调用：cooc 常驻占 30MB 进程内存，
    重载实测约 24ms（低于交互感知阈值），换空闲期把内存归还系统。
    """
    with _DATASET_LOCK:
        dataset = _DATASET_CACHE.get(os.path.abspath(data_dir))
    if dataset is None:
        return
    dataset.release_cooc()


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


# 语义层：字面层无强匹配时追加的向量召回
# 召回条数略多于最终展示上限，使 NSFW 过滤后仍能填满 top20
_SEMANTIC_TOP_K = 40
# 语义分映射到字面分层之下、模糊层之上的区间，使两类结果在同一次排序中竞争
_SEMANTIC_SCORE_MAX = 72.0
# post_count 加权占比（与参考实现 DanbooruSearchOnline 的 popularity_weight 一致）
_SEMANTIC_POP_WEIGHT = 0.15


def _semantic_score(cosine: float, pop_score: float) -> float:
    """把余弦相似度与热度分映射到可与字面层比较的分数区间。"""
    weighted = cosine * (1.0 - _SEMANTIC_POP_WEIGHT) + pop_score * _SEMANTIC_POP_WEIGHT
    normalized = (weighted - MIN_COSINE) / (CEILING_COSINE - MIN_COSINE)
    return min(max(normalized, 0.0), 1.0) * _SEMANTIC_SCORE_MAX


class FileDanbooruTagProvider:
    """基于本地编译产物的 Danbooru 标签补全（字面匹配 + 语义层，不依赖在线服务）。

    数据目录由入口注入；构造时校验编译产物存在（快速失败），内容懒加载：
    search 首次调用读 tags 产物，related 首次调用读共现 CSR 产物。
    semantic 由入口注入的语义层（向量产物与嵌入端点齐备时召回，否则为空实现）。
    """

    def __init__(
        self,
        data_dir: str,
        show_nsfw: bool = False,
        *,
        semantic: SemanticTagSearcher,
    ) -> None:
        if not data_dir:
            raise ValueError("data_dir 不能为空")
        self.data_dir = data_dir
        self.show_nsfw = show_nsfw
        self.semantic = semantic
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
        matched_ids: Set[int] = set()
        strong_count = 0
        has_top = False
        for record in index.records:
            if record.nsfw and not show_nsfw:
                continue
            score = _score_literal_match(query_norm, record, allow_fuzzy=False)
            if score > 0:
                scored.append((-score, -record.post_count, record))
                matched_ids.add(record.rec_id)
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
            for length in range(
                max(0, len(query_norm) - max_delta),
                len(query_norm) + max_delta + 1,
            ):
                for record in index.by_norm_len.get(length, ()):
                    if record.nsfw and not show_nsfw:
                        continue
                    if record.rec_id in matched_ids:
                        continue
                    fuzzy = _score_fuzzy_match(query_norm, record)
                    if fuzzy > 0:
                        scored.append((-fuzzy, -record.post_count, record))
                        matched_ids.add(record.rec_id)

        # 语义层：仅当字面层无强匹配（精确等于标签名或中文别名）时追加；
        # 接口超时或失败时降级为「字面结果 + 明确错误提示项」，补全不中断
        semantic_unavailable: Optional[str] = None
        if not has_top:
            try:
                hits = self.semantic.search(query.strip(), _SEMANTIC_TOP_K)
            except EmbeddingError as e:
                _LOGGER.warning("Danbooru 语义层不可用: %s", e)
                hits = []
                semantic_unavailable = str(e)
            if hits:
                # 热度分只在语义层真正召回时惰性计算：未配置语义层的用户零开销
                pop_scores = index.pop_scores
                for hit in hits:
                    # 共用 rec_id 作为向量行号，越界说明产物与标签表不对齐
                    if hit.rec_id < 0 or hit.rec_id >= len(index.records):
                        raise ValueError(f"语义层命中越界: {hit.rec_id}")
                    if hit.rec_id in matched_ids:
                        continue
                    record = index.records[hit.rec_id]
                    if record.nsfw and not show_nsfw:
                        continue
                    score = _semantic_score(hit.score, pop_scores[hit.rec_id])
                    if score <= 0:
                        continue
                    matched_ids.add(hit.rec_id)
                    scored.append((-score, -record.post_count, record))
        # 分高优先，同分按热度（post_count 降序）
        scored.sort(key=lambda item: (item[0], item[1]))
        results: List[DanbooruTag] = [
            DanbooruTag(
                tag=record.tag,
                cn_name=record.cn_name,
                wiki=record.wiki,
                category=record.category,
            )
            for _, _, record in scored[:20]
        ]
        if semantic_unavailable is not None:
            results.append(SemanticLayerUnavailable(reason=semantic_unavailable))
        return results

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


# #region Danbooru 缓存端口（在线链路专用）
#
# 缓存被抽象为端口，使裸工厂不必依赖 SQLite：在线链路注入 SQLiteDanbooruCache，
# 离线/复用场景注入 NullDanbooruCache。本地字面链路完全不使用缓存。


@dataclass(frozen=True)
class CachedTagDetail:
    """单个标签详情的缓存行。"""

    cn_name: str
    wiki: str
    category: str
    updated_at: int


@dataclass(frozen=True)
class CachedPayload:
    """序列化结果（搜索/联想）的缓存行。"""

    payload: str
    updated_at: int


class DanbooruCache(Protocol):
    """在线 Danbooru 结果的本地缓存端口（本地/字面链路不使用）。"""

    def tag_detail(self, tag: str) -> Optional[CachedTagDetail]: ...
    def save_tag_detail(
        self, tag: str, cn_name: str, wiki: str, category: str, ttl: int
    ) -> None: ...
    def search_payload(self, query: str) -> Optional[CachedPayload]: ...
    def save_search_payload(self, query: str, payload: str, ttl: int) -> None: ...
    def related_payload(self, key: str) -> Optional[CachedPayload]: ...
    def save_related_payload(self, key: str, payload: str, ttl: int) -> None: ...


class NullDanbooruCache:
    """缓存端口空实现：所有读取返回 None、所有写入为 no-op，无需任何数据库。"""

    def tag_detail(self, tag: str) -> Optional[CachedTagDetail]:
        return None

    def save_tag_detail(
        self, tag: str, cn_name: str, wiki: str, category: str, ttl: int
    ) -> None:
        pass

    def search_payload(self, query: str) -> Optional[CachedPayload]:
        return None

    def save_search_payload(self, query: str, payload: str, ttl: int) -> None:
        pass

    def related_payload(self, key: str) -> Optional[CachedPayload]:
        return None

    def save_related_payload(self, key: str, payload: str, ttl: int) -> None:
        pass


class SQLiteDanbooruCache:
    """缓存端口的 SQLite 实现，承载标签详情与搜索/联想结果的持久化。

    读取失败仅吞掉 sqlite3.Error（记录日志后返回 None），以保留既有的
    stale-while-revalidate 回退语义；其他异常（如数据形状错误）直接抛出。
    写入失败不捕获，交由调用方观察（快速失败）。
    """

    def __init__(self, store: SQLiteContext) -> None:
        self.store = store
        # 常驻 serve 多线程共享同一连接，串行化连接访问
        self._lock = threading.Lock()

    def tag_detail(self, tag: str) -> Optional[CachedTagDetail]:
        try:
            with self._lock:
                row = self.store.connection.execute(
                    "SELECT cn_name, wiki, category, updated_at FROM danbooru_tag_cache WHERE tag = ?",
                    (tag,),
                ).fetchone()
        except sqlite3.Error as e:
            _LOGGER.warning("SQLite tag cache read error: %s", e)
            return None
        if not row:
            return None
        cn_name, wiki, category, updated_at = row
        return CachedTagDetail(
            cn_name=cn_name, wiki=wiki, category=category, updated_at=updated_at
        )

    def save_tag_detail(
        self, tag: str, cn_name: str, wiki: str, category: str, ttl: int
    ) -> None:
        now = int(time.time())
        with self._lock:
            with self.store.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_tag_cache WHERE updated_at < ?",
                    (now - ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_tag_cache (tag, cn_name, wiki, category, updated_at) VALUES (?, ?, ?, ?, ?)",
                    (tag, cn_name, wiki, category, now),
                )

    def search_payload(self, query: str) -> Optional[CachedPayload]:
        try:
            with self._lock:
                row = self.store.connection.execute(
                    "SELECT results, updated_at FROM danbooru_search_cache WHERE query = ?",
                    (query,),
                ).fetchone()
        except sqlite3.Error as e:
            _LOGGER.warning("SQLite search cache read error: %s", e)
            return None
        if not row:
            return None
        payload, updated_at = row
        return CachedPayload(payload=payload, updated_at=updated_at)

    def save_search_payload(self, query: str, payload: str, ttl: int) -> None:
        now = int(time.time())
        with self._lock:
            with self.store.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_search_cache WHERE updated_at < ?",
                    (now - ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_search_cache (query, results, updated_at) VALUES (?, ?, ?)",
                    (query, payload, now),
                )

    def related_payload(self, key: str) -> Optional[CachedPayload]:
        try:
            with self._lock:
                row = self.store.connection.execute(
                    "SELECT results, updated_at FROM danbooru_related_cache WHERE tags = ?",
                    (key,),
                ).fetchone()
        except sqlite3.Error as e:
            _LOGGER.warning("SQLite related cache read error: %s", e)
            return None
        if not row:
            return None
        payload, updated_at = row
        return CachedPayload(payload=payload, updated_at=updated_at)

    def save_related_payload(self, key: str, payload: str, ttl: int) -> None:
        now = int(time.time())
        with self._lock:
            with self.store.transaction() as conn:
                conn.execute(
                    "DELETE FROM danbooru_related_cache WHERE updated_at < ?",
                    (now - ttl,),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO danbooru_related_cache (tags, results, updated_at) VALUES (?, ?, ?)",
                    (key, payload, now),
                )


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


class CachedDanbooruTagLoader:
    """带缓存端口的 DanbooruTagLoader 装饰器。"""

    def __init__(
        self,
        loader: DanbooruTagLoader,
        cache: DanbooruCache,
        ttl: int = 2592000,  # 30天
    ) -> None:
        self.loader = loader
        self.cache = cache
        self.ttl = ttl

    def load(self, tag: str) -> Optional[DanbooruTag]:
        if not tag.strip():
            return None

        now = int(time.time())
        # 1. 尝试从缓存端口读取精确匹配；读取失败由端口返回 None 回退上游（SWR 语义）
        cached = self.cache.tag_detail(tag)
        if cached is not None:
            if now - cached.updated_at < self.ttl:
                return DanbooruTag(
                    tag=tag,
                    cn_name=cached.cn_name,
                    wiki=cached.wiki,
                    category=cached.category,
                )

        # 2. 缓存未命中或已过期，同步调用底层 loader 获取最新信息
        result = self.loader.load(tag)
        if result:
            self.write_cache(result)
        return result

    def write_cache(self, item: DanbooruTag) -> None:
        """保存/更新单个 Tag 的详情至缓存端口。写入失败直接抛出（快速失败）。"""
        self.cache.save_tag_detail(
            item.tag, item.cn_name, item.wiki, item.category, self.ttl
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


class CachedDanbooruTagProvider:
    """带缓存端口装饰的 DanbooruTagProvider 包装器，支持 SWR (Stale-While-Revalidate) 机制。"""

    def __init__(
        self,
        provider: DanbooruTagProvider,
        cache: DanbooruCache,
        search_url: str,
        ttl: int = 86400,
    ) -> None:
        self.provider = provider
        self.cache = cache
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
        """持久化写入前缀搜索结果至缓存端口，并执行过期数据清理。

        缓存写入失败直接抛出（快速失败），调用方可见失败状态。
        """
        payload = json.dumps([item.__dict__ for item in results], ensure_ascii=False)
        self.cache.save_search_payload(query, payload, self.ttl)

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
        """持久化写入联想词结果至缓存端口，并执行过期数据清理。

        缓存写入失败直接抛出（快速失败），调用方可见失败状态。
        """
        tags_key = self._make_related_cache_key(tags, target_categories)
        payload = json.dumps([item.__dict__ for item in results], ensure_ascii=False)
        self.cache.save_related_payload(tags_key, payload, self.ttl)

    def search(self, query: str) -> List[DanbooruTag]:
        now = int(time.time())
        cached_results: Optional[List[DanbooruTag]] = None
        is_stale = False

        # 1. 尝试从缓存端口读取缓存；快照 JSON 损坏（JSONDecodeError）回退上游（SWR 语义）
        cached_payload = self.cache.search_payload(query)
        if cached_payload is not None:
            try:
                data = json.loads(cached_payload.payload)
                cached_results = [DanbooruTag(**item) for item in data]
            except json.JSONDecodeError as e:
                _LOGGER.warning("SQLite search cache read error: %s", e)
            else:
                if now - cached_payload.updated_at >= self.ttl:
                    is_stale = True

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
        cached_results: Optional[List[DanbooruTag]] = None
        is_stale = False

        # 1. 尝试从缓存端口读取缓存；快照 JSON 损坏（JSONDecodeError）回退上游（SWR 语义）
        cached_payload = self.cache.related_payload(tags_key)
        if cached_payload is not None:
            try:
                data = json.loads(cached_payload.payload)
                cached_results = [DanbooruTag(**item) for item in data]
            except json.JSONDecodeError as e:
                _LOGGER.warning("SQLite related cache read error: %s", e)
            else:
                if now - cached_payload.updated_at >= self.ttl:
                    is_stale = True

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


# #region 裸工厂（外部消费者复用入口）


def build_semantic_searcher(data_dir: str, endpoint_url: str) -> SemanticTagSearcher:
    """构建本地链路的语义层；不激活时显式传空实现，不让 provider 自行降级。

    激活需同时满足两个条件：标签向量产物存在、嵌入服务 URL 非空。缺任一时
    补全完全退回纯字面匹配，对未配置用户零行为变化。
    """
    if not endpoint_url or not has_compiled_embeddings(data_dir):
        return NullSemanticTagSearcher()
    endpoint = parse_embedding_endpoint(endpoint_url)
    return cached_semantic_searcher(data_dir, endpoint, OpenAIEmbeddingClient(endpoint))


def build_danbooru_tag_provider(
    *,
    data_dir: str,
    search_url: str,
    show_nsfw: bool,
    embedding_provider_url: str,
    cache: DanbooruCache,
) -> Optional[DanbooruTagProvider]:
    """构建裸的 Danbooru 标签补全 provider，供外部消费者（如外部 ComfyUI 节点）复用。

    与指令上下文解耦：不读取任何环境变量，不依赖 root_dir / directory_rel_path /
    target_command，也不持有 SQLite。data_dir 非空时优先启用本地编译产物（即使
    search_url 同时配置），编译产物缺失由 FileDanbooruTagProvider 构造时抛出
    FileNotFoundError；否则 search_url 非空时启用 Akizuki 在线链路，结果经传入的
    cache 端口缓存；两者皆空返回 None，表示「未配置任何数据来源」，由调用方决定如何呈现。
    cache 必须由调用者显式注入（在线链路可传 SQLiteDanbooruCache，无需持久化时传
    NullDanbooruCache），本地链路不使用 cache。
    """
    if data_dir:
        return FileDanbooruTagProvider(
            data_dir,
            show_nsfw=show_nsfw,
            semantic=build_semantic_searcher(data_dir, embedding_provider_url),
        )
    if search_url:
        loader = CachedDanbooruTagLoader(AkizukiDanbooruTagLoader(search_url), cache)
        inner = AkizukiDanbooruTagProvider(
            search_url, loader=loader, show_nsfw=show_nsfw
        )
        return CachedDanbooruTagProvider(inner, cache, search_url)
    return None


# #endregion


def update_cache(
    method: str,
    key_arg: str,
    search_url: str,
    db_ctx: Optional[SQLiteContext] = None,
) -> None:
    """供异步子进程调用的接口，用来执行真实的后台缓存更新。"""
    if db_ctx is None:
        db_ctx = SQLiteContext.from_env()

    cache = SQLiteDanbooruCache(db_ctx)
    raw_loader = AkizukiDanbooruTagLoader(search_url)
    cache_loader = CachedDanbooruTagLoader(raw_loader, cache)
    akizuki = AkizukiDanbooruTagProvider.from_env(search_url, loader=cache_loader)
    provider = CachedDanbooruTagProvider(akizuki, cache, search_url)

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
