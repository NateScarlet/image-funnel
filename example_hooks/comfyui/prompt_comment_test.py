# -*- coding: utf-8 -*-
"""prompt_comment.py 的单元测试。"""

import unittest

from .prompt_comment import is_comment_only, iter_prompt_segments, strip_comments

_NEWLINE = "\n"


class TestStripComments(unittest.TestCase):
    def test_strips_whole_line_comments(self):
        text = "hello" + _NEWLINE + "// this is a comment" + _NEWLINE + "world"
        self.assertEqual(strip_comments(text), "hello" + _NEWLINE + _NEWLINE + "world")

    def test_strips_inline_line_comment_to_end_of_line(self):
        self.assertEqual(strip_comments("1girl//blue_eyes"), "1girl")

    def test_keeps_line_terminator_of_line_comment(self):
        """行注释到行尾为止，换行本身不是注释，留在非注释区段里。"""
        self.assertEqual(
            strip_comments("masterpiece// note" + _NEWLINE + "1girl"),
            "masterpiece" + _NEWLINE + "1girl",
        )

    def test_strips_block_comment(self):
        self.assertEqual(strip_comments("blue_eyes/*a b*/"), "blue_eyes")

    def test_block_comment_may_span_lines_and_leaves_no_newline(self):
        self.assertEqual(
            strip_comments("blue_eyes /* multi" + _NEWLINE + "line */ red_hair"),
            "blue_eyes  red_hair",
        )

    def test_block_comment_is_non_greedy(self):
        self.assertEqual(strip_comments("a /* x /* y */ c"), "a  c")

    def test_unclosed_block_comment_is_not_a_comment(self):
        self.assertEqual(strip_comments("a /* not closed b"), "a /* not closed b")

    def test_keeps_text_without_comments_verbatim(self):
        text = "hello" + _NEWLINE + "world, masterpiece" + _NEWLINE
        self.assertEqual(strip_comments(text), text)

    def test_stripping_empty_text(self):
        self.assertEqual(strip_comments(""), "")


class TestIterPromptSegments(unittest.TestCase):
    def test_segments_are_marked_and_reassemble_to_source(self):
        text = "1girl//note" + _NEWLINE + "/* block */red_hair"
        segments = list(iter_prompt_segments(text))
        self.assertEqual(
            [(segment.is_comment, segment.text) for segment in segments],
            [
                (False, "1girl"),
                (True, "//note"),
                (False, _NEWLINE),
                (True, "/* block */"),
                (False, "red_hair"),
            ],
        )
        self.assertEqual("".join(segment.text for segment in segments), text)

    def test_text_without_comment_is_a_single_segment(self):
        segments = list(iter_prompt_segments("masterpiece"))
        self.assertEqual(
            [(False, "masterpiece")], [(s.is_comment, s.text) for s in segments]
        )

    def test_line_comment_swallows_block_comment_on_same_line(self):
        """行注释到行尾为止：同一行后出现的 /* */ 落在行注释内部，不再单独成段。"""
        segments = list(iter_prompt_segments("//a/*b*/"))
        self.assertEqual(
            [(True, "//a/*b*/")],
            [(segment.is_comment, segment.text) for segment in segments],
        )


class TestIsCommentOnly(unittest.TestCase):
    def test_line_comment_only(self):
        self.assertTrue(is_comment_only("// note"))

    def test_block_comment_only(self):
        self.assertTrue(is_comment_only("/* note */"))

    def test_text_with_inline_comment_is_not_comment_only(self):
        self.assertFalse(is_comment_only("1girl//note"))

    def test_empty_text(self):
        self.assertTrue(is_comment_only(""))


if __name__ == "__main__":
    unittest.main()
