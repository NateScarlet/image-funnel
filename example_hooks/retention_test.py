# 只允许使用项目测试脚本运行测试

import logging
import os
import unittest
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


class TestRetentionMain(unittest.TestCase):

    def setUp(self):
        self.env_patcher = patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_GRAPHQL_URL": "http://localhost:8000/graphql",
                "IMAGE_FUNNEL_TOKEN": "test-token",
                "HOOK_IMAGE_RATING": "2",
                "HOOK_MAX_RETAIN": "3",
                "HOOK_LOGGING_LEVEL": "WARNING",
            },
            clear=True,
        )
        self.env_patcher.start()

    def tearDown(self):
        self.env_patcher.stop()

    def test_no_excess_images(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {
            "node": {
                "images": _page(
                    [
                        {"id": "img:1", "modTime": "2024-01-01T00:00:00Z"},
                        {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
                    ]
                )
            }
        }

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
        mock_client.execute.assert_called_once()

    def test_exactly_max_retain(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {
            "node": {
                "images": _page(
                    [
                        {"id": "img:1", "modTime": "2024-01-01T00:00:00Z"},
                        {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
                        {"id": "img:3", "modTime": "2024-01-03T00:00:00Z"},
                    ]
                )
            }
        }

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
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
                                {"id": "img:1", "modTime": "2024-01-01T00:00:00Z"},
                                {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
                                {"id": "img:3", "modTime": "2024-01-03T00:00:00Z"},
                                {"id": "img:4", "modTime": "2024-01-04T00:00:00Z"},
                                {"id": "img:5", "modTime": "2024-01-05T00:00:00Z"},
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
                            {"id": "img:1", "modTime": "2024-01-01T00:00:00Z"},
                            {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
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

    def test_no_matching_images(self):
        mock_client = _make_client()
        mock_client.execute.return_value = {"node": {"images": _page([])}}

        with self.assertRaises(SystemExit) as cm:
            run_retention(mock_client)
        self.assertEqual(cm.exception.code, 0)
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
                                {"id": "img:5", "modTime": "2024-01-05T00:00:00Z"},
                                {"id": "img:1", "modTime": "2024-01-01T00:00:00Z"},
                                {"id": "img:3", "modTime": "2024-01-03T00:00:00Z"},
                                {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
                                {"id": "img:4", "modTime": "2024-01-04T00:00:00Z"},
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
                            {"id": "img:1", "modTime": "invalid-date"},
                            {"id": "img:2", "modTime": "2024-01-02T00:00:00Z"},
                            {"id": "img:3", "modTime": "2024-01-03T00:00:00Z"},
                            {"id": "img:4", "modTime": "2024-01-04T00:00:00Z"},
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


if __name__ == "__main__":
    unittest.main()
