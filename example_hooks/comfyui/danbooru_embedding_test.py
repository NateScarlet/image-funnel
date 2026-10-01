# -*- coding: utf-8 -*-
"""语义层：端点解析、OpenAI 兼容客户端、标签向量产物读写、向量召回。"""

from __future__ import annotations

import json
import os
import shutil
import tempfile
import threading
import unittest
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import (
    Any,
    Callable,
    Dict,
    Generator,
    List,
    Optional,
    Sequence,
    Tuple,
    cast,
)
from unittest.mock import MagicMock, patch

import numpy as np
import requests

from .danbooru_embedding import (
    CEILING_COSINE,
    COMPILED_EMBEDDINGS_FILENAME,
    DEFAULT_EMBEDDING_MODEL,
    DEFAULT_EMBEDDING_PROVIDER_URL,
    DEFAULT_EMBEDDING_TIMEOUT_MS,
    MIN_COSINE,
    EmbeddingClient,
    EmbeddingError,
    OpenAIEmbeddingClient,
    OpenAITagEmbeddingSource,
    TagEmbeddingMatrix,
    VectorSemanticTagSearcher,
    cached_semantic_searcher,
    load_compiled_embeddings,
    parse_embedding_endpoint,
    write_compiled_embeddings,
)

# 固定维度的可预测向量：让测试显式表达「相似度 = 与该向量夹角」而非依赖随机数
_VECTOR_HALF: List[float] = [1.0, 0.0, 0.0, 0.0]
_VECTOR_SECOND: List[float] = [0.0, 1.0, 0.0, 0.0]
_VECTOR_DIAGONAL: List[float] = [1.0, 1.0, 0.0, 0.0]
_VECTOR_TEXT_LENGTH: Callable[[str], List[float]] = lambda text: [
    float(len(text)),
    1.0,
    0.0,
    0.0,
]
_VECTOR_SECOND_ALWAYS: Callable[[str], List[float]] = lambda text: list(_VECTOR_SECOND)
_VECTOR_HALF_ALWAYS: Callable[[str], List[float]] = lambda text: list(_VECTOR_HALF)
_VECTOR_DIAGONAL_ALWAYS: Callable[[str], List[float]] = lambda text: list(
    _VECTOR_DIAGONAL
)


class RecordingEmbeddingClient:
    """测试用嵌入客户端：记录收到的文本批次，按固定映射返回向量。"""

    def __init__(
        self, vector_for: Optional[Callable[[str], Sequence[float]]] = None
    ) -> None:
        self.batches: List[List[str]] = []
        self._vector_for = vector_for

    def embed(self, texts: Sequence[str]) -> List[List[float]]:
        batch = list(texts)
        self.batches.append(batch)
        if self._vector_for is not None:
            return [
                [float(value) for value in self._vector_for(text)] for text in batch
            ]
        return [[1.0, 0.0, 0.0, 0.0] for _ in batch]


class FailingEmbeddingClient:
    """测试用嵌入客户端：模拟接口超时/不可达。"""

    def __init__(self, message: str = "嵌入接口不可用: 超时") -> None:
        self.message = message
        self.calls = 0

    def embed(self, texts: Sequence[str]) -> List[List[float]]:
        self.calls += 1
        raise EmbeddingError(self.message)


def _unit_vectors(count: int, dim: int) -> np.ndarray:
    """构造 count 个两两正交的归一化向量（dim 需 >= count）。"""
    matrix = np.eye(dim, dtype=np.float32)[:count]
    return matrix


