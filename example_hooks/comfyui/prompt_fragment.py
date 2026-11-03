#!/usr/bin/env python
# -*- coding: utf-8 -*-
# pyright: reportPrivateUsage=false
"""
prompt_fragment 模块：封装提示词片段的增删改查操作。

提供 PromptFragment 类，调用者通过 add/remove/get_weight/modify_weight 操作提示词文本，
无需关心双轨道（workflow + prompt）同步和 marker 标记的具体处理。
"""

import logging
import re
from dataclasses import dataclass
from typing import List, Optional, Sequence, Tuple

from .node_accessor import NodeAccessor
from .prompt_comment import is_comment_only, iter_prompt_segments, strip_comments
from .prompt_locator import (
    find_region_boundaries,
    get_region_at_line,
    get_region_at_position,
    get_region_content,
)

_LOGGER = logging.getLogger(__name__)


def _blank_lines_at_start(text: str) -> int:
    """统计文本开头连续的空行数量（空行内允许空白字符）。"""
    stripped = text.lstrip()
    leading = text[: len(text) - len(stripped)]
    newline_count = leading.count("\n")
    return max(newline_count - 1, 0) if newline_count else 0


def _blank_lines_at_end(text: str) -> int:
    """统计文本末尾连续的空行数量（空行内允许空白字符）。"""
    stripped = text.rstrip()
    trailing = text[len(stripped) :]
    newline_count = trailing.count("\n")
    return max(newline_count - 1, 0) if newline_count else 0


def _line_matches(line: str, targets: Sequence[str]) -> bool:
    """
    单行是否会被移除命中。命中判定只看非注释内容：注释里提到目标词不算命中，
    整行本就是注释的行保持原样（不算被移除）。
    """
    if is_comment_only(line):
        return False
    plain = strip_comments(line).lower()
    return any(target in plain for target in targets)


def matching_line_indices(text: str, targets: Sequence[str]) -> Tuple[int, ...]:
    """返回会被移除命中的行号；同一行命中多个目标只记一次。"""
    lowered = [target.lower() for target in targets]
    return tuple(
        index
        for index, line in enumerate(text.split("\n"))
        if _line_matches(line, lowered)
    )


def _comment_out_matching_lines(text: str, target_lower: str, hard: bool) -> str:
    """
    把命中 target_lower 的整行注释掉；hard 为真时直接删掉该行。
    """
    new_lines: List[str] = []
    for line in text.split("\n"):
        if not _line_matches(line, (target_lower,)):
            new_lines.append(line)
            continue
        if hard:
            # hard 语义：命中行整行删除，不产出
            continue
        indent = line[: len(line) - len(line.lstrip())]
        new_lines.append(f"{indent}// {line.lstrip()}")
    return "\n".join(new_lines)


def _insert_lines(text: str, index: int, new_lines: Sequence[str]) -> str:
    """在文本第 index 行之前插入若干行（index 等于行数时追加到末尾）。"""
    lines = text.split("\n")
    if not 0 <= index <= len(lines):
        raise ValueError(
            f"Insert line index {index} is out of range for text with {len(lines)} lines"
        )
    lines[index:index] = list(new_lines)
    return "\n".join(lines)


def _contains_prompt(area: str, prompt_str: str, raw: bool) -> bool:
    """目标提示词是否已存在于指定文本：注释不是提示词，先剔除注释区段再判断。"""
    target_lower = prompt_str.strip().lower()
    plain_area = strip_comments(area).lower()
    if raw:
        return target_lower in plain_area
    return any(target_lower in line for line in plain_area.splitlines())


# #region 移除结果


@dataclass(frozen=True)
class InsertAnchor:
    """原地插入的位置锚点。

    line 是插入位置依附的整行文本：after 为 True 时插在它之后，False 时插在它之前；
    line 为 None 表示没有可依附的行，直接追加到文本末尾。index_hint 仅在同内容行出现
    多次时用于选择最接近的候选，不单独作为插入位置依据。
    """

    line: Optional[str]
    after: bool
    index_hint: int


@dataclass(frozen=True)
class RemovalHit:
    """移除实际命中的一行：新提示词行插在该行处。

    region 为空表示命中行位于所有 region 之外。anchor 携带依附行内容，插入时据此在
    当前文本中重新定位，因此移除与插入之间的编辑增删行也不会让位置漂移。
    """

    node_id: str
    region: str
    anchor: InsertAnchor


