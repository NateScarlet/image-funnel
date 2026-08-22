# 只允许使用项目测试脚本运行测试

import contextlib
import io
import logging
import os
import unittest
import uuid
from unittest.mock import MagicMock, patch
from typing import Any, Dict, Optional

logging.disable(logging.CRITICAL)

from retention import run_retention
from graphql_utils import GraphQLClient


def _page(nodes: list[Dict[str, Any]], has_next: bool = False) -> Dict[str, Any]:
    return {
        "nodes": nodes,
        "pageInfo": {"hasNextPage": has_next, "endCursor": None},
    }


def _make_client() -> MagicMock:
    """创建 mock GraphQLClient，避免测试中使用 patch。"""
    return MagicMock(spec=GraphQLClient)


# 测试临时产物统一放在项目 .scratch 目录，不使用系统临时目录
_SCRATCH_DIR = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))), ".scratch"
)


class TestRetentionMain(unittest.TestCase):

    def setUp(self):
        # 操作覆盖文件直接落在项目 .scratch 目录下（沙箱环境对临时子目录支持不佳），
        # 测试结束后删除该文件
        self.action_path = os.path.join(
            _SCRATCH_DIR, f"retention_action_{uuid.uuid4().hex}.txt"
        )
        self.env_patcher = patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_GRAPHQL_URL": "http://localhost:8000/graphql",
                "IMAGE_FUNNEL_TOKEN": "test-token",
                "IMAGE_FUNNEL_ACTION": self.action_path,
                "HOOK_IMAGE_RATING": "2",
                "HOOK_MAX_RETAIN": "3",
                "HOOK_LOGGING_LEVEL": "WARNING",
            },
            clear=True,
        )
        self.env_patcher.start()

    def tearDown(self):
        self.env_patcher.stop()
        if os.path.exists(self.action_path):
            os.remove(self.action_path)

    def _run_retention_captured(self, client: GraphQLClient) -> str:
        """运行 run_retention 并捕获 stdout，断言以退出码 0 结束"""
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            with self.assertRaises(SystemExit) as cm:
                run_retention(client)
        self.assertEqual(cm.exception.code, 0)
        return buf.getvalue()

    def _read_action_override(self) -> Optional[str]:
        """读取操作覆盖文件内容，文件不存在时返回 None"""
        if not os.path.exists(self.action_path):
            return None
        with open(self.action_path, encoding="utf-8") as f:
            return f.read()

    def test_no_excess_images(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {
            "node": {
                "images": _page(
                    [
                        {
                            "id": "img:1",
                            "modTime": "2024-01-01T00:00:00Z",
                            "note": {"content": ""},
                        },
                        {
                            "id": "img:2",
                            "modTime": "2024-01-02T00:00:00Z",
                            "note": {"content": ""},
                        },
                    ]
                )
            }
        }

        # 未移除任何图片：应静默（无 stdout）并写入 KEEP 操作覆盖供 Runner 跳过通知
        output = self._run_retention_captured(mock_client)
        self.assertEqual(output, "")
        self.assertEqual(self._read_action_override(), "KEEP")
        mock_client.execute.assert_called_once()

    def test_exactly_max_retain(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {
            "node": {
                "images": _page(
                    [
                        {
                            "id": "img:1",
                            "modTime": "2024-01-01T00:00:00Z",
                            "note": {"content": ""},
                        },
                        {
                            "id": "img:2",
                            "modTime": "2024-01-02T00:00:00Z",
                            "note": {"content": ""},
                        },
                        {
                            "id": "img:3",
                            "modTime": "2024-01-03T00:00:00Z",
                            "note": {"content": ""},
                        },
                    ]
                )
            }
        }

        # 未移除任何图片：应静默并写入 KEEP
        output = self._run_retention_captured(mock_client)
        self.assertEqual(output, "")
        self.assertEqual(self._read_action_override(), "KEEP")
        mock_client.execute.assert_called_once()

    def test_excess_images_trashed(self):
        mock_client = _make_client()
        call_log: list[Any] = []

        def side_effect(
            query: str, variables: Optional[Dict[str, Any]] = None
        ) -> Dict[str, Any]:
            call_log.append((query, variables))
            if "query GetDirectoryImages" in query:
                return {
                    "node": {
                        "images": _page(
                            [
                                {
                                    "id": "img:1",
                                    "modTime": "2024-01-01T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:2",
                                    "modTime": "2024-01-02T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:3",
                                    "modTime": "2024-01-03T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:4",
                                    "modTime": "2024-01-04T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:5",
                                    "modTime": "2024-01-05T00:00:00Z",
                                    "note": {"content": ""},
                                },
                            ]
                        )
                    }
                }
            if "mutation TrashImages" in query and variables is not None:
                ids: list[str] = variables["input"]["filterBy"]["id"]
                self.assertEqual(ids, ["img:1", "img:2"])
                return {
                    "trashImages": {
                        "movedCount": 2,
                        "historyId": "hist:1",
                    }
                }
            return {}

        mock_client.execute.side_effect = side_effect

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
        self.assertEqual(len(call_log), 2)

    def test_zero_max_retain(self):
        mock_client = _make_client()
        mock_client.execute.side_effect = [
            {
                "node": {
                    "images": _page(
                        [
                            {
                                "id": "img:1",
                                "modTime": "2024-01-01T00:00:00Z",
                                "note": {"content": ""},
                            },
                            {
                                "id": "img:2",
                                "modTime": "2024-01-02T00:00:00Z",
                                "note": {"content": ""},
                            },
                        ]
                    )
                }
            },
            {"trashImages": {"movedCount": 2, "historyId": "hist:1"}},
        ]

        with patch.dict(os.environ, {"HOOK_MAX_RETAIN": "0"}, clear=False):
            with self.assertRaises(SystemExit) as cm:
                run_retention(mock_client)
            self.assertEqual(cm.exception.code, 0)
            mock_client.execute.assert_called()
            # 实际执行了移除：不应写入操作覆盖
            self.assertIsNone(self._read_action_override())

    def test_no_matching_images(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {"node": {"images": _page([])}}

        # 未移除任何图片：应静默并写入 KEEP
        output = self._run_retention_captured(mock_client)
        self.assertEqual(output, "")
        self.assertEqual(self._read_action_override(), "KEEP")
        mock_client.execute.assert_called_once()

    def test_missing_env_hook_image_rating(self):
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError) as cm:
                run_retention(_make_client())
            self.assertIn("HOOK_IMAGE_RATING", str(cm.exception))

    def test_missing_env_hook_max_retain(self):
        with patch.dict(
            os.environ,
            {"HOOK_IMAGE_RATING": "2"},
            clear=True,
        ):
            with self.assertRaises(ValueError) as cm:
                run_retention(_make_client())
            self.assertIn("HOOK_MAX_RETAIN", str(cm.exception))

    def test_invalid_rating(self):
        with patch.dict(os.environ, {"HOOK_IMAGE_RATING": "abc"}, clear=False):
            with self.assertRaises(SystemExit) as cm:
                run_retention(_make_client())
            self.assertEqual(cm.exception.code, 1)

    def test_invalid_max_retain(self):
        with patch.dict(os.environ, {"HOOK_MAX_RETAIN": "abc"}, clear=False):
            with self.assertRaises(SystemExit) as cm:
                run_retention(_make_client())
            self.assertEqual(cm.exception.code, 1)

    def test_negative_max_retain(self):
        with patch.dict(os.environ, {"HOOK_MAX_RETAIN": "-1"}, clear=False):
            with self.assertRaises(SystemExit) as cm:
                run_retention(_make_client())
            self.assertEqual(cm.exception.code, 1)

    def test_mod_time_order(self):
        mock_client = _make_client()
        trash_ids: list[str] = []

        def side_effect(
            query: str, variables: Optional[Dict[str, Any]] = None
        ) -> Dict[str, Any]:
            if "query GetDirectoryImages" in query:
                return {
                    "node": {
                        "images": _page(
                            [
                                {
                                    "id": "img:5",
                                    "modTime": "2024-01-05T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:1",
                                    "modTime": "2024-01-01T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:3",
                                    "modTime": "2024-01-03T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:2",
                                    "modTime": "2024-01-02T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:4",
                                    "modTime": "2024-01-04T00:00:00Z",
                                    "note": {"content": ""},
                                },
                            ]
                        )
                    }
                }
            if "mutation TrashImages" in query and variables is not None:
                ids_: list[str] = variables["input"]["filterBy"]["id"]
                trash_ids.extend(ids_)
                return {"trashImages": {"movedCount": 2, "historyId": "hist:1"}}
            return {}

        mock_client.execute.side_effect = side_effect

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
        self.assertEqual(trash_ids, ["img:1", "img:2"])

    def test_trash_filter_includes_rating(self):
        mock_client = _make_client()
        trash_variables: list[Dict[str, Any]] = []

        def side_effect(
            query: str, variables: Optional[Dict[str, Any]] = None
        ) -> Dict[str, Any]:
            if "query GetDirectoryImages" in query:
                return {
                    "node": {
                        "images": _page(
                            [
                                {
                                    "id": f"img:{i}",
                                    "modTime": f"2024-01-{i:02d}T00:00:00Z",
                                    "note": {"content": ""},
                                }
                                for i in range(1, 6)
                            ]
                        )
                    }
                }
            if "mutation TrashImages" in query and variables is not None:
                trash_variables.append(variables)
                return {"trashImages": {"movedCount": 2, "historyId": "hist:1"}}
            return {}

        mock_client.execute.side_effect = side_effect

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)

        filter_by: Dict[str, Any] = trash_variables[0]["input"]["filterBy"]
        self.assertIn("id", filter_by)
        self.assertIn("rating", filter_by)
        self.assertEqual(filter_by["rating"], [2])

    def test_node_null_crashes(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {"node": None}

        with self.assertRaises(TypeError):
            run_retention(mock_client)

    def test_missing_images_key_crashes(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {"node": {}}

        with self.assertRaises(KeyError):
            run_retention(mock_client)

    def test_invalid_mod_time_crashes(self):
        mock_client = _make_client()
        mock_client.execute.side_effect = [
            {
                "node": {
                    "images": _page(
                        [
                            {
                                "id": "img:1",
                                "modTime": "invalid-date",
                                "note": {"content": ""},
                            },
                            {
                                "id": "img:2",
                                "modTime": "2024-01-02T00:00:00Z",
                                "note": {"content": ""},
                            },
                            {
                                "id": "img:3",
                                "modTime": "2024-01-03T00:00:00Z",
                                "note": {"content": ""},
                            },
                            {
                                "id": "img:4",
                                "modTime": "2024-01-04T00:00:00Z",
                                "note": {"content": ""},
                            },
                        ]
                    )
                }
            },
            {"trashImages": {"movedCount": 3, "historyId": "hist:1"}},
        ]

        with patch.dict(os.environ, {"HOOK_MAX_RETAIN": "1"}, clear=False):
            with self.assertRaises(ValueError):
                run_retention(mock_client)

    def test_multi_page_fetches_all(self):
        mock_client = _make_client()
        calls: list[int] = []

        def side_effect(
            query: str, variables: Optional[Dict[str, Any]] = None
        ) -> Dict[str, Any]:
            if "query GetDirectoryImages" in query and variables is not None:
                after = variables.get("after")
                if after is None:
                    calls.append(1)
                    return {
                        "node": {
                            "images": {
                                "nodes": [
                                    {
                                        "id": "img:1",
                                        "modTime": "2024-01-01T00:00:00Z",
                                        "note": {"content": ""},
                                    },
                                ],
                                "pageInfo": {
                                    "hasNextPage": True,
                                    "endCursor": "cursor:1",
                                },
                            }
                        }
                    }
                if after == "cursor:1":
                    calls.append(2)
                    return {
                        "node": {
                            "images": {
                                "nodes": [
                                    {
                                        "id": "img:2",
                                        "modTime": "2024-01-02T00:00:00Z",
                                        "note": {"content": ""},
                                    },
                                ],
                                "pageInfo": {
                                    "hasNextPage": True,
                                    "endCursor": "cursor:2",
                                },
                            }
                        }
                    }
                if after == "cursor:2":
                    calls.append(3)
                    return {
                        "node": {
                            "images": {
                                "nodes": [
                                    {
                                        "id": "img:3",
                                        "modTime": "2024-01-03T00:00:00Z",
                                        "note": {"content": ""},
                                    },
                                ],
                                "pageInfo": {
                                    "hasNextPage": False,
                                    "endCursor": None,
                                },
                            }
                        }
                    }
            if "mutation TrashImages" in query:
                return {"trashImages": {"movedCount": 0, "historyId": "hist:1"}}
            return {}

        mock_client.execute.side_effect = side_effect

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
        # 应该获取了所有3页的数据
        self.assertEqual(calls, [1, 2, 3])

    def test_excess_all_protected_by_note(self):
        """所有超出的图片都有非空笔记，不移除任何图片"""
        mock_client = _make_client()
        mock_client.execute.return_value = {
            "node": {
                "images": _page(
                    [
                        {
                            "id": "img:1",
                            "modTime": "2024-01-01T00:00:00Z",
                            "note": {"content": "笔记内容"},
                        },
                        {
                            "id": "img:2",
                            "modTime": "2024-01-02T00:00:00Z",
                            "note": {"content": "笔记内容"},
                        },
                        {
                            "id": "img:3",
                            "modTime": "2024-01-03T00:00:00Z",
                            "note": {"content": ""},
                        },
                        {
                            "id": "img:4",
                            "modTime": "2024-01-04T00:00:00Z",
                            "note": {"content": ""},
                        },
                        {
                            "id": "img:5",
                            "modTime": "2024-01-05T00:00:00Z",
                            "note": {"content": ""},
                        },
                    ]
                )
            }
        }

        # 未移除任何图片：应静默并写入 KEEP
        output = self._run_retention_captured(mock_client)
        self.assertEqual(output, "")
        self.assertEqual(self._read_action_override(), "KEEP")
        # 只应有一次查询调用，没有 trash 调用
        mock_client.execute.assert_called_once()

    def test_excess_partially_protected_by_note(self):
        """部分超出图片有笔记，仅移除无笔记的超出图片"""
        mock_client = _make_client()
        call_log: list[Any] = []

        def side_effect(
            query: str, variables: Optional[Dict[str, Any]] = None
        ) -> Dict[str, Any]:
            call_log.append((query, variables))
            if "query GetDirectoryImages" in query:
                return {
                    "node": {
                        "images": _page(
                            [
                                {
                                    "id": "img:1",
                                    "modTime": "2024-01-01T00:00:00Z",
                                    "note": {"content": "笔记内容"},
                                },
                                {
                                    "id": "img:2",
                                    "modTime": "2024-01-02T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:3",
                                    "modTime": "2024-01-03T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:4",
                                    "modTime": "2024-01-04T00:00:00Z",
                                    "note": {"content": ""},
                                },
                                {
                                    "id": "img:5",
                                    "modTime": "2024-01-05T00:00:00Z",
                                    "note": {"content": ""},
                                },
                            ]
                        )
                    }
                }
            if "mutation TrashImages" in query and variables is not None:
                ids: list[str] = variables["input"]["filterBy"]["id"]
                self.assertEqual(ids, ["img:2"])
                return {
                    "trashImages": {
                        "movedCount": 1,
                        "historyId": "hist:1",
                    }
                }
            return {}

        mock_client.execute.side_effect = side_effect

        # 实际执行了移除：stdout 输出结果摘要，不写操作覆盖
        output = self._run_retention_captured(mock_client)
        self.assertIn("已清理", output)
        self.assertIsNone(self._read_action_override())
        # 有笔记保护的 img:1 不应出现在 trash 列表中
        self.assertEqual(len(call_log), 2)

    def test_missing_env_action_override(self):
        """跳过场景需要写入操作覆盖文件，缺失 IMAGE_FUNNEL_ACTION 应快速失败"""
        with patch.dict(os.environ, {}, clear=True):
            with patch.dict(
                os.environ,
                {
                    "HOOK_IMAGE_RATING": "2",
                    "HOOK_MAX_RETAIN": "3",
                    "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                },
                clear=False,
            ):
                mock_client = _make_client()
                mock_client.execute.return_value = {"node": {"images": _page([])}}
                with self.assertRaises(ValueError) as cm:
                    run_retention(mock_client)
                self.assertIn("IMAGE_FUNNEL_ACTION", str(cm.exception))


if __name__ == "__main__":
    unittest.main()