class TestParseEmbeddingEndpoint(unittest.TestCase):
    def test_defaults_from_bare_base(self) -> None:
        endpoint = parse_embedding_endpoint("http://localhost:1234")
        self.assertEqual(endpoint.url, "http://localhost:1234/v1/embeddings")
        self.assertEqual(endpoint.model, DEFAULT_EMBEDDING_MODEL)
        self.assertEqual(endpoint.api_key, "")
        self.assertAlmostEqual(endpoint.timeout, DEFAULT_EMBEDDING_TIMEOUT_MS / 1000)

    def test_fragment_params_apply(self) -> None:
        endpoint = parse_embedding_endpoint(
            "http://host:9/v1#apiKey=secret&model=bge&timeoutMs=250"
        )
        self.assertEqual(endpoint.url, "http://host:9/v1/embeddings")
        self.assertEqual(endpoint.model, "bge")
        self.assertEqual(endpoint.api_key, "secret")
        self.assertAlmostEqual(endpoint.timeout, 0.25)

    def test_trailing_embeddings_path_not_duplicated(self) -> None:
        endpoint = parse_embedding_endpoint("http://host:9/v1/embeddings#model=bge")
        self.assertEqual(endpoint.url, "http://host:9/v1/embeddings")

    def test_documented_default_url_round_trips(self) -> None:
        endpoint = parse_embedding_endpoint(DEFAULT_EMBEDDING_PROVIDER_URL)
        self.assertEqual(endpoint.url, "http://localhost:1234/v1/embeddings")
        self.assertEqual(endpoint.model, "text-embedding-bge-m3")
        self.assertEqual(endpoint.timeout, 3.0)

    def test_unknown_fragment_param_rejected(self) -> None:
        with self.assertRaises(ValueError) as ctx:
            parse_embedding_endpoint("http://host:9#tokne=x")
        self.assertIn("tokne", str(ctx.exception))

    def test_non_positive_timeout_rejected(self) -> None:
        with self.assertRaises(ValueError):
            parse_embedding_endpoint("http://host:9#timeoutMs=0")

    def test_non_numeric_timeout_rejected(self) -> None:
        with self.assertRaises(ValueError) as ctx:
            parse_embedding_endpoint("http://host:9#timeoutMs=abc")
        self.assertIn("timeoutMs", str(ctx.exception))

    def test_empty_base_rejected(self) -> None:
        with self.assertRaises(ValueError):
            parse_embedding_endpoint("#model=bge")


