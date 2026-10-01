# -*- coding: utf-8 -*-
"""Danbooru 标签语义层：远程 OpenAI 兼容 /v1/embeddings + 本地标签向量矩阵。

语义层只在字面层没有任何强匹配（查询精确等于标签名或某个中文别名）时追加，
把查询与标签都表示为向量，按余弦相似度召回语义相近的标签。标签向量由编译脚本
生成（三视图各一个矩阵，行序与标签表严格对齐），运行时以 mmap 加载，
内存分配交给 OS（同机可能同时跑 ComfyUI，不硬占内存）。
"""

from __future__ import annotations

import logging
import os
import struct
import threading
from collections import OrderedDict
from dataclasses import dataclass
from pathlib import Path
from typing import (
    Any,
    Dict,
    FrozenSet,
    List,
    Optional,
    Protocol,
    Sequence,
    Tuple,
    cast,
)
from urllib.parse import parse_qsl, urlsplit

import numpy as np
import requests

_LOGGER = logging.getLogger(__name__)

# #region 嵌入服务端点

# 语义层端点由用户提供的 OpenAI 兼容服务决定（本地 LM Studio、远程网关等）
EMBEDDING_PROVIDER_URL_ENV = "HOOK_AUTOCOMPLETE_EMBEDDING_PROVIDER_URL"
DEFAULT_EMBEDDING_MODEL = "text-embedding-bge-m3"
DEFAULT_EMBEDDING_TIMEOUT_MS = 3000
DEFAULT_EMBEDDING_PROVIDER_URL = (
    f"http://localhost:1234#apiKey=&model={DEFAULT_EMBEDDING_MODEL}"
    f"&timeoutMs={DEFAULT_EMBEDDING_TIMEOUT_MS}"
)

# URL 片段参数白名单：<base>#apiKey=<k>&model=<m>&timeoutMs=<ms>
_ENDPOINT_PARAMS: FrozenSet[str] = frozenset({"apiKey", "model", "timeoutMs"})
_EMBEDDINGS_PATH = "/v1/embeddings"


@dataclass(frozen=True)
class EmbeddingEndpoint:
    """解析后的嵌入服务端点（apiKey 为空表示不发送 Authorization 头）。"""

    url: str
    model: str
    api_key: str
    timeout: float


def parse_embedding_endpoint(spec: str) -> EmbeddingEndpoint:
    """解析 `<base>#apiKey=&model=&timeoutMs=` 形式的端点 URL。"""
    raw = spec.strip()
    # 环境变量值常被连同引号一起设置（cmd 的 set VAR="..." 会把引号留在值里），
    # 与其让它在下游变成看不懂的 timeoutMs 非法，不如直接指出多余引号
    if len(raw) >= 2 and raw[0] == raw[-1] and raw[0] in ("'", '"'):
        raise ValueError(
            f"嵌入服务 URL 含多余的成对引号: {raw!r}；"
            '环境变量的值不要带引号（TOML 里写成 "..." 时引号是字符串定界符，'
            "不会出现在取到的值中）"
        )
    parts = urlsplit(raw)
    params = dict(parse_qsl(parts.fragment, keep_blank_values=True))
    unknown = set(params) - _ENDPOINT_PARAMS
    if unknown:
        raise ValueError(f"嵌入服务 URL 含未知参数: {sorted(unknown)}")
    base = parts._replace(fragment="").geturl().rstrip("/")
    if not base:
        raise ValueError("嵌入服务 URL 缺少地址")

    raw_timeout = params.get("timeoutMs", str(DEFAULT_EMBEDDING_TIMEOUT_MS))
    try:
        timeout_ms = int(raw_timeout)
    except ValueError as e:
        raise ValueError(f"嵌入服务 URL 的 timeoutMs 非法: {raw_timeout!r}") from e
    if timeout_ms <= 0:
        raise ValueError(f"嵌入服务 URL 的 timeoutMs 必须为正: {timeout_ms}")

    # 基址可省略 /v1 后缀（http://host 与 http://host/v1 等价），
    # 也可直接给到完整 /v1/embeddings；三种写法收敛到同一请求地址
    if base.endswith(_EMBEDDINGS_PATH):
        url = base
    elif base.endswith("/v1"):
        url = base + _EMBEDDINGS_PATH[len("/v1") :]
    else:
        url = base + _EMBEDDINGS_PATH
    return EmbeddingEndpoint(
        url=url,
        model=params.get("model", "").strip() or DEFAULT_EMBEDDING_MODEL,
        api_key=params.get("apiKey", ""),
        timeout=timeout_ms / 1000,
    )


