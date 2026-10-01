# -*- coding: utf-8 -*-
"""编译管线：源识别、产物写出/加载、环境目录解析。"""

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
from typing import Any, Generator, List, Optional, Sequence, Tuple, cast
from unittest.mock import patch

from .danbooru_data import (
    COMPILED_COOC_FILENAME,
    COMPILED_TAGS_FILENAME,
    COMPILE_SCRIPT_HINT,
    SOURCE_DOWNLOAD_HINT,
    compile_dataset,
    default_compile_data_dir,
    find_cooc_source,
    find_tags_source,
    has_compiled_dataset,
    load_compiled_cooc,
    load_compiled_tags,
    resolve_danbooru_data_dir,
    resolve_image_funnel_data_dir,
)
from .danbooru_embedding import (
    COMPILED_EMBEDDINGS_FILENAME,
    EMBEDDING_PROVIDER_URL_ENV,
    EmbeddingError,
    NullTagEmbeddingSource,
    TagEmbeddingMatrix,
)
from .danbooru_test import FakeTagEmbeddingSource, write_file_fixture


class TestFindSource(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = Path(tempfile.mkdtemp())

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def test_preferred_names_win(self) -> None:
        write_file_fixture(
            str(self.tmp_dir),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        # 同目录再放一个 schema 合法的变体名，精确名应优先
        (self.tmp_dir / "my_tags.csv").write_text(
            "name,cn_name\nsolo,单人\n", encoding="utf-8"
        )
        tags_src = find_tags_source(self.tmp_dir)
        assert tags_src is not None
        self.assertEqual(tags_src.name, "tags_enhanced.csv")

        cooc_src = find_cooc_source(self.tmp_dir)
        assert cooc_src is not None
        self.assertEqual(cooc_src.name, "cooccurrence_clean.parquet")

    def test_scan_renamed_inputs_by_schema(self) -> None:
        write_file_fixture(
            str(self.tmp_dir),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        # 重命名精确名 → 进入 schema 扫描
        (self.tmp_dir / "tags_enhanced.csv").rename(self.tmp_dir / "export_tags.csv")
        (self.tmp_dir / "cooccurrence_clean.parquet").rename(
            self.tmp_dir / "pairs_v2.parquet"
        )
        tags_src = find_tags_source(self.tmp_dir)
        assert tags_src is not None
        self.assertEqual(tags_src.name, "export_tags.csv")
        cooc_src = find_cooc_source(self.tmp_dir)
        assert cooc_src is not None
        self.assertEqual(cooc_src.name, "pairs_v2.parquet")

    def test_missing_sources_returns_none(self) -> None:
        self.assertIsNone(find_tags_source(self.tmp_dir))
        self.assertIsNone(find_cooc_source(self.tmp_dir))
        self.assertIsNone(find_tags_source(self.tmp_dir / "no-such"))


class TestCompileDataset(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = Path(tempfile.mkdtemp())

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp_dir, ignore_errors=True)

    def test_compile_missing_sources_raises_with_download_hint(self) -> None:
        out = self.tmp_dir / "out"
        with self.assertRaises(FileNotFoundError) as ctx:
            compile_dataset(self.tmp_dir, out, FakeTagEmbeddingSource())
        msg = str(ctx.exception)
        self.assertIn("github.com", msg)
        self.assertIn("huggingface.co", msg)
        self.assertIn("modelscope.cn", msg)

    def test_compile_missing_dir_raises(self) -> None:
        with self.assertRaises(FileNotFoundError) as ctx:
            compile_dataset(
                self.tmp_dir / "no-such", self.tmp_dir / "out", FakeTagEmbeddingSource()
            )
        self.assertIn("github.com", str(ctx.exception))

    def test_embedding_failure_aborts_before_writing_artifacts(self) -> None:
        """嵌入接口不可用时快速失败：tags/cooc/embeddings 三个产物都不落盘。"""
        source = self.tmp_dir / "source"
        output = self.tmp_dir / "out"
        source.mkdir()
        write_file_fixture(
            str(source),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "A girl.",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        (source / COMPILED_TAGS_FILENAME).unlink()
        (source / COMPILED_COOC_FILENAME).unlink()
        (source / COMPILED_EMBEDDINGS_FILENAME).unlink()

        class FailingEmbeddingSource:
            def build(
                self, views: Sequence[Tuple[str, str, str]]
            ) -> Optional[TagEmbeddingMatrix]:
                raise EmbeddingError("嵌入接口不可达")

        with self.assertRaises(EmbeddingError):
            compile_dataset(source, output, cast(Any, FailingEmbeddingSource()))
        self.assertFalse((output / COMPILED_TAGS_FILENAME).exists())
        self.assertFalse((output / COMPILED_COOC_FILENAME).exists())
        self.assertFalse((output / COMPILED_EMBEDDINGS_FILENAME).exists())

    def test_null_embedding_source_compiles_literal_only(self) -> None:
        """--no-embedding：只编译字面匹配产物，且不产出向量（仍满足 has_compiled_dataset）。"""
        source = self.tmp_dir / "source"
        source.mkdir()
        write_file_fixture(
            str(source),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "A girl.",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        # 只留源数据，清掉夹具顺手编译出的产物
        for name in (
            COMPILED_TAGS_FILENAME,
            COMPILED_COOC_FILENAME,
            COMPILED_EMBEDDINGS_FILENAME,
        ):
            (source / name).unlink()

        output = self.tmp_dir / "out"
        compiled = compile_dataset(source, output, NullTagEmbeddingSource())
        self.assertIsNone(compiled.embeddings)
        self.assertTrue(compiled.tags.is_file())
        self.assertTrue(compiled.cooc.is_file())
        self.assertFalse((output / COMPILED_EMBEDDINGS_FILENAME).exists())
        # 字面链路照常可用：补全入口据此启用本地 provider
        self.assertTrue(has_compiled_dataset(output))

    def test_null_embedding_source_removes_stale_vectors(self) -> None:
        """显式跳过向量时清掉旧向量：留着会与新标签表行序错位，静默给出错误召回。"""
        write_file_fixture(
            str(self.tmp_dir),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "A girl.",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        stale = self.tmp_dir / COMPILED_EMBEDDINGS_FILENAME
        self.assertTrue(stale.is_file())

        compiled = compile_dataset(self.tmp_dir, self.tmp_dir, NullTagEmbeddingSource())
        self.assertIsNone(compiled.embeddings)
        self.assertFalse(stale.exists())
        self.assertTrue(has_compiled_dataset(self.tmp_dir))

    def test_source_and_output_are_separate(self) -> None:
        """源目录只放源数据；产物只写入 output_dir，不污染源目录。"""
        source = self.tmp_dir / "source"
        output = self.tmp_dir / "out"
        source.mkdir()
        write_file_fixture(
            str(source),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩",
                    "wiki": "",
                    "post_count": "10",
                    "category": "0",
                    "nsfw": "0",
                }
            ],
            [],
        )
        # write_file_fixture 会在 source 内编译；清掉产物后单独验证分离路径
        (source / COMPILED_TAGS_FILENAME).unlink()
        (source / COMPILED_COOC_FILENAME).unlink()
        (source / COMPILED_EMBEDDINGS_FILENAME).unlink()

        compiled = compile_dataset(source, output, FakeTagEmbeddingSource())
        self.assertTrue(compiled.tags.is_file())
        self.assertTrue(compiled.cooc.is_file())
        assert compiled.embeddings is not None
        self.assertTrue(compiled.embeddings.is_file())
        self.assertTrue(has_compiled_dataset(output))
        # 源目录不写产物
        self.assertFalse((source / COMPILED_TAGS_FILENAME).is_file())
        self.assertFalse((source / COMPILED_COOC_FILENAME).is_file())

    def test_roundtrip_tags_and_cooc(self) -> None:
        write_file_fixture(
            str(self.tmp_dir),
            [
                {
                    "name": "1girl",
                    "cn_name": "一个女孩,女孩",
                    "wiki": "A girl.",
                    "post_count": "100",
                    "category": "0",
                    "nsfw": "0",
                },
                {
                    "name": "white_hair",
                    "cn_name": "白发",
                    "wiki": "White hair.",
                    "post_count": "50",
                    "category": "0",
                    "nsfw": "0",
                },
            ],
            [("1girl", "white_hair", 10)],
        )
        tags_path = self.tmp_dir / COMPILED_TAGS_FILENAME
        cooc_path = self.tmp_dir / COMPILED_COOC_FILENAME
        self.assertTrue(tags_path.is_file())
        self.assertTrue(cooc_path.is_file())
        self.assertTrue(has_compiled_dataset(self.tmp_dir))

        records = load_compiled_tags(tags_path)
        self.assertEqual([r.tag for r in records], ["1girl", "white_hair"])
        # rec_id 即 CSR 节点下标
        self.assertEqual([r.rec_id for r in records], [0, 1])
        self.assertEqual(records[0].category, "General")
        self.assertEqual(records[0].name_norm, "1girl")
        self.assertEqual(records[0].cn_aliases_norm, ("一个女孩", "女孩"))

        index = load_compiled_cooc(cooc_path)
        self.assertEqual(index.n_nodes, 2)
        # 双向 CSR：每端各一条边
        self.assertEqual(index.n_edges, 2)
        scores = index.aggregate({0})
        self.assertEqual(scores, {1: 10})
        # 种子互斥
        self.assertEqual(index.aggregate({0, 1}), {})

    def test_gbk_csv_compiles(self) -> None:
        write_file_fixture(
            str(self.tmp_dir),
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
        records = load_compiled_tags(self.tmp_dir / COMPILED_TAGS_FILENAME)
        self.assertEqual(records[0].cn_name, "一个女孩")


class TestResolveDirs(unittest.TestCase):
    def test_resolve_image_funnel_env(self) -> None:
        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": "/custom/root"}):
            self.assertEqual(resolve_image_funnel_data_dir(), "/custom/root")

    def test_resolve_image_funnel_fallback_user_config(self) -> None:
        env = {
            k: v
            for k, v in os.environ.items()
            if k not in ("IMAGE_FUNNEL_DATA_DIR", "DANBOORU_DATA_DIR")
        }
        env["APPDATA"] = r"C:\Users\test\AppData\Roaming"
        with patch.dict(os.environ, env, clear=True):
            root = resolve_image_funnel_data_dir()
            self.assertEqual(
                root,
                os.path.join(
                    r"C:\Users\test\AppData\Roaming",
                    "io.github.natescarlet.image-funnel",
                ),
            )

    def test_default_compile_data_dir_joins_danbooru(self) -> None:
        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": "/data/if"}):
            self.assertEqual(
                default_compile_data_dir(), os.path.join("/data/if", "danbooru")
            )

    def test_resolve_danbooru_explicit_wins(self) -> None:
        with patch.dict(
            os.environ,
            {"DANBOORU_DATA_DIR": "/explicit", "IMAGE_FUNNEL_DATA_DIR": "/base"},
        ):
            self.assertEqual(resolve_danbooru_data_dir(), "/explicit")

    def test_resolve_danbooru_default_requires_compiled_artifacts(self) -> None:
        with tempfile.TemporaryDirectory() as base:
            default_dir = os.path.join(base, "danbooru")
            env = {"DANBOORU_DATA_DIR": "", "IMAGE_FUNNEL_DATA_DIR": base}
            with patch.dict(os.environ, env, clear=False):
                # 未编译 → 空（回落在线链路）
                self.assertEqual(resolve_danbooru_data_dir(), "")
                os.makedirs(default_dir, exist_ok=True)
                self.assertEqual(resolve_danbooru_data_dir(), "")

                write_file_fixture(
                    default_dir,
                    [
                        {
                            "name": "1girl",
                            "cn_name": "一个女孩",
                            "wiki": "",
                            "post_count": "10",
                            "category": "0",
                            "nsfw": "0",
                        }
                    ],
                    [],
                )
                self.assertEqual(resolve_danbooru_data_dir(), default_dir)

    def test_resolve_danbooru_no_base_returns_empty(self) -> None:
        with patch.dict(
            os.environ, {"DANBOORU_DATA_DIR": "", "IMAGE_FUNNEL_DATA_DIR": ""}
        ):
            self.assertEqual(resolve_danbooru_data_dir(), "")

    def test_compile_script_hint_points_to_script(self) -> None:
        self.assertIn("compile_danbooru_data.py", COMPILE_SCRIPT_HINT)

    def test_download_hint_only_for_compile(self) -> None:
        self.assertIn("github.com", SOURCE_DOWNLOAD_HINT)


class _StubEmbeddingHandler(BaseHTTPRequestHandler):
    """最小 OpenAI 兼容 /v1/embeddings 桩服务，供编译脚本端到端用例使用。"""

    protocol_version = "HTTP/1.1"

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler 约定命名
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        inputs = cast(List[str], payload.get("input", []))
        body = json.dumps(
            {
                "object": "list",
                "data": [
                    {
                        "object": "embedding",
                        "index": i,
                        "embedding": [float(len(text) % 5) + 1.0] * 4,
                    }
                    for i, text in enumerate(inputs)
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
    """启动桩嵌入服务并产出 `<base>#apiKey=&model=stub&timeoutMs=5000` 端点。"""
    server = ThreadingHTTPServer(("127.0.0.1", 0), _StubEmbeddingHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        host, port = cast(Tuple[str, int], server.server_address[:2])
        yield f"http://{host}:{port}#apiKey=&model=stub&timeoutMs=5000"
    finally:
        server.shutdown()
        server.server_close()


@contextmanager
def minimal_source_dir(root: Path) -> Generator[Path, None, None]:
    """在 root 下写入最小源数据（一条 tags + 空共现 parquet）。"""
    import csv as csv_mod
    import pyarrow as pa  # pyright: ignore[reportMissingTypeStubs]
    import pyarrow.parquet as pq  # pyright: ignore[reportMissingTypeStubs]

    pa_mod = cast(Any, pa)
    pq_mod = cast(Any, pq)
    root.mkdir()
    with open(root / "tags_enhanced.csv", "w", encoding="utf-8", newline="") as f:
        writer = csv_mod.DictWriter(
            f,
            fieldnames=["name", "cn_name", "wiki", "post_count", "category", "nsfw"],
        )
        writer.writeheader()
        writer.writerow(
            {
                "name": "1girl",
                "cn_name": "一个女孩",
                "wiki": "",
                "post_count": "10",
                "category": "0",
                "nsfw": "0",
            }
        )
    table = pa_mod.table(
        {
            "tag_a": pa_mod.array([], type=pa_mod.string()),
            "tag_b": pa_mod.array([], type=pa_mod.string()),
            "count": pa_mod.array([], type=pa_mod.int32()),
        }
    )
    pq_mod.write_table(table, root / "cooccurrence_clean.parquet")
    yield root


class TestCompileScriptMain(unittest.TestCase):
    """compile_danbooru_data.py 入口：源目录必填、产物写入补全默认路径。"""

    def setUp(self) -> None:
        self.script = Path(__file__).resolve().parents[1] / "compile_danbooru_data.py"

    def _run(self, args: list[str], env: dict[str, str]) -> Any:
        import subprocess
        import sys

        return cast(
            Any,
            subprocess.run(
                [sys.executable, str(self.script), *args],
                capture_output=True,
                encoding="utf-8",
                errors="replace",
                env=env,
                timeout=180,
            ),
        )

    def test_missing_source_exits_nonzero_with_download_urls(self) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            proc = self._run([source_dir], dict(os.environ))
        self.assertNotEqual(proc.returncode, 0)
        err = cast(str, proc.stderr) or ""
        self.assertIn("github.com", err)
        self.assertIn("huggingface.co", err)

    def test_missing_source_dir_argument_exits_nonzero(self) -> None:
        proc = self._run([], dict(os.environ))
        self.assertNotEqual(proc.returncode, 0)

    def test_output_goes_to_image_funnel_default_not_source(self) -> None:
        """参数是源目录；产物写入 ${IMAGE_FUNNEL_DATA_DIR}/danbooru。"""
        with tempfile.TemporaryDirectory() as tmp:
            source_dir = Path(tmp) / "source"
            env = dict(os.environ)
            image_funnel_root = str(Path(tmp) / "if-root")
            env["IMAGE_FUNNEL_DATA_DIR"] = image_funnel_root
            env.pop("DANBOORU_DATA_DIR", None)

            with minimal_source_dir(source_dir), stub_embedding_server() as url:
                proc = self._run(
                    [str(source_dir), "--embedding-provider-url", url], env
                )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            out_dir = Path(image_funnel_root) / "danbooru"
            self.assertTrue((out_dir / COMPILED_TAGS_FILENAME).is_file())
            self.assertTrue((out_dir / COMPILED_COOC_FILENAME).is_file())
            self.assertTrue((out_dir / COMPILED_EMBEDDINGS_FILENAME).is_file())
            # 源目录不写产物
            self.assertFalse((source_dir / COMPILED_TAGS_FILENAME).is_file())

    def test_unreachable_embedding_endpoint_fails_without_artifacts(self) -> None:
        """嵌入端点不可达时快速失败中止编译，不产出半成品，并给出两条可选出路。"""
        with tempfile.TemporaryDirectory() as tmp:
            source_dir = Path(tmp) / "source"
            env = dict(os.environ)
            image_funnel_root = str(Path(tmp) / "if-root")
            env["IMAGE_FUNNEL_DATA_DIR"] = image_funnel_root
            env.pop("DANBOORU_DATA_DIR", None)

            with minimal_source_dir(source_dir):
                # 端口 1 必然拒绝连接，快速失败而非长时间挂起
                proc = self._run(
                    [
                        str(source_dir),
                        "--embedding-provider-url",
                        "http://127.0.0.1:1#apiKey=&model=stub&timeoutMs=300",
                    ],
                    env,
                )
            self.assertNotEqual(proc.returncode, 0)
            err = cast(str, proc.stderr) or ""
            # 失败原因 + 两条出路（配置服务 / --no-embedding）
            self.assertIn("标签向量生成失败", err)
            self.assertIn(EMBEDDING_PROVIDER_URL_ENV, err)
            self.assertIn("--no-embedding", err)
            out_dir = Path(image_funnel_root) / "danbooru"
            self.assertFalse((out_dir / COMPILED_TAGS_FILENAME).exists())
            self.assertFalse((out_dir / COMPILED_EMBEDDINGS_FILENAME).exists())

    def test_no_embedding_compiles_literal_dataset_without_endpoint(self) -> None:
        """--no-embedding：没有嵌入服务也能编译出字面匹配产物，且提示语义层仍需环境变量。"""
        with tempfile.TemporaryDirectory() as tmp:
            source_dir = Path(tmp) / "source"
            env = dict(os.environ)
            image_funnel_root = str(Path(tmp) / "if-root")
            env["IMAGE_FUNNEL_DATA_DIR"] = image_funnel_root
            env.pop("DANBOORU_DATA_DIR", None)
            # 端点必然不可达：--no-embedding 下不应被访问
            env[EMBEDDING_PROVIDER_URL_ENV] = "http://127.0.0.1:1#model=stub"

            with minimal_source_dir(source_dir):
                proc = self._run([str(source_dir), "--no-embedding"], env)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            out_dir = Path(image_funnel_root) / "danbooru"
            self.assertTrue((out_dir / COMPILED_TAGS_FILENAME).is_file())
            self.assertTrue((out_dir / COMPILED_COOC_FILENAME).is_file())
            self.assertFalse((out_dir / COMPILED_EMBEDDINGS_FILENAME).exists())
            # 字面链路可用
            self.assertTrue(has_compiled_dataset(out_dir))
            out = cast(str, proc.stdout) or ""
            self.assertIn("--no-embedding", out)
            self.assertIn("仅字面匹配", out)
            self.assertNotIn(EMBEDDING_PROVIDER_URL_ENV, out)

    def test_successful_compile_reports_semantic_layer_needs_env(self) -> None:
        """编译成功 ≠ 语义层已启用：产物齐备后仍要提示设置环境变量。"""
        with tempfile.TemporaryDirectory() as tmp:
            source_dir = Path(tmp) / "source"
            env = dict(os.environ)
            env["IMAGE_FUNNEL_DATA_DIR"] = str(Path(tmp) / "if-root")
            env.pop("DANBOORU_DATA_DIR", None)

            with minimal_source_dir(source_dir), stub_embedding_server() as url:
                proc = self._run(
                    [str(source_dir), "--embedding-provider-url", url], env
                )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            out = cast(str, proc.stdout) or ""
            self.assertIn(EMBEDDING_PROVIDER_URL_ENV, out)
            self.assertIn("无需重新编译", out)
            self.assertNotIn("--no-embedding", out)


if __name__ == "__main__":
    unittest.main()
