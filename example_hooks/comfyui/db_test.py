# -*- coding: utf-8 -*-
import os
import tempfile
import unittest
from unittest.mock import patch

from .db import SQLiteContext


class TestSQLiteContext(unittest.TestCase):

    def test_sqlite_context_lazy_connection(self) -> None:
        ctx = SQLiteContext(":memory:")
        self.assertFalse(ctx.is_connected)

        with ctx as c:
            self.assertFalse(c.is_connected)
            conn = c.connection
            self.assertTrue(c.is_connected)

            cursor = conn.cursor()
            cursor.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='history'"
            )
            self.assertIsNotNone(cursor.fetchone())

            cursor.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='danbooru_search_cache'"
            )
            self.assertIsNotNone(cursor.fetchone())

            cursor.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='danbooru_related_cache'"
            )
            self.assertIsNotNone(cursor.fetchone())

    def test_sqlite_context_transaction_commit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = os.path.join(tmp_dir, "test.db")
            ctx = SQLiteContext(db_path)
            with ctx.transaction() as conn:
                conn.execute(
                    "INSERT INTO history (command, data) VALUES (?, ?)", ("test", "{}")
                )

            # 物理连接依然保持开启
            self.assertTrue(ctx.is_connected)

            # 验证数据是否已 commit
            row = ctx.connection.execute("SELECT command FROM history").fetchone()
            self.assertEqual(row[0], "test")
            ctx.close()

    def test_sqlite_context_transaction_rollback(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = os.path.join(tmp_dir, "test.db")
            ctx = SQLiteContext(db_path)

            # 触发初始化建表
            _ = ctx.connection

            try:
                with ctx.transaction() as conn:
                    conn.execute(
                        "INSERT INTO history (command, data) VALUES (?, ?)",
                        ("test", "{}"),
                    )
                    raise RuntimeError("Force Rollback")
            except RuntimeError:
                pass

            # 验证数据是否已被回滚（即数据库为空）
            row = ctx.connection.execute("SELECT COUNT(*) FROM history").fetchone()
            self.assertEqual(row[0], 0)
            ctx.close()

    def test_sqlite_context_from_env(self) -> None:
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "/dummy/root",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "dummy_dir",
            },
        ):
            ctx = SQLiteContext.from_env()
            self.assertEqual(
                os.path.basename(ctx.db_path), ".io.github.natescarlet.hook.db"
            )
            self.assertIn("dummy_dir", ctx.db_path)