# #endregion

# #region 嵌入客户端


class EmbeddingError(Exception):
    """嵌入接口不可用（网络/超时/HTTP/响应格式）；语义层据此降级为纯字面结果。"""


class EmbeddingClient(Protocol):
    """把文本批量转换为向量的接口。"""

    def embed(self, texts: Sequence[str]) -> List[List[float]]:
        """返回与 texts 等长同序的向量列表；失败一律抛 EmbeddingError。"""
        ...


def _parse_embeddings(payload: object, expected: int) -> List[List[float]]:
    """校验并按 index 归位 OpenAI 兼容的 embeddings 响应。"""
    if not isinstance(payload, dict):
        raise EmbeddingError("嵌入接口响应不是 JSON 对象")
    raw_data = cast(Dict[str, object], payload).get("data")
    if not isinstance(raw_data, list):
        raise EmbeddingError("嵌入接口响应缺少 data 字段")
    entries = cast(List[object], raw_data)
    if len(entries) != expected:
        raise EmbeddingError(
            f"嵌入接口返回条目数不符: 期望 {expected}，实际 {len(entries)}"
        )
    ordered: List[Optional[List[float]]] = [None] * expected
    for position, raw_item in enumerate(entries):
        if not isinstance(raw_item, dict):
            raise EmbeddingError("嵌入接口返回的条目不是对象")
        item = cast(Dict[str, object], raw_item)
        index = item.get("index")
        slot = index if isinstance(index, int) and 0 <= index < expected else position
        vector = item.get("embedding")
        if not isinstance(vector, list) or not vector:
            raise EmbeddingError("嵌入接口返回的向量为空")
        # 声明为数值向量但仍逐项 float() 转换：响应来自网络，非数值必须在这里暴露
        values = cast(List[float], vector)
        try:
            ordered[slot] = [float(value) for value in values]
        except (TypeError, ValueError) as e:
            raise EmbeddingError(f"嵌入接口返回的向量含非数值: {e}") from e
    if any(vector is None for vector in ordered):
        raise EmbeddingError("嵌入接口响应缺少向量")
    return cast(List[List[float]], ordered)


class OpenAIEmbeddingClient:
    """调用 OpenAI 兼容 /v1/embeddings 的嵌入客户端。"""

    def __init__(self, endpoint: EmbeddingEndpoint) -> None:
        self.endpoint = endpoint

    def embed(self, texts: Sequence[str]) -> List[List[float]]:
        items = list(texts)
        if not items:
            return []
        headers: Dict[str, str] = {"Content-Type": "application/json"}
        # apiKey 为空表示端点不需要鉴权，此时不发送 Authorization 头
        if self.endpoint.api_key:
            headers["Authorization"] = f"Bearer {self.endpoint.api_key}"
        try:
            response = requests.post(
                self.endpoint.url,
                json={"model": self.endpoint.model, "input": items},
                headers=headers,
                timeout=self.endpoint.timeout,
            )
            response.raise_for_status()
            payload = response.json()
        except requests.RequestException as e:
            raise EmbeddingError(f"嵌入接口不可用: {e}") from e
        except ValueError as e:
            raise EmbeddingError(f"嵌入接口响应不是合法 JSON: {e}") from e
        return _parse_embeddings(payload, len(items))


# #endregion

# #region 标签向量产物

# 脚本定义的规范产物名（与源数据解耦）；缺此产物时语义层完全不激活
COMPILED_EMBEDDINGS_FILENAME = "embeddings.bin"

_EMBEDDINGS_MAGIC = b"DNEM"
_EMBEDDINGS_VERSION = 1
# header: magic(4) + version(u32le) + n_rows(u32le) + dim(u32le) + n_views(u32le)
_EMBEDDINGS_HEADER = struct.Struct("<4sIIII")

# 三视图分离：tag / cn_name（整段一向量，含多别名）/ wiki 各一个矩阵
EMBEDDING_VIEWS = ("tag", "cn_name", "wiki")

# 编译期默认批量：串行分批请求，进度输出按视图与批次刷新
DEFAULT_EMBEDDING_BATCH_SIZE = 32
# 编译期单批默认超时：批量生成一次请求要发送上千条文本，远慢于交互式单条查询
DEFAULT_COMPILE_TIMEOUT_MS = 60000
# 编译脚本自带的默认端点：与补全侧同一台服务，但超时按编译批量设定
DEFAULT_COMPILE_PROVIDER_URL = (
    f"http://localhost:1234#apiKey=&model={DEFAULT_EMBEDDING_MODEL}"
    f"&timeoutMs={DEFAULT_COMPILE_TIMEOUT_MS}"
)