class TestOpenAIEmbeddingClient(unittest.TestCase):
    def test_sends_model_input_and_timeout(self) -> None:
        endpoint = parse_embedding_endpoint("http://host:9#model=bge&timeoutMs=250")
        client = OpenAIEmbeddingClient(endpoint)
        response = MagicMock()
        response.json.return_value = {
            "data": [
                {"index": 0, "embedding": [1.0, 2.0]},
                {"index": 1, "embedding": [3.0, 4.0]},
            ]
        }
        with patch(
            "comfyui.danbooru_embedding.requests.post", return_value=response
        ) as post:
            vectors = client.embed(["a", "b"])
        self.assertEqual(vectors, [[1.0, 2.0], [3.0, 4.0]])
        kwargs = cast(Any, post.call_args).kwargs
        self.assertEqual(kwargs["json"], {"model": "bge", "input": ["a", "b"]})
        self.assertEqual(kwargs["timeout"], 0.25)

    def test_empty_api_key_sends_no_authorization_header(self) -> None:
        client = OpenAIEmbeddingClient(
            parse_embedding_endpoint("http://host:9#apiKey=")
        )
        response = MagicMock()
        response.json.return_value = {"data": [{"index": 0, "embedding": [1.0]}]}
        with patch(
            "comfyui.danbooru_embedding.requests.post", return_value=response
        ) as post:
            client.embed(["a"])
        headers = cast(Any, post.call_args).kwargs["headers"]
        self.assertNotIn("Authorization", headers)

    def test_api_key_sends_bearer_authorization_header(self) -> None:
        client = OpenAIEmbeddingClient(
            parse_embedding_endpoint("http://host:9#apiKey=abc")
        )
        response = MagicMock()
        response.json.return_value = {"data": [{"index": 0, "embedding": [1.0]}]}
        with patch(
            "comfyui.danbooru_embedding.requests.post", return_value=response
        ) as post:
            client.embed(["a"])
        headers = cast(Any, post.call_args).kwargs["headers"]
        self.assertEqual(headers["Authorization"], "Bearer abc")

    def test_empty_input_skips_request(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        with patch("comfyui.danbooru_embedding.requests.post") as post:
            self.assertEqual(client.embed([]), [])
        post.assert_not_called()

    def test_request_exception_wrapped(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        with patch(
            "comfyui.danbooru_embedding.requests.post",
            side_effect=requests.Timeout("timed out"),
        ):
            with self.assertRaises(EmbeddingError) as ctx:
                client.embed(["a"])
        self.assertIn("timed out", str(ctx.exception))

    def test_http_error_wrapped(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        response = MagicMock()
        response.raise_for_status.side_effect = requests.HTTPError("500")
        with patch("comfyui.danbooru_embedding.requests.post", return_value=response):
            with self.assertRaises(EmbeddingError):
                client.embed(["a"])

    def test_response_count_mismatch_rejected(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        response = MagicMock()
        response.json.return_value = {"data": [{"index": 0, "embedding": [1.0]}]}
        with patch("comfyui.danbooru_embedding.requests.post", return_value=response):
            with self.assertRaises(EmbeddingError) as ctx:
                client.embed(["a", "b"])
        self.assertIn("条目数", str(ctx.exception))

    def test_missing_index_falls_back_to_position(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        response = MagicMock()
        response.json.return_value = {
            "data": [{"embedding": [1.0]}, {"embedding": [2.0]}]
        }
        with patch("comfyui.danbooru_embedding.requests.post", return_value=response):
            self.assertEqual(client.embed(["a", "b"]), [[1.0], [2.0]])

    def test_empty_vector_rejected(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        response = MagicMock()
        response.json.return_value = {"data": [{"index": 0, "embedding": []}]}
        with patch("comfyui.danbooru_embedding.requests.post", return_value=response):
            with self.assertRaises(EmbeddingError):
                client.embed(["a"])

    def test_non_numeric_vector_rejected(self) -> None:
        client = OpenAIEmbeddingClient(parse_embedding_endpoint("http://host:9"))
        response = MagicMock()
        response.json.return_value = {"data": [{"index": 0, "embedding": ["x"]}]}
        with patch("comfyui.danbooru_embedding.requests.post", return_value=response):
            with self.assertRaises(EmbeddingError):
                client.embed(["a"])


class TestEmbeddingArtifact(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = Path(tempfile.mkdtemp())

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def _matrix(self, n_rows: int, dim: int) -> TagEmbeddingMatrix:
        return TagEmbeddingMatrix(
            tag=np.arange(n_rows * dim, dtype=np.float32).reshape(n_rows, dim),
            cn_name=np.ones((n_rows, dim), dtype=np.float32),
            wiki=np.full((n_rows, dim), 2.0, dtype=np.float32),
        )

    def test_roundtrip_preserves_rows_and_views(self) -> None:
        source = self._matrix(3, 4)
        path = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME
        write_compiled_embeddings(path, source)
        loaded = load_compiled_embeddings(path)
        self.assertEqual(loaded.n_rows, 3)
        self.assertEqual(loaded.dim, 4)
        np.testing.assert_allclose(np.asarray(loaded.tag), source.tag)
        np.testing.assert_allclose(np.asarray(loaded.cn_name), source.cn_name)
        np.testing.assert_allclose(np.asarray(loaded.wiki), source.wiki)

    def test_loaded_matrices_are_memmapped(self) -> None:
        path = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME
        write_compiled_embeddings(path, self._matrix(2, 4))
        loaded = load_compiled_embeddings(path)
        for _, view in loaded.views():
            self.assertIsInstance(view, np.memmap)
            self.assertEqual(cast(Any, view).mode, "r")

    def test_bad_magic_rejected(self) -> None:
        path = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME
        write_compiled_embeddings(path, self._matrix(2, 4))
        data = bytearray(path.read_bytes())
        data[0:4] = b"XXXX"
        path.write_bytes(bytes(data))
        with self.assertRaises(ValueError):
            load_compiled_embeddings(path)

    def test_truncated_file_rejected(self) -> None:
        path = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME
        write_compiled_embeddings(path, self._matrix(2, 4))
        data = path.read_bytes()
        path.write_bytes(data[:-4])
        with self.assertRaises(ValueError):
            load_compiled_embeddings(path)

    def test_compile_source_batches_three_views_and_normalizes(self) -> None:
        client = RecordingEmbeddingClient(vector_for=_VECTOR_TEXT_LENGTH)
        source = OpenAITagEmbeddingSource(client, batch_size=2)
        matrix = source.build(
            [("1girl", "一个女孩", "A girl."), ("solo", "单人", "Solo.")]
        )
        self.assertEqual(matrix.n_rows, 2)
        self.assertEqual(matrix.dim, 4)
        # 三视图各自串行分批：每视图 2 行 / batch=2 → 共 3 批
        self.assertEqual([len(batch) for batch in client.batches], [2, 2, 2])
        self.assertEqual(client.batches[0], ["1girl", "solo"])
        self.assertEqual(client.batches[1], ["一个女孩", "单人"])
        self.assertEqual(client.batches[2], ["A girl.", "Solo."])
        for _, view in matrix.views():
            norms = np.linalg.norm(view, axis=1)
            np.testing.assert_allclose(norms, 1.0, rtol=1e-6)

    def test_compile_source_propagates_embedding_error(self) -> None:
        source = OpenAITagEmbeddingSource(FailingEmbeddingClient(), batch_size=2)
        with self.assertRaises(EmbeddingError):
            source.build([("1girl", "一个女孩", "A girl.")])

    def test_compile_source_rejects_empty_rows(self) -> None:
        source = OpenAITagEmbeddingSource(RecordingEmbeddingClient())
        with self.assertRaises(ValueError):
            source.build([])


class TestVectorSemanticTagSearcher(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = Path(tempfile.mkdtemp())
        self.path = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def _write(self, matrix: TagEmbeddingMatrix) -> None:
        write_compiled_embeddings(self.path, matrix)

    def _searcher(self, client: EmbeddingClient) -> VectorSemanticTagSearcher:
        matrix = TagEmbeddingMatrix(
            tag=_unit_vectors(4, 4),
            cn_name=_unit_vectors(4, 4),
            wiki=_unit_vectors(4, 4),
        )
        return VectorSemanticTagSearcher(matrix, client)

    def test_query_vector_uses_cache_and_skips_second_call(self) -> None:
        client = RecordingEmbeddingClient(vector_for=_VECTOR_HALF_ALWAYS)
        searcher = self._searcher(client)
        searcher.search("blue hair", 4)
        searcher.search("blue hair", 4)
        self.assertEqual(len(client.batches), 1)

    def test_different_queries_each_call_interface(self) -> None:
        client = RecordingEmbeddingClient(vector_for=_VECTOR_HALF_ALWAYS)
        searcher = self._searcher(client)
        searcher.search("blue hair", 4)
        searcher.search("red hair", 4)
        self.assertEqual(len(client.batches), 2)

    def test_cosine_ranking_uses_max_across_views(self) -> None:
        # tag 视图第 3 行与查询同向；cn_name 视图第 1 行更接近
        client = RecordingEmbeddingClient(vector_for=_VECTOR_SECOND_ALWAYS)
        matrix = TagEmbeddingMatrix(
            tag=_unit_vectors(4, 4),
            cn_name=np.array(
                [
                    [1.0, 0.0, 0.0, 0.0],
                    [0.0, 1.0, 0.0, 0.0],
                    [0.0, 0.0, 1.0, 0.0],
                    [0.0, 0.0, 0.0, 1.0],
                ],
                dtype=np.float32,
            ),
            wiki=_unit_vectors(4, 4),
        )
        hits = VectorSemanticTagSearcher(matrix, client).search("q", 4)
        self.assertEqual([hit.rec_id for hit in hits], [1])
        self.assertAlmostEqual(hits[0].score, 1.0, places=5)

    def test_low_cosine_candidates_dropped(self) -> None:
        # 所有标签向量同为 e1，查询指向 e2 → 余弦 0，低于候选下限
        rows = np.tile(np.array([[1.0, 0.0, 0.0, 0.0]], dtype=np.float32), (4, 1))
        client = RecordingEmbeddingClient(vector_for=_VECTOR_SECOND_ALWAYS)
        searcher = VectorSemanticTagSearcher(
            TagEmbeddingMatrix(tag=rows, cn_name=rows, wiki=rows), client
        )
        self.assertEqual(searcher.search("q", 4), [])

    def test_top_k_limits_results_per_view(self) -> None:
        client = RecordingEmbeddingClient(vector_for=_VECTOR_DIAGONAL_ALWAYS)
        searcher = self._searcher(client)
        hits = searcher.search("q", 1)
        # 每视图只取 top1；三视图 top1 同为第 0 行 → 去重后一条
        self.assertEqual([hit.rec_id for hit in hits], [0])

    def test_error_propagates_to_caller(self) -> None:
        client = FailingEmbeddingClient("嵌入接口不可用: 超时")
        searcher = self._searcher(client)
        with self.assertRaises(EmbeddingError) as ctx:
            searcher.search("q", 4)
        self.assertIn("超时", str(ctx.exception))

    def test_loaded_mapping_stays_held_after_search(self) -> None:
        """映射常驻以避免逐查询缺页开销；代价是产物文件被占用，编译需先退出补全进程。"""
        self._write(
            TagEmbeddingMatrix(
                tag=_unit_vectors(4, 4),
                cn_name=_unit_vectors(4, 4),
                wiki=_unit_vectors(4, 4),
            )
        )
        client = RecordingEmbeddingClient(vector_for=_VECTOR_HALF_ALWAYS)
        searcher = VectorSemanticTagSearcher(
            load_compiled_embeddings(self.path), client
        )
        searcher.search("q", 4)
        # Windows 上 mmap 常驻时覆盖写失败（Errno 22）；非 Windows 平台可正常覆盖
        if os.name == "nt":
            with self.assertRaises(OSError):
                with self.path.open("wb") as f:
                    f.write(b"\x00" * 16)

    def test_score_range_constants_ordered(self) -> None:
        self.assertLess(MIN_COSINE, CEILING_COSINE)


class _StubEmbeddingHandler(BaseHTTPRequestHandler):
    """最小 OpenAI 兼容 /v1/embeddings 桩服务。"""

    protocol_version = "HTTP/1.1"
    requests_seen: List[Tuple[Dict[str, str], List[str]]] = []
    lock = threading.Lock()

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler 约定命名
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        inputs = cast(List[str], payload.get("input", []))
        with _StubEmbeddingHandler.lock:
            _StubEmbeddingHandler.requests_seen.append(
                (dict(self.headers), list(inputs))
            )
        body = json.dumps(
            {
                "object": "list",
                "data": [
                    {
                        "object": "embedding",
                        "index": i,
                        "embedding": [1.0, 0.0, 0.0, 0.0],
                    }
                    for i in range(len(inputs))
                ],
            }
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:
        return


@contextmanager
def stub_embedding_server() -> Generator[str, None, None]:
    server = ThreadingHTTPServer(("127.0.0.1", 0), _StubEmbeddingHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        host, port = cast(Tuple[str, int], server.server_address[:2])
        yield f"http://{host}:{port}#apiKey=&model=stub&timeoutMs=5000"
    finally:
        server.shutdown()
        server.server_close()


class TestSharedSemanticSearcher(unittest.TestCase):
    def setUp(self) -> None:
        # 每个用例独占一个临时目录，模块缓存按目录路径分键互不干扰
        self.tmp_dir = Path(tempfile.mkdtemp())

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def _write_matrix(self) -> None:
        write_compiled_embeddings(
            self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME,
            TagEmbeddingMatrix(
                tag=_unit_vectors(4, 4),
                cn_name=_unit_vectors(4, 4),
                wiki=_unit_vectors(4, 4),
            ),
        )

    def _searcher(self, url: str) -> VectorSemanticTagSearcher:
        endpoint = parse_embedding_endpoint(url)
        return cached_semantic_searcher(
            str(self.tmp_dir), endpoint, OpenAIEmbeddingClient(endpoint)
        )

    def test_same_dir_and_endpoint_reuses_instance(self) -> None:
        self._write_matrix()
        url = "http://127.0.0.1:1#model=stub"
        first = self._searcher(url)
        second = self._searcher(url)
        self.assertIsInstance(first, VectorSemanticTagSearcher)
        # 同 (目录, 端点) 复用同一实例，查询向量 LRU 才能跨请求命中
        self.assertIs(first, second)

    def test_different_endpoint_yields_separate_instance(self) -> None:
        self._write_matrix()
        first = self._searcher("http://127.0.0.1:1#model=stub")
        second = self._searcher("http://127.0.0.1:1#model=other")
        self.assertIsNot(first, second)

    def test_missing_artifact_fails_fast(self) -> None:
        """激活判定已由入口完成，映射缺失属产物损坏：直接报错而非静默空结果。"""
        with self.assertRaises(OSError):
            self._searcher("http://127.0.0.1:1#model=stub")

    def test_vector_searcher_round_trips_through_real_http_endpoint(self) -> None:
        self._write_matrix()
        _StubEmbeddingHandler.requests_seen.clear()
        with stub_embedding_server() as url:
            searcher = self._searcher(url)
            hits = searcher.search("q", 4)
            searcher.search("q", 4)
        self.assertEqual([hit.rec_id for hit in hits], [0])
        # 第二次同查询命中 LRU，未再发出请求
        self.assertEqual(len(_StubEmbeddingHandler.requests_seen), 1)
        headers, inputs = _StubEmbeddingHandler.requests_seen[0]
        self.assertEqual(inputs, ["q"])
        self.assertNotIn("Authorization", headers)


if __name__ == "__main__":
    unittest.main()
