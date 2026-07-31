# 只允许使用项目测试脚本运行测试

import os
import sys
import unittest
from unittest.mock import MagicMock, patch

logging_disabled = False
import logging

logging.disable(logging.CRITICAL)

from fork import parse_args, run_fork
from graphql_utils import GraphQLClient


def _make_client() -> MagicMock:
    return MagicMock(spec=GraphQLClient)


class TestForkHook(unittest.TestCase):

    def setUp(self):
        self.env_patcher = patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_DIRECTORY_ID": "dir:test",
                "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "folder1",
                "IMAGE_FUNNEL_IMAGE_IDS": '["img:1", "img:2"]',
                "IMAGE_FUNNEL_GRAPHQL_URL": "http://localhost:8000/graphql",
                "IMAGE_FUNNEL_TOKEN": "test-token",
                "HOOK_LOGGING_LEVEL": "WARNING",
            },
            clear=True,
        )
        self.env_patcher.start()

    def tearDown(self):
        self.env_patcher.stop()

    def test_parse_args_with_suffix(self):
        with patch.object(sys, "argv", ["fork.py", "custom_suffix"]):
            args = parse_args()
            self.assertEqual(args.suffix, "custom_suffix")

    def test_parse_args_without_suffix(self):
        with patch.object(sys, "argv", ["fork.py"]):
            args = parse_args()
            self.assertEqual(args.suffix, "TODO")

    def test_run_fork_with_explicit_suffix(self):
        mock_client = _make_client()
        with patch.object(sys, "argv", ["fork.py", "custom_suffix"]):
            with self.assertRaises(SystemExit) as cm:
                run_fork(mock_client)
            self.assertEqual(cm.exception.code, 0)
            mock_client.move_images.assert_called_once_with(
                "dir:test",
                ["img:1", "img:2"],
                os.path.normpath("folder1,custom_suffix"),
            )

    def test_run_fork_with_default_todo_suffix(self):
        mock_client = _make_client()
        with patch.object(sys, "argv", ["fork.py"]):
            with self.assertRaises(SystemExit) as cm:
                run_fork(mock_client)
            self.assertEqual(cm.exception.code, 0)
            mock_client.move_images.assert_called_once_with(
                "dir:test", ["img:1", "img:2"], os.path.normpath("folder1,TODO")
            )


if __name__ == "__main__":
    unittest.main()
