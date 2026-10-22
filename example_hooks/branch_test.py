import contextlib
import io
import os
import sys
import tempfile
import unittest
from unittest.mock import MagicMock, patch

# 在 clear=True 的环境补丁生效前定格临时目录基准，避免临时目录落到 cwd
from test_support import pin_tempdir

pin_tempdir()

logging_disabled = False
import logging

logging.disable(logging.CRITICAL)

from branch import (
    get_branch_autocomplete_suggestions,
    parse_args,
    run_autocomplete,
    run_branch,
)
from graphql_utils import GraphQLClient


def _make_client() -> MagicMock:
    return MagicMock(spec=GraphQLClient)


class TestBranchHook(unittest.TestCase):

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

    def _run_branch_captured(self, client: GraphQLClient, argv: list[str]) -> str:
        """运行 run_branch 并捕获 stdout，断言正常退出，避免污染测试输出。"""
        buf = io.StringIO()
        with patch.object(sys, "argv", argv), contextlib.redirect_stdout(buf):
            with self.assertRaises(SystemExit) as cm:
                run_branch(client)
        self.assertEqual(cm.exception.code, 0)
        return buf.getvalue()

    def test_parse_args_with_suffix(self):
        with patch.object(sys, "argv", ["branch.py", "custom_suffix"]):
            args = parse_args()
            self.assertEqual(args.suffix, "custom_suffix")

    def test_parse_args_without_suffix(self):
        with patch.object(sys, "argv", ["branch.py"]):
            args = parse_args()
            self.assertEqual(args.suffix, "TODO")

    def test_run_branch_with_explicit_suffix(self):
        mock_client = _make_client()
        output = self._run_branch_captured(mock_client, ["branch.py", "custom_suffix"])
        mock_client.move_images.assert_called_once_with(
            "dir:test",
            ["img:1", "img:2"],
            os.path.normpath("folder1,custom_suffix"),
        )
        # 分支成功的用户可见反馈必须指向实际目标目录
        self.assertIn("folder1,custom_suffix", output)

    def test_run_branch_with_default_todo_suffix(self):
        mock_client = _make_client()
        output = self._run_branch_captured(mock_client, ["branch.py"])
        mock_client.move_images.assert_called_once_with(
            "dir:test", ["img:1", "img:2"], os.path.normpath("folder1,TODO")
        )
        self.assertIn("folder1,TODO", output)

    def test_autocomplete_in_root_directory(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))
            os.makedirs(os.path.join(temp_dir, "beta"))
            with open(os.path.join(temp_dir, "file1.txt"), "w") as f:
                f.write("test")

            suggestions = get_branch_autocomplete_suggestions(temp_dir, "")
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

            suggestions = get_branch_autocomplete_suggestions(temp_dir, "folder1")
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

            suggestions = get_branch_autocomplete_suggestions(temp_dir, "folder1", "r")
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
                    "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/branch"]',
                    "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "/branch",
                },
            ):
                captured_stdout = io.StringIO()
                with patch.object(sys, "stdout", captured_stdout):
                    run_autocomplete()

                output = captured_stdout.getvalue().strip()
                self.assertIn('"text": "tag_1"', output)
                self.assertIn('"description": "已存在同级目录：folder1,tag_1"', output)

    def test_run_autocomplete_only_suggests_on_first_positional(self):
        """usage 只有一个可选位置参数：`/branch foo ` 之后的位置一律不给建议。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))

            cases: list[tuple[str, str, bool]] = [
                ('["/branch"]', "/branch", True),
                # 指令模板插入后尚未改写占位符：仍处于第一个位置参数位
                ('["/branch", "<suffix>"]', "<suffix>", True),
                ('["/branch", "foo"]', "foo", False),
                ('["/branch", "foo", "bar"]', "bar", False),
            ]
            for cwords, prev_word, expects_suggestions in cases:
                with self.subTest(cwords=cwords):
                    with patch.dict(
                        os.environ,
                        {
                            "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                            "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "",
                            "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "",
                            "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": cwords,
                            "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": prev_word,
                        },
                    ):
                        captured_stdout = io.StringIO()
                        with patch.object(sys, "stdout", captured_stdout):
                            run_autocomplete()

                    output = captured_stdout.getvalue()
                    self.assertEqual(bool(output.strip()), expects_suggestions)
                    if expects_suggestions:
                        self.assertIn('"text": "alpha"', output)


if __name__ == "__main__":
    unittest.main()
