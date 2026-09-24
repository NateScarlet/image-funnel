# -*- coding: utf-8 -*-
"""编译管线：源识别、产物写出/加载、环境目录解析。"""

from __future__ import annotations

import os
import shutil
import tempfile
import unittest
from pathlib import Path
from typing import Any, cast
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
from .danbooru_test import write_file_fixture


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
            compile_dataset(self.tmp_dir, out)
        msg = str(ctx.exception)
        self.assertIn("github.com", msg)
        self.assertIn("huggingface.co", msg)
        self.assertIn("modelscope.cn", msg)

    def test_compile_missing_dir_raises(self) -> None:
        with self.assertRaises(FileNotFoundError) as ctx:
            compile_dataset(self.tmp_dir / "no-such", self.tmp_dir / "out")
        self.assertIn("github.com", str(ctx.exception))

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

        tags_out, cooc_out = compile_dataset(source, output)
        self.assertTrue(tags_out.is_file())
        self.assertTrue(cooc_out.is_file())
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


class TestCompileScriptMain(unittest.TestCase):
    """compile_danbooru_data.py 入口：源目录必填、产物写入补全默认路径。"""

    def test_missing_source_exits_nonzero_with_download_urls(self) -> None:
        import subprocess
        import sys

        script = Path(__file__).resolve().parents[1] / "compile_danbooru_data.py"
        with tempfile.TemporaryDirectory() as source_dir:
            # Windows 控制台默认非 UTF-8；errors=replace 避免解码失败
            proc = cast(
                Any,
                subprocess.run(
                    [sys.executable, str(script), source_dir],
                    capture_output=True,
                    encoding="utf-8",
                    errors="replace",
                    timeout=60,
                ),
            )
        self.assertNotEqual(proc.returncode, 0)
        err = cast(str, proc.stderr) or ""
        self.assertIn("github.com", err)
        self.assertIn("huggingface.co", err)

    def test_missing_source_dir_argument_exits_nonzero(self) -> None:
        import subprocess
        import sys

        script = Path(__file__).resolve().parents[1] / "compile_danbooru_data.py"
        proc = cast(
            Any,
            subprocess.run(
                [sys.executable, str(script)],
                capture_output=True,
                encoding="utf-8",
                errors="replace",
                timeout=60,
            ),
        )
        self.assertNotEqual(proc.returncode, 0)

    def test_output_goes_to_image_funnel_default_not_source(self) -> None:
        """参数是源目录；产物写入 ${IMAGE_FUNNEL_DATA_DIR}/danbooru。"""
        import subprocess
        import sys

        script = Path(__file__).resolve().parents[1] / "compile_danbooru_data.py"
        with tempfile.TemporaryDirectory() as tmp:
            source_dir = Path(tmp) / "source"
            source_dir.mkdir()
            env = dict(os.environ)
            image_funnel_root = str(Path(tmp) / "if-root")
            env["IMAGE_FUNNEL_DATA_DIR"] = image_funnel_root
            env.pop("DANBOORU_DATA_DIR", None)

            # 在源目录写入最小源数据（CSV + 空 parquet）
            import csv as csv_mod
            import pyarrow as pa  # pyright: ignore[reportMissingTypeStubs]
            import pyarrow.parquet as pq  # pyright: ignore[reportMissingTypeStubs]

            pa_mod = cast(Any, pa)
            pq_mod = cast(Any, pq)
            with open(
                source_dir / "tags_enhanced.csv", "w", encoding="utf-8", newline=""
            ) as f:
                writer = csv_mod.DictWriter(
                    f,
                    fieldnames=[
                        "name",
                        "cn_name",
                        "wiki",
                        "post_count",
                        "category",
                        "nsfw",
                    ],
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
            pq_mod.write_table(table, source_dir / "cooccurrence_clean.parquet")

            proc = cast(
                Any,
                subprocess.run(
                    [sys.executable, str(script), str(source_dir)],
                    capture_output=True,
                    encoding="utf-8",
                    errors="replace",
                    env=env,
                    timeout=60,
                ),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            out_dir = Path(image_funnel_root) / "danbooru"
            self.assertTrue((out_dir / COMPILED_TAGS_FILENAME).is_file())
            self.assertTrue((out_dir / COMPILED_COOC_FILENAME).is_file())
            # 源目录不写产物
            self.assertFalse((source_dir / COMPILED_TAGS_FILENAME).is_file())
            self.assertFalse((source_dir / COMPILED_COOC_FILENAME).is_file())


if __name__ == "__main__":
    unittest.main()
