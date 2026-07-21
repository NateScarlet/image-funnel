# -*- coding: utf-8 -*-
import os
import sqlite3
from contextlib import contextmanager
from types import TracebackType
from typing import Generator, Optional

_MIGRATIONS: list[str] = [
    """
    CREATE TABLE IF NOT EXISTS history (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        command    TEXT NOT NULL,
        data       TEXT NOT NULL,
        created_at TEXT NOT NULL DEFAULT (datetime('now'))
    );
    """,
    """
    CREATE INDEX IF NOT EXISTS idx_history_command ON history(command);
    """,
    """
    CREATE TABLE IF NOT EXISTS danbooru_search_cache (
        query      TEXT PRIMARY KEY,
        results    TEXT NOT NULL,
        updated_at INTEGER NOT NULL
    );
    """,
    """
    CREATE TABLE IF NOT EXISTS danbooru_related_cache (
        tags       TEXT PRIMARY KEY,
        results    TEXT NOT NULL,
        updated_at INTEGER NOT NULL
    );
    """,
    """
    CREATE TABLE IF NOT EXISTS danbooru_tag_cache (
        tag        TEXT PRIMARY KEY,
        cn_name    TEXT NOT NULL,
        wiki       TEXT NOT NULL,
        category   TEXT NOT NULL,
        updated_at INTEGER NOT NULL
    );
    """,
]


class SQLiteContext:
    """SQLite 数据库连接上下文管理器，支持延迟连接、自动迁移和事务控制。"""

    def __init__(self, db_path: str) -> None:
        self.db_path = db_path
        self._conn: Optional[sqlite3.Connection] = None

    @staticmethod
    def from_env() -> "SQLiteContext":
        """从环境变量构造 SQLiteContext 实例。"""
        db_path = os.path.join(
            os.environ["IMAGE_FUNNEL_ROOT_DIR"],
            os.environ["IMAGE_FUNNEL_DIRECTORY_REL_PATH"],
            ".io.github.natescarlet.hook.db",
        )
        return SQLiteContext(db_path)

    def __enter__(self) -> "SQLiteContext":
        return self

    @property
    def connection(self) -> sqlite3.Connection:
        """延迟获取 SQLite 连接。在第一次获取时建立连接并执行数据库初始化/迁移。"""
        if self._conn is None:
            if self.db_path != ":memory:":
                os.makedirs(os.path.dirname(self.db_path), exist_ok=True)
            self._conn = sqlite3.connect(self.db_path, check_same_thread=False)
            for sql in _MIGRATIONS:
                self._conn.execute(sql)
            self._conn.commit()
        return self._conn

    @property
    def is_connected(self) -> bool:
        """当前是否已建立物理连接。"""
        return self._conn is not None

    def close(self) -> None:
        """关闭物理数据库连接并清理资源。"""
        if self._conn is not None:
            self._conn.close()
            self._conn = None

    def __exit__(
        self,
        exc_type: Optional[type[BaseException]],
        exc_val: Optional[BaseException],
        exc_tb: Optional[TracebackType],
    ) -> None:
        """退出连接 Scope，关闭连接释放资源。"""
        self.close()

    @contextmanager
    def transaction(self) -> Generator[sqlite3.Connection, None, None]:
        """开启一个显式数据库事务。在事务块退出时 commit，若发生异常则 rollback。"""
        conn = self.connection
        conn.execute("BEGIN TRANSACTION")
        try:
            yield conn
        except Exception:
            conn.rollback()
            raise
        else:
            conn.commit()
