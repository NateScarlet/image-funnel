# -*- coding: utf-8 -*-
import argparse
import json
import os
import sqlite3
import tempfile
import unittest
from unittest.mock import patch

from .db import SQLiteContext
from .operation_history import OperationHistory


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

    def test_extract_params_queue(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(command="queue")
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            self.assertEqual(rows[0], ("queue", "{}"))

    def test_extract_params_remove_again(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(command="remove-again")
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            self.assertEqual(rows[0], ("remove-again", "{}"))

    def test_extract_params_add(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(command="add", prompt=["1girl", "solo"])
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history ORDER BY id")
            self.assertEqual(len(rows), 2)
            expected_base = {
                "region": None,
                "node": None,
                "neg": False,
                "raw": False,
                "hard": False,
                "all": False,
                "no_skip": False,
            }
            self.assertEqual(
                json.loads(rows[0][1]), {**expected_base, "prompt": "1girl"}
            )
            self.assertEqual(
                json.loads(rows[1][1]), {**expected_base, "prompt": "solo"}
            )

    def test_extract_params_remove(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(command="remove", prompt=["bad hands"])
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            expected_base = {
                "region": None,
                "node": None,
                "neg": False,
                "raw": False,
                "hard": False,
                "all": False,
                "no_skip": False,
            }
            self.assertEqual(
                json.loads(rows[0][1]), {**expected_base, "prompt": "bad hands"}
            )

    def test_extract_params_adjust_lora(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(
                    command="adjust", adjust_type="lora", name="my_lora", weight="0.5"
                )
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            self.assertEqual(
                json.loads(rows[0][1]),
                {
                    "adjust_type": "lora",
                    "name": "my_lora",
                    "weight": "0.5",
                },
            )

    def test_extract_params_adjust_cfg(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(
                    command="adjust", adjust_type="cfg", weight="7.0"
                )
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            self.assertEqual(
                json.loads(rows[0][1]), {"adjust_type": "cfg", "weight": "7.0"}
            )

    def test_extract_params_adjust_aspect(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            with SQLiteContext(db_path) as db_ctx:
                history = OperationHistory(db_ctx)
                args = argparse.Namespace(
                    command="adjust", adjust_type="aspect", ratio="16:9"
                )
                history.extract_params(args)

            rows = _query(db_path, "SELECT command, data FROM history")
            self.assertEqual(
                json.loads(rows[0][1]), {"adjust_type": "aspect", "ratio": "16:9"}
            )

    def test_from_env(self):
        with patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_ROOT_DIR": "/tmp",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "test_dir",
            },
        ):
            history = OperationHistory.from_env()
        expected = os.path.join("/tmp", "test_dir", ".io.github.natescarlet.hook.db")
        self.assertEqual(history.db_path, expected)

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
