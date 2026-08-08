import io
import os
import sys
import tempfile
import unittest
from unittest.mock import MagicMock, patch

logging_disabled = False
import logging

logging.disable(logging.CRITICAL)

from fork import (
    get_fork_autocomplete_suggestions,
    parse_args,
    run_autocomplete,
    run_fork,
)
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

    def test_autocomplete_in_root_directory(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))
            os.makedirs(os.path.join(temp_dir, "beta"))
            with open(os.path.join(temp_dir, "file1.txt"), "w") as f:
                f.write("test")

            suggestions = get_fork_autocomplete_suggestions(temp_dir, "")
            self.assertEqual(len(suggestions), 2)
            self.assertEqual(suggestions[0].text, "alpha")
            self.assertEqual(suggestions[0].description, "已存在目录：alpha")
            self.assertEqual(suggestions[1].text, "beta")
            self.assertEqual(suggestions[1].description, "已存在目录：beta")

    def test_autocomplete_in_subdirectory(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            folder1 = os.path.join(temp_dir, "folder1")
            os.makedirs(folder1)
            os.makedirs(os.path.join(temp_dir, "folder1,tag_a"))
            os.makedirs(os.path.join(temp_dir, "folder1,tag_b"))
            os.makedirs(os.path.join(temp_dir, "folder2"))
            with open(os.path.join(temp_dir, "folder1,file.txt"), "w") as f:
                f.write("file")

            suggestions = get_fork_autocomplete_suggestions(temp_dir, "folder1")
            self.assertEqual(len(suggestions), 2)
            self.assertEqual(suggestions[0].text, "tag_a")
            self.assertEqual(
                suggestions[0].description, "已存在同级目录：folder1,tag_a"
            )
            self.assertEqual(suggestions[1].text, "tag_b")
            self.assertEqual(
                suggestions[1].description, "已存在同级目录：folder1,tag_b"
            )

    def test_autocomplete_query_filtering(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            folder1 = os.path.join(temp_dir, "folder1")
            os.makedirs(folder1)
            os.makedirs(os.path.join(temp_dir, "folder1,red"))
            os.makedirs(os.path.join(temp_dir, "folder1,rose"))
            os.makedirs(os.path.join(temp_dir, "folder1,blue"))

            suggestions = get_fork_autocomplete_suggestions(temp_dir, "folder1", "r")
            self.assertEqual(len(suggestions), 2)
            self.assertEqual([s.text for s in suggestions], ["red", "rose"])

    def test_run_autocomplete_cli_output(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            folder1 = os.path.join(temp_dir, "folder1")
            os.makedirs(folder1)
            os.makedirs(os.path.join(temp_dir, "folder1,tag_1"))

            with patch.dict(
                os.environ,
                {
                    "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                    "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "folder1",
                    "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                },
            ):
                captured_stdout = io.StringIO()
                with patch.object(sys, "stdout", captured_stdout):
                    run_autocomplete()

                output = captured_stdout.getvalue().strip()
                self.assertIn('"text": "tag_1"', output)
                self.assertIn('"description": "已存在同级目录：folder1,tag_1"', output)


if __name__ == "__main__":
    unittest.main()
