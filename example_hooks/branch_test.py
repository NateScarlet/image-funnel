import contextlib
import io
import os
import shutil
import sys
import tempfile
import unittest
from typing import List, Optional
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
from comfyui.cancellable_http import HttpError
from comfyui.cancellation import CancelToken
from comfyui.danbooru import (
    DanbooruTag,
    NullDanbooruCache,
    SemanticLayerUnavailable,
    build_danbooru_tag_provider,
)
from comfyui.danbooru_test import write_file_fixture
from graphql_utils import GraphQLClient

# 不涉及取消协议的用例共用：/branch 补全走单次执行，令牌只需满足调用期签名
_TOKEN = CancelToken()


def _make_client() -> MagicMock:
    return MagicMock(spec=GraphQLClient)


def _fake_tag_provider(
    tags: Optional[List[DanbooruTag]] = None,
    error: Optional[Exception] = None,
) -> MagicMock:
    """假搜索实现：标签结果或搜索失败都由用例预置，全程不触网。"""
    provider = MagicMock()
    if error is not None:
        provider.search.side_effect = error
    else:
        provider.search.return_value = tags or []
    return provider


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

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "", None, _TOKEN
            )
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

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "folder1", "", None, _TOKEN
            )
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

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "folder1", "r", None, _TOKEN
            )
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
                # 未配置任何 Danbooru 数据来源：退化为纯目录补全，不出现错误提示项
                self.assertNotIn('"type": "error"', output)

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