@dataclass(frozen=True)
class RemovalResult:
    """一次移除的结果。executed 表示未跳过；hits 是实际被注释或删除的提示词行。"""

    executed: bool
    hits: Tuple[RemovalHit, ...] = ()


def _locate_insert_index(text: str, anchor: InsertAnchor, region: str) -> Optional[int]:
    """按依附行重新定位插入位置，且位置必须落在 region 内；否则返回 None。

    同一内容行可能出现在多个 region（各 region 的同名标签、空行尤其常见），只保留
    落在期望 region 的候选，避免把标签插到别的 region。
    """
    lines = text.split("\n")
    if anchor.line is None:
        # 没有可依附的行：追加到末尾，文本以换行结尾时插在结尾空行之前
        position = len(lines) - 1 if lines[-1] == "" else len(lines)
        return position if get_region_at_position(text, position) == region else None

    candidates: List[int] = []
    for index, line in enumerate(lines):
        if line != anchor.line:
            continue
        position = index + 1 if anchor.after else index
        if get_region_at_position(text, position) == region:
            candidates.append(position)
    if not candidates:
        return None
    # 同一 region 内仍有同内容行：优先取最接近原始位置的一处
    return min(candidates, key=lambda pos: (abs(pos - anchor.index_hint), pos))


def _removal_hits(
    node_id: str,
    new_workflow_text: str,
    matched_offsets: Sequence[Tuple[int, str]],
    hard: bool,
) -> Tuple[RemovalHit, ...]:
    """按移除后文本中的命中行位置构造插入锚点。

    非 hard 时命中行已被注释，取该行作为依附行、新行插在它之后；hard 时命中行已删除，
    取紧随其后的一行作为依附行、新行插在它之前；没有后续行时追加到末尾。
    """
    lines = new_workflow_text.split("\n")
    hits: List[RemovalHit] = []
    for offset, region in matched_offsets:
        if offset >= len(lines):
            anchor = InsertAnchor(line=None, after=True, index_hint=offset)
        else:
            anchor = InsertAnchor(
                line=lines[offset],
                after=not hard,
                index_hint=offset if hard else offset + 1,
            )
        hits.append(RemovalHit(node_id=node_id, region=region, anchor=anchor))
    return tuple(hits)


# #endregion


def _region_separators(before: str, after: str) -> Tuple[str, str]:
    """
    计算 region 前后与相邻文本之间的连接符，保留原有空行结构：
    原本有空行则保留一个空行，原本没有则不额外添加；前后无内容时不添加空行。
    """
    if before.strip():
        sep_before = "\n\n" if _blank_lines_at_end(before) >= 1 else "\n"
    else:
        sep_before = ""
    if after.strip():
        sep_after = "\n\n" if _blank_lines_at_start(after) >= 1 else "\n"
    else:
        sep_after = ""
    return sep_before, sep_after


