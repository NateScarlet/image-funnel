# -*- coding: utf-8 -*-
import json
import os
import sqlite3
import tempfile
import unittest

from test_support import pin_tempdir

pin_tempdir()

from .db import SQLiteContext
from .operation_history import OperationHistory
from .remix import RemixRequest, LoraEdit, PromptWeightEdit


def _query(db_path: str, sql: str, params: tuple[object, ...] = ()):
    conn = sqlite3.connect(db_path)
    try:
        return conn.execute(sql, params).fetchall()
    finally:
        conn.close()


class TestOperationHistory(unittest.TestCase):

    def test_record_writes_to_db(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record("add", {"prompt": "1girl"})

            rows = _query(db_path, "SELECT command, data FROM history ORDER BY id")
            self.assertEqual(len(rows), 1)
            self.assertEqual(rows[0][0], "add")
            self.assertEqual(json.loads(rows[0][1]), {"prompt": "1girl"})

    def test_record_multiple(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record("add", {"prompt": "1girl"})
                history.record("remove", {"prompt": "bad hands"})

            rows = _query(db_path, "SELECT command, data FROM history ORDER BY id")
            self.assertEqual(len(rows), 2)
            self.assertEqual(
                rows[0], ("add", json.dumps({"prompt": "1girl"}, ensure_ascii=False))
            )
            self.assertEqual(
                rows[1],
                ("remove", json.dumps({"prompt": "bad hands"}, ensure_ascii=False)),
            )

    def test_record_remix_splits_each_prompt_into_its_own_record(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record_remix(
                    RemixRequest(add_tags=("1girl", "solo"), remove_tags=("bad hands",))
                )

            rows = _query(db_path, "SELECT command, data FROM history ORDER BY id")
            self.assertEqual([row[0] for row in rows], ["add", "add", "remove"])
            self.assertEqual(json.loads(rows[0][1]), {"prompt": "1girl"})
            self.assertEqual(json.loads(rows[1][1]), {"prompt": "solo"})
            self.assertEqual(
                json.loads(rows[2][1]),
                {"prompt": "bad hands", "raw": False, "hard": False},
            )

    def test_record_remix_keeps_removal_mode(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record_remix(
                    RemixRequest(remove_tags=("a",), raw=True, hard=True)
                )

            rows = _query(db_path, "SELECT data FROM history")
            self.assertEqual(
                json.loads(rows[0][0]),
                {"prompt": "a", "raw": True, "hard": True},
            )

    def test_record_remix_records_weight_edits_by_original_command(self):
        """调权仍以 adjust 落库，既有查询与重放因此不受指令合并影响。"""
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record_remix(
                    RemixRequest(
                        prompt_weights=(PromptWeightEdit("masterpiece", "0.8"),),
                        loras=(LoraEdit("my_lora", "0.5"),),
                        cfgs=("5", "7"),
                        aspects=("16:9",),
                    )
                )

            rows = _query(db_path, "SELECT data FROM history ORDER BY id")
            self.assertEqual(
                [json.loads(row[0]) for row in rows],
                [
                    {
                        "adjust_type": "prompt",
                        "text": "masterpiece",
                        "weight": "0.8",
                    },
                    {
                        "adjust_type": "lora",
                        "name": "my_lora",
                        "weight": "0.5",
                    },
                    {"adjust_type": "cfg", "weight": "5"},
                    {"adjust_type": "cfg", "weight": "7"},
                    {"adjust_type": "aspect", "ratio": "16:9"},
                ],
            )

    def test_record_remix_without_edits_writes_nothing(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record_remix(RemixRequest())
                self.assertEqual(history.list_remove(), [])

    def test_list_remove_drops_legacy_scope_parameters(self):
        """旧记录中的范围参数在重混语义下无意义，显式丢弃而不是随记录带出。"""
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, ".io.github.natescarlet.hook.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record(
                    "remove",
                    {
                        "prompt": "bad hands",
                        "region": ["positive"],
                        "node": ["1"],
                        "neg": True,
                        "raw": True,
                        "hard": False,
                        "all": True,
                        "no_skip": False,
                    },
                )

                records = history.list_remove()
                self.assertEqual(len(records), 1)
                self.assertEqual(records[0].prompt, "bad hands")
                self.assertTrue(records[0].raw)
                self.assertFalse(records[0].hard)

    def test_get_added_prompts(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, ".io.github.natescarlet.hook.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record("add", {"prompt": "1girl"})
                history.record("add", {"prompt": "solo"})
                history.record("remove", {"prompt": "bad hands"})

                # 测试正常匹配
                res = history.get_added_prompts(
                    ["1girl", "solo", "bad hands", "absent"]
                )
                self.assertEqual(res, {"1girl", "solo"})

                # 测试空候选
                self.assertEqual(history.get_added_prompts([]), set())

    def test_get_added_prompts_db_not_exist(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(
                tmp, "non_exist_dir", ".io.github.natescarlet.hook.db"
            )
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                res = history.get_added_prompts(["1girl"])
                self.assertEqual(res, set())

    def test_get_all_added_prompts(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, ".io.github.natescarlet.hook.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                history.record("add", {"prompt": "solo"})
                history.record("add", {"prompt": "1girl"})
                history.record("add", {"prompt": "solo"})  # 重复添加
                history.record("remove", {"prompt": "bad hands"})

                res = history.get_all_added_prompts()
                # 按最近添加时间倒序，solo 最后添加应排最前
                self.assertEqual(len(res), 2)
                self.assertEqual(res[0][0], "solo")
                self.assertEqual(res[1][0], "1girl")

    def test_get_all_added_prompts_db_not_exist(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(
                tmp, "non_exist_dir", ".io.github.natescarlet.hook.db"
            )
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                res = history.get_all_added_prompts()
                self.assertEqual(res, [])
