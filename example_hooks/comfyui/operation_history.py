# -*- coding: utf-8 -*-
"""操作历史记录模块：将 comfyui 指令执行记录写入当前目录下的 SQLite 数据库。

数据库文件路径：{IMAGE_FUNNEL_ROOT_DIR}/{IMAGE_FUNNEL_DIRECTORY_REL_PATH}/.io.github.natescarlet.hook.db
仅在 __main__.py 的执行路径中调用，autocomplete 有独立入口，不会经过此模块。
"""

import argparse
import json
import os
from dataclasses import dataclass
from typing import Any

from .db import SQLiteContext


@dataclass
class RemoveRecord:
    """历史 remove 操作的完整参数记录"""

    prompt: str
    region: list[str] | None = None
    node: list[str] | None = None
    neg: bool = False
    raw: bool = False
    hard: bool = False
    all: bool = False
    no_skip: bool = False


class OperationHistory:

    def __init__(self, db_ctx: SQLiteContext) -> None:
        self.db_ctx = db_ctx

    @property
    def db_path(self) -> str:
        """返回底层数据库路径以保证对现有测试的向后兼容。"""
        return self.db_ctx.db_path

    @staticmethod
    def from_env() -> "OperationHistory":
        return OperationHistory(SQLiteContext.from_env())

    def record(self, command: str, data: dict[str, Any]) -> None:
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO history (command, data) VALUES (?, ?)",
                (command, json.dumps(data, ensure_ascii=False)),
            )

    def extract_params(self, args: argparse.Namespace) -> None:
        """从 argparse 解析结果中提取参数并写入历史记录。

        add/remove 的每个 prompt 参数是独立操作，分别写入一条记录。
        """

        cmd = args.command

        if cmd in ("queue", "remove-again"):
            self.record(cmd, {})
            return

        if cmd in ("add", "remove"):
            base = {
                "region": getattr(args, "region", None),
                "node": getattr(args, "node", None),
                "neg": getattr(args, "neg", False),
                "raw": getattr(args, "raw", False),
                "hard": getattr(args, "hard", False),
                "all": getattr(args, "all", False),
                "no_skip": getattr(args, "no_skip", False),
            }
            for prompt in args.prompt:
                self.record(cmd, {**base, "prompt": prompt})
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

    def get_added_prompts(self, candidates: list[str]) -> set[str]:
        """返回 candidates 中已在历史记录中存在的提示词集合。"""
        if not candidates:
            return set()

        if self.db_path != ":memory:" and not os.path.isfile(self.db_path):
            return set()

        placeholders = ",".join("?" * len(candidates))
        rows = self.db_ctx.connection.execute(
            f"SELECT DISTINCT json_extract(data, '$.prompt') FROM history WHERE command = 'add' AND json_extract(data, '$.prompt') IN ({placeholders})",
            candidates,
        ).fetchall()
        return {row[0] for row in rows}

    def list_remove(self) -> list[RemoveRecord]:
        """返回所有历史 remove 操作的去重列表，按首次出现顺序排列。"""
        with self.db_ctx.transaction() as conn:
            rows = conn.execute(
                "SELECT data, MIN(id) FROM history "
                "WHERE command = 'remove' "
                "GROUP BY data "
                "ORDER BY MIN(id)"
            ).fetchall()
            return [RemoveRecord(**json.loads(row[0])) for row in rows]

    def get_added_prompt_times(self, candidates: list[str]) -> dict[str, str]:
        """返回 candidates 中各提示词最近一次添加的 created_at 时间戳字典。"""
        if not candidates:
            return {}

        if self.db_path != ":memory:" and not os.path.isfile(self.db_path):
            return {}

        placeholders = ",".join("?" * len(candidates))
        rows = self.db_ctx.connection.execute(
            f"SELECT json_extract(data, '$.prompt'), MAX(created_at) FROM history "
            f"WHERE command = 'add' AND json_extract(data, '$.prompt') IN ({placeholders}) "
            f"GROUP BY json_extract(data, '$.prompt')",
            candidates,
        ).fetchall()
        return {row[0]: row[1] for row in rows}


def get_added_prompts(candidates: list[str]) -> set[str]:
    """返回 candidates 中已在历史记录中存在的提示词集合。"""
    return OperationHistory.from_env().get_added_prompts(candidates)


def get_added_prompt_times(candidates: list[str]) -> dict[str, str]:
    """返回 candidates 中各提示词最近一次添加的 created_at 时间戳字典。"""
    return OperationHistory.from_env().get_added_prompt_times(candidates)