class PromptFragment:
    """
    表示定位出来的一段提示词片段。
    它可能是整个 CLIPTextEncode 节点的文本，也可能是被特定区域标记包裹的局部文本段。
    region 为空表示不使用标记，直接操作全部文本。
    """

    def __init__(
        self,
        accessor: NodeAccessor,
        node_id: str,
        region: str = "",
    ):
        self.accessor = accessor
        self.node_id = node_id
        self.region = region

    @property
    def text(self) -> str:
        """
        获取该片段当前的文本内容（剥离区域 marker）。
        """
        workflow_text = self.accessor.get_workflow_node_text(self.node_id)
        if workflow_text is None:
            return ""
        if self.region:
            content = get_region_content(workflow_text, self.region)
            if content is not None:
                return content
        return workflow_text

    def add(
        self,
        prompt_str: str,
        raw: bool = False,
        no_skip: bool = False,
    ) -> bool:
        """
        往该片段追加提示词，支持双轨道同步更新。
        返回 True 表示执行了操作，False 表示跳过。
        """
        return self._process_double_track(
            self.node_id,
            "add",
            prompt_str,
            self.region,
            raw,
            no_skip,
            hard=False,
        ).executed

    def insert(
        self,
        prompts: Sequence[str],
        anchor: InsertAnchor,
        raw: bool = False,
        no_skip: bool = False,
    ) -> Optional[int]:
        """
        把多个提示词按序插在锚点处，返回实际插入的条数。

        锚点依附的行在当前文本中重新定位（移除与插入之间的编辑可能增删行），因此
        插入位置不会漂移；该行已不存在时返回 None，由调用方回落默认目标。多条提示词
        共用同一次行插入，去重范围与 add 一致，prompt 轨道按剔除注释的工作流文本重建。
        """
        workflow_text = self.accessor.get_workflow_node_text(self.node_id)
        if workflow_text is None:
            return None
        prompt_text = self.accessor.get_prompt_input(self.node_id, "text")
        if not isinstance(prompt_text, str):
            prompt_text = ""

        # 与 add/remove 一致：先按节点相连模型的格式重排标签与全文
        from .model_format import format_text_for_node

        formatted_prompts = [
            format_text_for_node(
                self.accessor.prompt, self.node_id, prompt, prompt_text
            )
            for prompt in prompts
        ]
        workflow_text = format_text_for_node(
            self.accessor.prompt, self.node_id, workflow_text, prompt_text
        )

        insert_at = _locate_insert_index(workflow_text, anchor, self.region)
        if insert_at is None:
            return None

        if self.region:
            content = get_region_content(workflow_text, self.region)
            target_area = content if content is not None else workflow_text
        else:
            target_area = workflow_text

        inserted: List[str] = []
        seen_area = target_area
        for prompt_str in formatted_prompts:
            if not no_skip and _contains_prompt(seen_area, prompt_str, raw):
                _LOGGER.debug(
                    "Prompt '%s' already exists at the insert target, skipping.",
                    prompt_str,
                )
                continue
            inserted.append(prompt_str)
            # 本批前面已插入的标签同样参与去重，避免同一次 --add 内重复写入
            seen_area = f"{seen_area}\n{prompt_str}"
        if not inserted:
            return 0

        new_lines = [prompt_str if raw else f"{prompt_str}," for prompt_str in inserted]
        new_workflow_text = _insert_lines(workflow_text, insert_at, new_lines)
        self.accessor.set_prompt_input(
            self.node_id, "text", strip_comments(new_workflow_text)
        )
        self.accessor.update_workflow_node_text(self.node_id, new_workflow_text)
        return len(inserted)

    def remove(
        self,
        prompt_str: str,
        raw: bool = False,
        hard: bool = False,
        no_skip: bool = False,
    ) -> RemovalResult:
        """
        从该片段中移除提示词，支持双轨道同步更新。
        返回实际被注释或删除的提示词行位置，供同一变体内的新增原地插入。
        """
        return self._process_double_track(
            self.node_id,
            "remove",
            prompt_str,
            self.region,
            raw,
            no_skip,
            hard,
        )

    def get_weight(self, target_prompt: str) -> Optional[float]:
        """
        在当前片段文本中查找目标提示词的权重。
        """
        text = self.text
        if not text:
            return None

        text = strip_comments(text)

        escaped = re.escape(target_prompt)

        # 1. 匹配带权重的格式: (word:1.2)
        pattern_with_weight = re.compile(
            rf"\(\s*{escaped}\s*:\s*([0-9.-]+)\s*\)", re.IGNORECASE
        )
        m = pattern_with_weight.search(text)
        if m:
            return float(m.group(1))

        # 2. 匹配带括号无权重的格式: (word) → 默认权重 1.0
        pattern_brackets = re.compile(rf"\(\s*{escaped}\s*\)", re.IGNORECASE)
        if pattern_brackets.search(text):
            return 1.0

        # 3. 匹配裸词 → 默认权重 1.0
        pattern_bare = re.compile(rf"(?<!\w){escaped}(?!\w)", re.IGNORECASE)
        if pattern_bare.search(text):
            return 1.0

        return None

    def modify_weight(self, target_prompt: str, weight: float, skip_add: bool) -> bool:
        """
        在当前片段中调整目标提示词的权重。
        """
        workflow_text = self.accessor.get_workflow_node_text(self.node_id)
        if workflow_text is None:
            return False

        prompt_text = self.accessor.setdefault_prompt_input(self.node_id, "text", "")
        if not isinstance(prompt_text, str):
            prompt_text = ""

        new_workflow_text, mod_wf = self._adjust_prompt_weight_in_text(
            workflow_text, target_prompt, weight
        )
        new_prompt_text, mod_pr = self._adjust_prompt_weight_in_text(
            prompt_text, target_prompt, weight
        )

        if mod_wf or mod_pr:
            self.accessor.set_prompt_input(
                self.node_id, "text", strip_comments(new_prompt_text)
            )
            self.accessor.update_workflow_node_text(self.node_id, new_workflow_text)
            return True

        if not skip_add:
            added_text = f"({target_prompt}:{weight})"
            self._process_double_track(
                self.node_id,
                "add",
                added_text,
                self.region,
                raw=False,
                no_skip=False,
                hard=False,
            )
            return True

        return False

    # #region 私有方法

    @staticmethod
    def _split_region(
        text: str, region_name: str
    ) -> Optional[Tuple[str, str, str, str, str]]:
        """
        将文本按 region 标记分割。
        返回 (before, region_line, content, end_line, after) 或 None。
        """
        start, endregion_start = find_region_boundaries(text, region_name)
        if start == -1:
            return None

        before = text[:start]
        line_end = text.find("\n", start)
        if line_end == -1:
            line_end = start
        region_line = text[start:line_end]

        # 找到 #endregion 行的结尾
        endregion_line_end = text.find("\n", endregion_start)
        if endregion_line_end == -1:
            endregion_line_end = len(text)
        end_line = text[endregion_start:endregion_line_end]

        content = text[line_end + 1 : endregion_start]

        after = text[endregion_line_end:]

        return (before, region_line, content, end_line, after)

    @staticmethod
    def _make_region_line(region_name: str) -> str:
        """生成 region 起始标记行"""
        return f"// #region {region_name}"

    @staticmethod
    def _make_end_region_line(region_name: str) -> str:
        """生成 region 结束标记行"""
        return f"// #endregion {region_name}"

    def _process_double_track(
        self,
        node_id: str,
        command: str,
        prompt_str_arg: str,
        region: str,
        raw: bool,
        no_skip: bool,
        hard: bool,
    ) -> RemovalResult:
        """
        对指定节点执行 add/remove 操作，原地修改 prompt 和 workflow 文本。
        region 非空时操作限定在对应 region 内部；为空时操作全部文本。
        返回 executed 表示是否执行或按 no_skip 继续，hits 为移除实际命中的行位置
        （add 时为空）。
        """
        workflow_text = self.accessor.get_workflow_node_text(node_id)
        if workflow_text is None:
            return RemovalResult(executed=False)
        prompt_text = self.accessor.get_prompt_input(node_id, "text")
        if not isinstance(prompt_text, str):
            prompt_text = ""

        # add/remove 根据节点连接模型的格式重排新增/移除的标签及双轨道全文。
        # 格式解析需要 IMAGE_FUNNEL_DATA_DIR，缺失时 format_text_for_node 内部触发
        # MissingDataDirError 快速失败（不静默降级）；disabled 作为 opt-out 原样返回。
        if command in ("add", "remove"):
            from .model_format import format_text_for_node

            prompt_str_arg = format_text_for_node(
                self.accessor.prompt, node_id, prompt_str_arg, prompt_text
            )
            workflow_text = format_text_for_node(
                self.accessor.prompt, node_id, workflow_text, prompt_text
            )
            prompt_text = format_text_for_node(
                self.accessor.prompt, node_id, prompt_text, prompt_text
            )

        stripped_workflow = strip_comments(workflow_text)
        workflow_cleaned = "\n".join(
            [line.strip() for line in stripped_workflow.splitlines() if line.strip()]
        )
        prompt_cleaned = "\n".join(
            [line.strip() for line in prompt_text.splitlines() if line.strip()]
        )
        is_equivalent = workflow_cleaned == prompt_cleaned

        # 尝试查找 region 边界
        region_parts = None
        if region:
            region_parts = self._split_region(workflow_text, region)

        new_workflow_text = None
        new_prompt_text = None
        hits: Tuple[RemovalHit, ...] = ()

        if command == "add":
            if region_parts:
                before, region_line, marker_content, end_line, after = region_parts

                if (
                    _contains_prompt(marker_content, prompt_str_arg, raw)
                    and not no_skip
                ):
                    _LOGGER.debug(
                        "Prompt '%s' already exists in marker region, skipping.",
                        prompt_str_arg,
                    )
                    return RemovalResult(executed=False)

                stripped = marker_content.strip()
                if stripped:
                    if not stripped.endswith(","):
                        stripped += ","
                    new_content_prompt = f"{stripped}\n{prompt_str_arg},"
                else:
                    new_content_prompt = f"{prompt_str_arg},"

                sep_before, sep_after = _region_separators(before, after)
                new_workflow_text = (
                    before.rstrip()
                    + sep_before
                    + region_line
                    + "\n"
                    + new_content_prompt
                    + "\n"
                    + end_line
                    + sep_after
                    + after.lstrip()
                )

                if is_equivalent:
                    # prompt 轨道无 region 标记，保持紧凑（单换行）拼接，
                    # 仅在前后无内容时不添加多余换行
                    new_prompt_text_raw = (
                        before.rstrip()
                        + ("\n" if before.strip() else "")
                        + new_content_prompt
                        + ("\n" if after.strip() else "")
                        + after.lstrip()
                    )
                    new_prompt_text = strip_comments(new_prompt_text_raw)
                else:
                    target_match_content = strip_comments(marker_content).strip()
                    if target_match_content and target_match_content in prompt_text:
                        new_prompt_text = prompt_text.replace(
                            target_match_content, new_content_prompt.strip(), 1
                        )
                    else:
                        if raw:
                            new_prompt_text = (
                                prompt_text.rstrip() + "\n" + prompt_str_arg
                            )
                        else:
                            new_prompt_text = (
                                prompt_text.rstrip() + f"\n{prompt_str_arg},"
                            )
            else:
                if _contains_prompt(workflow_text, prompt_str_arg, raw) and not no_skip:
                    _LOGGER.debug(
                        "Prompt '%s' already exists in text, skipping.",
                        prompt_str_arg,
                    )
                    return RemovalResult(executed=False)

                if raw:
                    new_content_prompt = prompt_str_arg
                else:
                    new_content_prompt = f"{prompt_str_arg},"

                if region:
                    # 存在 region 名但文本中无标记 → 创建新标记
                    region_line = self._make_region_line(region)
                    end_line = self._make_end_region_line(region)
                    new_workflow_text = (
                        workflow_text.rstrip()
                        + f"\n\n{region_line}\n"
                        + new_content_prompt
                        + f"\n{end_line}\n"
                    )
                else:
                    new_workflow_text = (
                        workflow_text.rstrip() + "\n" + new_content_prompt
                    )
                new_prompt_text = prompt_text.rstrip() + "\n" + new_content_prompt

        else:  # remove
            effective_hard = hard or raw
            if region_parts:
                before, region_line, marker_content, end_line, after = region_parts
                matched: Tuple[int, ...] = ()

                if not _contains_prompt(marker_content, prompt_str_arg, raw):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in marker region (has_marker=True)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found in marker region, skipping.",
                            prompt_str_arg,
                        )
                        return RemovalResult(executed=False)
                    # 未命中且 no_skip：没有需要改写的行，保持原文不动，
                    # 避免无谓的 region 重排改变原有空行
                    return RemovalResult(executed=True)
                elif raw:
                    new_content_prompt = marker_content.replace(prompt_str_arg, "")
                else:
                    target_lower = prompt_str_arg.strip().lower()
                    matched = matching_line_indices(marker_content, (target_lower,))
                    new_content_prompt = _comment_out_matching_lines(
                        marker_content, target_lower, effective_hard
                    )

                # 内容非空但没有结尾换行（raw 吃掉了换行）时补回，保证标记独占一行
                content_block = new_content_prompt
                if content_block and not content_block.endswith("\n"):
                    content_block += "\n"

                sep_before, sep_after = _region_separators(before, after)
                # 移除只按行改写 region 内容，内容自带的换行结构原样保留，
                # 因此标记行直接衔接内容，不额外插入换行
                new_workflow_text = (
                    before.rstrip()
                    + sep_before
                    + region_line
                    + "\n"
                    + content_block
                    + end_line
                    + sep_after
                    + after.lstrip()
                )

                if matched:
                    # region 内容在重建后文本中的起始行：标记行之后的那一行
                    content_start = (before.rstrip() + sep_before + region_line).count(
                        "\n"
                    ) + 1
                    hits = _removal_hits(
                        node_id,
                        new_workflow_text,
                        [(content_start + index, region) for index in matched],
                        effective_hard,
                    )

                if is_equivalent:
                    # prompt 轨道无 region 标记，保持紧凑（单换行）拼接，
                    # 仅在前后无内容时不添加多余换行
                    new_prompt_text_raw = (
                        before.rstrip()
                        + ("\n" if before.strip() else "")
                        + new_content_prompt
                        + ("\n" if after.strip() else "")
                        + after.lstrip()
                    )
                    new_prompt_text = strip_comments(new_prompt_text_raw)
                else:
                    target_match_content = strip_comments(marker_content).strip()
                    if target_match_content and target_match_content in prompt_text:
                        new_prompt_text = prompt_text.replace(
                            target_match_content, new_content_prompt.strip(), 1
                        )
                    else:
                        new_prompt_text = prompt_text
            else:
                matched: Tuple[int, ...] = ()
                if not _contains_prompt(workflow_text, prompt_str_arg, raw):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in full text (has_marker=False)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found, skipping.", prompt_str_arg
                        )
                        return RemovalResult(executed=False)
                    new_content_prompt = workflow_text
                else:
                    if raw:
                        new_content_prompt = workflow_text.replace(prompt_str_arg, "")
                    else:
                        target_lower = prompt_str_arg.strip().lower()
                        matched = matching_line_indices(workflow_text, (target_lower,))
                        new_content_prompt = _comment_out_matching_lines(
                            workflow_text,
                            target_lower,
                            effective_hard,
                        )

                new_workflow_text = new_content_prompt
                new_prompt_text = new_content_prompt

                if matched:
                    hits = _removal_hits(
                        node_id,
                        new_workflow_text,
                        [
                            (index, get_region_at_line(workflow_text, index))
                            for index in matched
                        ],
                        effective_hard,
                    )

        new_prompt_text = strip_comments(new_prompt_text)

        self.accessor.set_prompt_input(node_id, "text", new_prompt_text)
        self.accessor.update_workflow_node_text(node_id, new_workflow_text)
        return RemovalResult(executed=True, hits=hits)

    @staticmethod
    def _adjust_prompt_weight_in_text(
        text: str, target_prompt: str, new_weight: float
    ) -> Tuple[str, bool]:
        """
        在文本中找到并更新特定提示词的权重为 new_weight。
        支持匹配格式：(word:weight)、(word) 以及裸词 word。
        只改写非注释区段，注释区段原样保留；目标词只出现在注释里时视为未找到。
        返回 (new_text, is_modified)。
        """
        escaped_target = re.escape(target_prompt)

        pattern_with_weight = re.compile(
            rf"\(\s*{escaped_target}\s*:\s*[0-9.-]+\s*\)", re.IGNORECASE
        )
        pattern_with_brackets = re.compile(
            rf"\(\s*{escaped_target}\s*\)", re.IGNORECASE
        )
        pattern_bare = re.compile(rf"(?<!\w){escaped_target}(?!\w)", re.IGNORECASE)

        modified = False
        new_segments: List[str] = []
        target_lower = target_prompt.lower()

        for segment in iter_prompt_segments(text):
            # 注释区段原样保留：目标词只出现在注释里时不算命中
            if segment.is_comment:
                new_segments.append(segment.text)
                continue

            # 粗检查：目标文本不在当前区段则跳过正则
            if target_lower not in segment.text.lower():
                new_segments.append(segment.text)
                continue

            if pattern_with_weight.search(segment.text):
                segment_text = pattern_with_weight.sub(
                    f"({target_prompt}:{new_weight})", segment.text
                )
                modified = True
            elif pattern_with_brackets.search(segment.text):
                segment_text = pattern_with_brackets.sub(
                    f"({target_prompt}:{new_weight})", segment.text
                )
                modified = True
            elif pattern_bare.search(segment.text):
                segment_text = pattern_bare.sub(
                    f"({target_prompt}:{new_weight})", segment.text
                )
                modified = True
            else:
                segment_text = segment.text

            new_segments.append(segment_text)

        return "".join(new_segments), modified

    # #endregion
