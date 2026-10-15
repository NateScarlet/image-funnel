# -*- coding: utf-8 -*-
"""操作历史记录模块：将 comfyui 指令执行记录写入当前目录下的 SQLite 数据库。

数据库文件路径：{IMAGE_FUNNEL_ROOT_DIR}/{IMAGE_FUNNEL_DIRECTORY_REL_PATH}/.io.github.natescarlet.hook.db
仅在 __main__.py 的执行路径中调用，autocomplete 有独立入口，不会经过此模块。
"""

import json
import os
from dataclasses import dataclass
from typing import Any

from .db import SQLiteContext
from .remix import RemixRequest


@dataclass(frozen=True)
class RemoveRecord:
    """历史 remove 操作的完整参数记录。

    移除动作已无范围概念（重混总是作用于所有 CLIPTextEncode 节点），因此这里只保留
    提示词本身与删除方式；范围参数仅存在于旧记录中，重放时一律忽略。
    """

    prompt: str
    raw: bool = False
    hard: bool = False


class OperationHistory:

    def __init__(self, db_ctx: SQLiteContext) -> None:
        self.db_ctx = db_ctx

    @property
    def db_path(self) -> str:
        """返回底层数据库路径以保证对现有测试的向后兼容。"""
        return self.db_ctx.db_path

    def record(self, command: str, data: dict[str, Any]) -> None:
        with self.db_ctx.transaction() as conn:
            conn.execute(
                "INSERT INTO history (command, data) VALUES (?, ?)",
                (command, json.dumps(data, ensure_ascii=False)),
            )

    def record_remix(self, request: RemixRequest) -> None:
        """把一次重混请求逐项写入历史记录。

        各项仍以原 command 名（add / remove / adjust）落库，既有的 `list_remove`、
        `get_added_prompts` 等查询与 `--remove-again` 重放因此不受指令合并影响。
        每个新增/移除提示词是独立操作，分别写入一条记录。
        """
        for tag in request.add_tags:
            self.record("add", {"prompt": tag})

        for tag in request.remove_tags:
            self.record(
                "remove",
                {"prompt": tag, "raw": request.raw, "hard": request.hard},
            )

        for edit in request.prompt_weights:
            self.record(
                "adjust",
                {"adjust_type": "prompt", "text": edit.tag, "weight": edit.weight_expr},
            )
        for edit in request.loras:
            self.record(
                "adjust",
                {"adjust_type": "lora", "name": edit.name, "weight": edit.weight_expr},
            )
        for expr in request.cfgs:
            self.record("adjust", {"adjust_type": "cfg", "weight": expr})
        for expr in request.aspects:
            self.record("adjust", {"adjust_type": "aspect", "ratio": expr})

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
        """返回所有历史 remove 操作的去重列表，按首次出现顺序排列。

        旧记录中的范围参数（region / node / neg / all）在重混语义下已无意义，
        显式丢弃而不是随记录原样带出。
        """
        with self.db_ctx.transaction() as conn:
            rows = conn.execute(
                "SELECT data, MIN(id) FROM history "
                "WHERE command = 'remove' "
                "GROUP BY data "
                "ORDER BY MIN(id)"
            ).fetchall()
        records: list[RemoveRecord] = []
        for row in rows:
            data: dict[str, Any] = json.loads(row[0])
            records.append(
                RemoveRecord(
                    prompt=data["prompt"],
                    raw=bool(data.get("raw", False)),
                    hard=bool(data.get("hard", False)),
                )
            )
        return records

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

    def get_all_added_prompts(self) -> list[tuple[str, str]]:
        """返回所有历史 add 操作的去重提示词列表，按最近添加时间倒序排列。

        返回 (prompt, created_at) 元组列表。
        """
        if self.db_path != ":memory:" and not os.path.isfile(self.db_path):
            return []

        rows = self.db_ctx.connection.execute(
            "SELECT json_extract(data, '$.prompt'), MAX(created_at) FROM history "
            "WHERE command = 'add' AND json_extract(data, '$.prompt') IS NOT NULL "
            "GROUP BY json_extract(data, '$.prompt') "
            "ORDER BY MAX(created_at) DESC"
        ).fetchall()
        return [(row[0], row[1]) for row in rows]
