import argparse
import json
import os
import sqlite3
import tempfile
from unittest.mock import patch

from .operation_history import OperationHistory


def _query(history: OperationHistory, sql: str, params: tuple[object, ...] = ()):
    conn = sqlite3.connect(history.db_path)
    try:
        return conn.execute(sql, params).fetchall()
    finally:
        conn.close()


def test_record_writes_to_db():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        history.ensure_db()
        history.record("add", {"prompt": "1girl"})

        rows = _query(history, "SELECT command, data FROM history ORDER BY id")
        assert len(rows) == 1
        assert rows[0][0] == "add"
        assert json.loads(rows[0][1]) == {"prompt": "1girl"}


def test_record_multiple():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        history.ensure_db()
        history.record("add", {"prompt": "1girl"})
        history.record("remove", {"prompt": "bad hands"})

        rows = _query(history, "SELECT command, data FROM history ORDER BY id")
        assert len(rows) == 2
        assert rows[0] == ("add", json.dumps({"prompt": "1girl"}, ensure_ascii=False))
        assert rows[1] == (
            "remove",
            json.dumps({"prompt": "bad hands"}, ensure_ascii=False),
        )


def test_extract_params_queue():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(command="queue")
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history")
        assert rows[0] == ("queue", "{}")


def test_extract_params_add():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(command="add", prompt=["1girl", "solo"])
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history ORDER BY id")
        assert len(rows) == 2
        assert rows[0] == ("add", json.dumps({"prompt": "1girl"}, ensure_ascii=False))
        assert rows[1] == ("add", json.dumps({"prompt": "solo"}, ensure_ascii=False))


def test_extract_params_remove():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(command="remove", prompt=["bad hands"])
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history")
        assert rows[0] == (
            "remove",
            json.dumps({"prompt": "bad hands"}, ensure_ascii=False),
        )


def test_extract_params_adjust_lora():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(
            command="adjust", adjust_type="lora", name="my_lora", weight="0.5"
        )
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history")
        assert json.loads(rows[0][1]) == {
            "adjust_type": "lora",
            "name": "my_lora",
            "weight": "0.5",
        }


def test_extract_params_adjust_cfg():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(command="adjust", adjust_type="cfg", weight="7.0")
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history")
        assert json.loads(rows[0][1]) == {"adjust_type": "cfg", "weight": "7.0"}


def test_extract_params_adjust_aspect():
    with tempfile.TemporaryDirectory() as tmp:
        db_path = os.path.join(tmp, "test.db")
        history = OperationHistory(db_path)
        args = argparse.Namespace(command="adjust", adjust_type="aspect", ratio="16:9")
        history.extract_params(args)

        rows = _query(history, "SELECT command, data FROM history")
        assert json.loads(rows[0][1]) == {"adjust_type": "aspect", "ratio": "16:9"}


def test_from_env():
    with patch.dict(
        os.environ,
        {
            "IMAGE_FUNNEL_ROOT_DIR": "/tmp",
            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "test_dir",
        },
    ):
        history = OperationHistory.from_env()
    assert history.db_path == "/tmp/test_dir/.io.github.nantescarlet.hook.db"