class TestBranchAutocompleteDanbooru(unittest.TestCase):
    """`/branch` 在已存在目录候选之后追加 Danbooru 标签候选。"""

    def test_tag_suggestions_follow_existing_directory_suggestions(self):
        """两类候选顺序固定：先已存在目录，再标签；补全不依赖图片/工作流。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))
            provider = _fake_tag_provider(
                [DanbooruTag("alpha wolf", "阿尔法狼", "Wolf girl.", "General")]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "a", provider, _TOKEN
            )

        self.assertEqual([s.text for s in suggestions], ["alpha", "alpha_wolf"])
        self.assertEqual(suggestions[0].type, "directory")
        self.assertEqual(suggestions[1].type, "danbooru")

    def test_empty_query_with_real_provider_only_returns_directories(self):
        """真实本地 provider 对空查询无结果：空 suffix 时只给目录候选。

        `/branch` 不依赖工作流也没有历史种子，空查询不会像 /remix 那样回退到
        关联联想或历史标签；这里用真实编译产物夹具固化该链路行为（离线）。
        """
        data_dir = tempfile.mkdtemp()
        try:
            write_file_fixture(
                data_dir,
                [
                    {
                        "name": "blue_eyes",
                        "cn_name": "蓝眼",
                        "wiki": "Blue eyes.",
                        "post_count": "100",
                        "category": "0",
                        "nsfw": "0",
                    }
                ],
                [],
            )
            provider = build_danbooru_tag_provider(
                data_dir=data_dir,
                search_url="",
                show_nsfw=False,
                embedding_provider_url="",
                cache=NullDanbooruCache(),
            )
            assert provider is not None
            self.assertEqual(provider.search("", cancel_token=_TOKEN), [])

            with tempfile.TemporaryDirectory() as temp_dir:
                os.makedirs(os.path.join(temp_dir, "alpha"))
                suggestions = get_branch_autocomplete_suggestions(
                    temp_dir, "", "", provider, _TOKEN
                )

            self.assertEqual([s.text for s in suggestions], ["alpha"])
            self.assertEqual({s.type for s in suggestions}, {"directory"})
        finally:
            shutil.rmtree(data_dir, ignore_errors=True)

    def test_tag_suggestion_uses_sdxl_text_and_keeps_display_and_wiki(self):
        """插入文本走提交路径的 sdxl 格式化；展示保留中文名，描述保留释义。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            provider = _fake_tag_provider(
                [DanbooruTag("long hair", "长发", "Long hair.", "General")]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "long", provider, _TOKEN
            )

        self.assertEqual(suggestions[0].text, "long_hair")
        self.assertEqual(suggestions[0].displayText, "long hair (长发)")
        self.assertEqual(suggestions[0].description, "Long hair.")
        self.assertEqual(suggestions[0].style, "")

    def test_hand_typed_suffix_is_passed_to_search_unchanged(self):
        """手输值原样保留：查询原样交给搜索管线，不预先归一化。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            provider = _fake_tag_provider(
                [DanbooruTag("long hair", "长发", "Long hair.", "General")]
            )

            get_branch_autocomplete_suggestions(
                temp_dir, "", "Long Hair", provider, _TOKEN
            )

        provider.search.assert_called_once_with("Long Hair", cancel_token=_TOKEN)

    def test_tag_duplicating_directory_candidate_is_deduped(self):
        """与目录候选文本相同时只保留目录条目（它已带「已存在」语义）。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "folder1"))
            os.makedirs(os.path.join(temp_dir, "folder1,blue_eyes"))
            provider = _fake_tag_provider(
                [DanbooruTag("blue eyes", "蓝眼", "Blue eyes.", "General")]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "folder1", "blue", provider, _TOKEN
            )

        self.assertEqual([s.text for s in suggestions], ["blue_eyes"])
        self.assertEqual(suggestions[0].type, "directory")
        self.assertEqual(
            suggestions[0].description, "已存在同级目录：folder1,blue_eyes"
        )

    def test_tag_suggestion_marks_existing_sibling_directory(self):
        """标签候选落到已存在的目标目录时带「已存在」标记并置为 muted。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "folder1"))
            os.makedirs(os.path.join(temp_dir, "folder1,long_hair"))
            provider = _fake_tag_provider(
                [DanbooruTag("long hair", "长发", "Long hair.", "General")]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "folder1", "hair", provider, _TOKEN
            )

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].style, "muted")
        self.assertEqual(suggestions[0].description, "(已存在) Long hair.")

    def test_tag_suggestion_marks_existing_root_directory(self):
        """根目录触发时目标目录即标签文本本身。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "blue_eyes"))
            provider = _fake_tag_provider(
                [DanbooruTag("blue eyes", "蓝眼", "Blue eyes.", "General")]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "eyes", provider, _TOKEN
            )

        self.assertEqual(len(suggestions), 1)
        self.assertEqual(suggestions[0].style, "muted")
        self.assertEqual(suggestions[0].description, "(已存在) Blue eyes.")

    def test_tag_suggestion_keeps_illegal_filename_characters(self):
        """含 Windows 非法文件名字符的候选原样给出，不过滤、不替换。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            provider = _fake_tag_provider([DanbooruTag("16:9", "", "", "Meta")])

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "16", provider, _TOKEN
            )

        self.assertEqual(suggestions[0].text, "16:9")

    def test_semantic_unavailable_keeps_literal_results_and_adds_error_item(self):
        """语义层不可用不中断补全：字面结果照常返回，另附一条不可插入的提示项。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            provider = _fake_tag_provider(
                [
                    DanbooruTag("blue eyes", "蓝眼", "Blue eyes.", "General"),
                    SemanticLayerUnavailable(reason="嵌入接口不可用"),
                ]
            )

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "blue", provider, _TOKEN
            )

        self.assertEqual(len(suggestions), 2)
        self.assertEqual(suggestions[0].text, "blue_eyes")
        self.assertEqual(suggestions[1].text, "")
        self.assertEqual(suggestions[1].type, "error")
        self.assertIn("语义搜索不可用", suggestions[1].displayText)
        self.assertEqual(suggestions[1].description, "嵌入接口不可用")

    def test_tag_search_failure_yields_error_item_after_directories(self):
        """标签搜索失败以错误提示项呈现，整次补全不失败。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))
            provider = _fake_tag_provider(error=HttpError("boom"))

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "a", provider, _TOKEN
            )

        self.assertEqual([s.text for s in suggestions], ["alpha", ""])
        self.assertEqual(suggestions[1].type, "error")
        self.assertIn("Danbooru 搜索失败", suggestions[1].displayText)
        self.assertEqual(suggestions[1].description, "boom")

    def test_without_provider_only_directory_suggestions(self):
        """未配置任何数据来源时只给目录候选，不报错、不出现错误提示项。"""
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "alpha"))

            suggestions = get_branch_autocomplete_suggestions(
                temp_dir, "", "alpha", None, _TOKEN
            )

        self.assertEqual([s.text for s in suggestions], ["alpha"])
        self.assertEqual({s.type for s in suggestions}, {"directory"})

    def test_run_autocomplete_wires_danbooru_data_dir_from_env(self):
        """环境接线：DANBOORU_DATA_DIR 指向本地编译产物时给出标签候选（离线）。"""
        data_dir = tempfile.mkdtemp()
        try:
            write_file_fixture(
                data_dir,
                [
                    {
                        "name": "blue_eyes",
                        "cn_name": "蓝眼",
                        "wiki": "Blue eyes.",
                        "post_count": "100",
                        "category": "0",
                        "nsfw": "0",
                    }
                ],
                [],
            )
            with tempfile.TemporaryDirectory() as temp_dir:
                with patch.dict(
                    os.environ,
                    {
                        "IMAGE_FUNNEL_ROOT_DIR": temp_dir,
                        "IMAGE_FUNNEL_DIRECTORY_REL_PATH": "",
                        "IMAGE_FUNNEL_AUTOCOMPLETE_QUERY": "blue",
                        "IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS": '["/branch"]',
                        "IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD": "/branch",
                        "DANBOORU_DATA_DIR": data_dir,
                        "DANBOORU_SEARCH_URL": "",
                        "IMAGE_FUNNEL_DATA_DIR": "",
                    },
                    clear=True,
                ):
                    captured_stdout = io.StringIO()
                    with patch.object(sys, "stdout", captured_stdout):
                        run_autocomplete()

                output = captured_stdout.getvalue()
                self.assertIn('"text": "blue_eyes"', output)
                self.assertIn('"type": "danbooru"', output)
                self.assertIn('"displayText": "blue_eyes (蓝眼)"', output)
                self.assertNotIn('"type": "error"', output)
        finally:
            shutil.rmtree(data_dir, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