# #region 编译期用户提示（只由编译脚本打印）
#
# 嵌入向量服务是可选项：没配服务不应该连字面匹配都用不了，因此失败与成功
# 两条路径都要明确告诉用户下一步该做什么。

EMBEDDING_FAILURE_HINT = f"""\
两种处理方式，任选其一：
  1) 配置嵌入向量服务后重新编译（推荐，可启用语义匹配）
     - 设置环境变量 {EMBEDDING_PROVIDER_URL_ENV}=<base>#apiKey=<k>&model=<m>&timeoutMs=<ms>
       （值不要带引号；本地 LM Studio 可直接用 {DEFAULT_COMPILE_PROVIDER_URL}）
     - 或用 --embedding-provider-url 指定本次编译的端点，
       --embedding-timeout-ms / --embedding-batch-size 调整批量与超时
  2) 只要字面匹配：加 --no-embedding 跳过向量生成
     补全仍可按标签名、中文名、前缀、子串与笔误匹配工作，只是不做语义检索
已中止编译，未写入任何产物。"""

SEMANTIC_LAYER_ENABLE_HINT = f"""\
提示：语义匹配还需要在钩子配置的 [env] 里设置 {EMBEDDING_PROVIDER_URL_ENV}
  （值不要带引号；本地 LM Studio 可直接用 {DEFAULT_EMBEDDING_PROVIDER_URL}）
  未设置时补全只使用字面匹配；本次已编译出标签向量，
  之后设置该环境变量即可启用，无需重新编译。"""

# #endregion

# 零向量（如空文本的嵌入）归一化时的分母下界，避免除零产生 NaN 污染排序
_MIN_VECTOR_NORM = 1e-12


def _normalized(vectors: List[List[float]]) -> np.ndarray:
    """把一批向量转 float32 并逐行 L2 归一化（余弦相似度 = 矩阵乘）。"""
    block = np.asarray(vectors, dtype=np.float32)
    norms = np.linalg.norm(block, axis=1, keepdims=True)
    block /= np.maximum(norms, _MIN_VECTOR_NORM)
    return block


@dataclass(frozen=True)
class TagEmbeddingMatrix:
    """三视图标签向量矩阵；行序与标签表严格对齐，行向量为 L2 归一化的 float32。"""

    tag: np.ndarray
    cn_name: np.ndarray
    wiki: np.ndarray

    def views(self) -> List[Tuple[str, np.ndarray]]:
        """按固定视图顺序返回 (视图名, 矩阵) 对。"""
        return [(name, getattr(self, name)) for name in EMBEDDING_VIEWS]

    @property
    def n_rows(self) -> int:
        return int(self.tag.shape[0])

    @property
    def dim(self) -> int:
        return int(self.tag.shape[1])


def write_compiled_embeddings(path: Path, matrix: TagEmbeddingMatrix) -> None:
    """写出标签向量产物：header + 三视图矩阵（float32 LE，行主序）。"""
    header = _EMBEDDINGS_HEADER.pack(
        _EMBEDDINGS_MAGIC,
        _EMBEDDINGS_VERSION,
        matrix.n_rows,
        matrix.dim,
        len(EMBEDDING_VIEWS),
    )
    with path.open("wb") as f:
        f.write(header)
        for _, view in matrix.views():
            f.write(np.ascontiguousarray(view, dtype="<f4").tobytes())


def load_compiled_embeddings(path: Path) -> TagEmbeddingMatrix:
    """mmap 载入标签向量产物；魔数/版本/长度不符时快速失败。"""
    size = path.stat().st_size
    if size < _EMBEDDINGS_HEADER.size:
        raise ValueError(f"标签向量产物过短: {path}")
    with path.open("rb") as f:
        magic, version, n_rows, dim, n_views = _EMBEDDINGS_HEADER.unpack(
            f.read(_EMBEDDINGS_HEADER.size)
        )
    if magic != _EMBEDDINGS_MAGIC:
        raise ValueError(f"标签向量产物格式不符: {path}")
    if version != _EMBEDDINGS_VERSION:
        raise ValueError(
            f"标签向量产物版本不支持: {version}（期望 {_EMBEDDINGS_VERSION}）: {path}"
        )
    if n_views != len(EMBEDDING_VIEWS):
        raise ValueError(
            f"标签向量产物视图数不符: {n_views}（期望 {len(EMBEDDING_VIEWS)}）: {path}"
        )
    matrix_bytes = n_rows * dim * 4
    expected = _EMBEDDINGS_HEADER.size + n_views * matrix_bytes
    if size != expected:
        raise ValueError(
            f"标签向量产物长度不符: 期望 {expected} 字节，实际 {size}: {path}"
        )
    matrices = [
        np.memmap(
            path,
            dtype="<f4",
            mode="r",
            offset=_EMBEDDINGS_HEADER.size + i * matrix_bytes,
            shape=(n_rows, dim),
        )
        for i in range(n_views)
    ]
    return TagEmbeddingMatrix(tag=matrices[0], cn_name=matrices[1], wiki=matrices[2])


