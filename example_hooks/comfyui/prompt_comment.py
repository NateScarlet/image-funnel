# -*- coding: utf-8 -*-
r"""提示词注释词法：全包唯一的「什么算注释」判断。

格式推理、文本格式化、提示词增删改查与补全都从这里取注释边界：各写一套正则
正是注释口径曾经漂移的原因。词法对齐 ComfyUI 上游 stripComments：

    str.replace(/\/\*[\s\S]*?\*\/|\/\/.*/g, '')

- /* ... */ 块注释整段消失：可跨行、非贪婪匹配最近的 */、不支持嵌套
- // 行注释从出现位置到该行行尾；上游的 . 视 \n \r \u2028 \u2029 为行终止符，
  这里照抄同一组

未闭合的 /* 不被识别为注释：不产生注释区段，因此仍按普通文本参与后续处理。
"""

import re
from typing import Iterator, NamedTuple

_COMMENT_PATTERN = re.compile("/\\*[\\s\\S]*?\\*/|//[^\\r\\n\\u2028\\u2029]*")


class PromptSegment(NamedTuple):
    """提示词文本的一段。is_comment 标记该段是否被词法识别为注释。"""

    is_comment: bool
    text: str


def iter_prompt_segments(text: str) -> Iterator[PromptSegment]:
    """把提示词文本切成注释区段与非注释区段。

    唯一不变式：按序拼接所有区段即原文，因此调用方可以逐段处理后再拼回。
    """
    pos = 0
    for match in _COMMENT_PATTERN.finditer(text):
        if match.start() > pos:
            yield PromptSegment(False, text[pos : match.start()])
        yield PromptSegment(True, match.group(0))
        pos = match.end()
    if pos < len(text):
        yield PromptSegment(False, text[pos:])


def strip_comments(text: str) -> str:
    """剔除全部注释区段（行内 // 至行尾、闭合的 /* ... */），其余文本逐字符保留。"""
    return "".join(
        segment.text for segment in iter_prompt_segments(text) if not segment.is_comment
    )


def is_comment_only(text: str) -> bool:
    """判断一段文本是否整段都是注释（剔除注释后没有剩余内容）。"""
    return not strip_comments(text).strip()
