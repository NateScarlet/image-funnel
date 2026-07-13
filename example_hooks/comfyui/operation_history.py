#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""操作历史记录模块：将 comfyui 指令执行记录写入当前目录下的 SQLite 数据库。

数据库文件路径：{IMAGE_FUNNEL_ROOT_DIR}/{IMAGE_FUNNEL_DIRECTORY_REL_PATH}/.io.github.nantescarlet.hook.db
仅在 __main__.py 的执行路径中调用，autocomplete 有独立入口，不会经过此模块。
"""

import argparse
import json
import os
import sqlite3

_DB_FILENAME = ".io.github.nantescarlet.hook.db"

_CREATE_TABLE_SQL = """
CREATE TABLE IF NOT EXISTS history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    command    TEXT NOT NULL,
    data       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_history_command ON history(command);
"""


class OperationHistory:
    def __init__(self, db_path: str) -> None:
        self.db_path = db_path

    @staticmethod
    def from_env() -> "OperationHistory":
        return OperationHistory(
            os.path.join(
                os.environ["IMAGE_FUNNEL_ROOT_DIR"],
                os.environ["IMAGE_FUNNEL_DIRECTORY_REL_PATH"],
                _DB_FILENAME,
            )
        )

    def ensure_db(self) -> None:
        os.makedirs(os.path.dirname(self.db_path), exist_ok=True)
        conn = sqlite3.connect(self.db_path)
        try:
            conn.executescript(_CREATE_TABLE_SQL)
            conn.commit()
        finally:
            conn.close()

    def record(self, command: str, data: dict[str, str]) -> None:
        conn = sqlite3.connect(self.db_path)
        try:
            conn.execute(
                "INSERT INTO history (command, data) VALUES (?, ?)",
                (command, json.dumps(data, ensure_ascii=False)),
            )
            conn.commit()
        finally:
            conn.close()

    def extract_params(self, args: argparse.Namespace) -> None:
        """从 argparse 解析结果中提取参数并写入历史记录。

        add/remove 的每个 prompt 参数是独立操作，分别写入一条记录。
        """
        self.ensure_db()

        cmd = args.command

        if cmd == "queue":
            self.record("queue", {})
            return

        if cmd in ("add", "remove"):
            for prompt in args.prompt:
                self.record(cmd, {"prompt": prompt})
            return

        if cmd == "adjust":
            adjust_type: str = getattr(args, "adjust_type", "")
            data: dict[str, str] = {"adjust_type": adjust_type}
            if adjust_type == "lora":
                data["name"] = args.name
                data["weight"] = args.weight
            elif adjust_type == "prompt":
                data["text"] = args.text
                data["weight"] = args.weight
            elif adjust_type == "cfg":
                data["weight"] = args.weight
            elif adjust_type == "aspect":
                data["ratio"] = args.ratio
            else:
                raise AssertionError(
                    f"unreachable: unknown adjust_type {adjust_type!r}"
                )
            self.record("adjust", data)
            return

        raise AssertionError(f"unreachable: unknown command {cmd!r}")