class TagEmbeddingSource(Protocol):
    """标签向量来源：为三视图批量生成与标签表行序对齐的向量矩阵。"""

    def build(
        self, views: Sequence[Tuple[str, str, str]]
    ) -> Optional[TagEmbeddingMatrix]:
        """按 (tag, cn_name, wiki) 行序生成矩阵。

        失败一律抛出以中止编译；显式返回 None 表示本次编译不产出向量
        （见 NullTagEmbeddingSource），不是「出错后降级」。
        """
        ...


class NullTagEmbeddingSource:
    """显式空实现：本次编译不产出标签向量（命令行 --no-embedding）。

    只编译字面匹配所需的标签与共现产物；向量层因此保持未激活。
    """

    def build(
        self, views: Sequence[Tuple[str, str, str]]
    ) -> Optional[TagEmbeddingMatrix]:
        return None


class OpenAITagEmbeddingSource:
    """编译期标签向量来源：逐视图串行分批调用嵌入接口。"""

    def __init__(
        self, client: EmbeddingClient, batch_size: int = DEFAULT_EMBEDDING_BATCH_SIZE
    ) -> None:
        self.client = client
        self.batch_size = batch_size

    def build(self, views: Sequence[Tuple[str, str, str]]) -> TagEmbeddingMatrix:
        n_rows = len(views)
        stacked: Optional[np.ndarray] = None
        for view_index, view_name in enumerate(EMBEDDING_VIEWS):
            texts = [row[view_index] for row in views]
            for start in range(0, n_rows, self.batch_size):
                chunk = texts[start : start + self.batch_size]
                block = _normalized(self.client.embed(chunk))
                if stacked is None:
                    stacked = np.empty(
                        (n_rows, len(EMBEDDING_VIEWS), block.shape[1]), dtype=np.float32
                    )
                stacked[start : start + len(chunk), view_index, :] = block
                _LOGGER.info(
                    "标签向量 %s 视图进度 %d/%d",
                    view_name,
                    min(start + len(chunk), n_rows),
                    n_rows,
                )
        if stacked is None:
            raise ValueError("标签视图为空，无法生成向量矩阵")
        columns = [
            np.ascontiguousarray(stacked[:, i, :]) for i in range(len(EMBEDDING_VIEWS))
        ]
        return TagEmbeddingMatrix(tag=columns[0], cn_name=columns[1], wiki=columns[2])


# #endregion

# #region 语义层召回


@dataclass(frozen=True)
class SemanticHit:
    """语义层命中：标签在标签表中的行号与余弦相似度。"""

    rec_id: int
    score: float


# 候选下限：实测 bge-m3 系列对无关标签的相似度中位数约 0.45~0.55，
# 0.5 以下基本是噪声；中文查询整体偏低（Top1 常在 0.65 上下），故下限取 0.5
MIN_COSINE = 0.5
# 归一化上限：相似度达到该值即视为满分命中，避免精确匹配（≈1.0）
# 把其余候选压到语义层底部
CEILING_COSINE = 0.8
# 查询→向量 LRU 容量（命中缓存不调嵌入接口）
QUERY_VECTOR_CACHE_SIZE = 128


class SemanticTagSearcher(Protocol):
    """语义层召回接口；向量产物与嵌入服务缺失时由空实现顶替。"""

    def search(self, query: str, top_k: int) -> List[SemanticHit]:
        """返回按余弦相似度降序的命中；接口不可用时抛 EmbeddingError。"""
        ...


class NullSemanticTagSearcher:
    """语义层未激活时的空实现：缺向量产物或未配置嵌入服务端点。"""

    def search(self, query: str, top_k: int) -> List[SemanticHit]:
        return []


class _QueryVectorCache:
    """查询文本→向量的线程安全 LRU；补全为多线程常驻服务，命中即免去接口调用。"""

    def __init__(self, capacity: int) -> None:
        self._capacity = capacity
        self._items: "OrderedDict[str, np.ndarray]" = OrderedDict()
        self._lock = threading.Lock()

    def get(self, key: str) -> Optional[np.ndarray]:
        with self._lock:
            vector = self._items.get(key)
            if vector is not None:
                self._items.move_to_end(key)
            return vector

    def put(self, key: str, vector: np.ndarray) -> None:
        with self._lock:
            self._items[key] = vector
            self._items.move_to_end(key)
            while len(self._items) > self._capacity:
                self._items.popitem(last=False)


class VectorSemanticTagSearcher:
    """向量召回语义层：查询向量 × 归一化标签矩阵 + BLAS 全量点积。

    标签矩阵在构造时 mmap 一次并常驻：每次查询都要扫全表，逐查询重新映射的
    缺页开销（659MB 产物实测 271ms）远高于映射本身的矩阵乘（实测 26ms），
    常驻映射把页缓存留给 OS 按需换出。
    """

    def __init__(
        self,
        matrix: TagEmbeddingMatrix,
        client: EmbeddingClient,
        query_cache_size: int = QUERY_VECTOR_CACHE_SIZE,
    ) -> None:
        self.matrix = matrix
        self.client = client
        self._cache = _QueryVectorCache(query_cache_size)

    def query_vector(self, query: str) -> np.ndarray:
        """查询文本→向量，走 LRU 缓存；未命中才调用嵌入接口并归一化。"""
        cached = self._cache.get(query)
        if cached is not None:
            return cached
        vector = _normalized(self.client.embed([query]))[0]
        self._cache.put(query, vector)
        return vector

    def search(self, query: str, top_k: int) -> List[SemanticHit]:
        vector = self.query_vector(query)
        # 三视图各取本地 top_k 后按标签取最大相似度（同一标签跨视图只出一条）
        best: Dict[int, float] = {}
        for view_name, view in self.matrix.views():
            scores = view @ vector
            # 全量点积后只取本视图 top_k，避免对 5 万行排序；
            # numpy 的 argpartition 在类型存根里返回 Unknown，故显式收敛为 int 列表
            candidates: List[int] = cast(
                List[int],
                (
                    cast(Any, np.argpartition(scores, -top_k))[-top_k:]
                    if scores.shape[0] > top_k
                    else cast(Any, np.arange(scores.shape[0]))
                ).tolist(),
            )
            kept = 0
            for position in candidates:
                score = float(scores[position])
                if score < MIN_COSINE:
                    continue
                rec_id = int(position)
                previous = best.get(rec_id)
                if previous is None or score > previous:
                    best[rec_id] = score
                kept += 1
            _LOGGER.debug("语义层视图 %s 召回候选 %d 条", view_name, kept)
        hits = [
            SemanticHit(rec_id=rec_id, score=score) for rec_id, score in best.items()
        ]
        hits.sort(key=lambda hit: (-hit.score, hit.rec_id))
        return hits[:top_k]


# #endregion

# #region 语义层实例缓存

# 按 (数据目录, 端点) 缓存：常驻服务每请求重建 provider，查询向量 LRU 必须
# 跨请求存活才能在连续输入时免去接口调用
_SEARCHER_CACHE: Dict[Tuple[str, EmbeddingEndpoint], VectorSemanticTagSearcher] = {}
_SEARCHER_LOCK = threading.Lock()


def has_compiled_embeddings(data_dir: str | Path) -> bool:
    """目录内是否存在标签向量产物（语义层激活条件之一，由入口判定）。"""
    return (Path(data_dir) / COMPILED_EMBEDDINGS_FILENAME).is_file()


def cached_semantic_searcher(
    data_dir: str,
    endpoint: EmbeddingEndpoint,
    client: EmbeddingClient,
) -> VectorSemanticTagSearcher:
    """取（或建）目录与端点对应的向量语义层，映射与 LRU 随实例跨请求复用。

    激活判定（向量产物存在且端点 URL 非空）由入口完成，本函数只负责按需构建。
    """
    key = (os.path.abspath(data_dir), endpoint)
    with _SEARCHER_LOCK:
        cached = _SEARCHER_CACHE.get(key)
        if cached is not None:
            return cached
        matrix = load_compiled_embeddings(Path(data_dir) / COMPILED_EMBEDDINGS_FILENAME)
        searcher = VectorSemanticTagSearcher(matrix, client)
        _SEARCHER_CACHE[key] = searcher
        return searcher
