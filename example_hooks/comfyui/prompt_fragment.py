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
from typing import List, Optional, Tuple


from .node_accessor import NodeAccessor
from .prompt_locator import get_region_content, find_region_boundaries

_LOGGER = logging.getLogger(__name__)


def strip_comments_for_prompt(text: str) -> str:
    """
    为 prompt 剥离注释行。如果一行去除首尾空白后以 '//' 开头，该整行将被完全过滤掉。
    """
    lines: List[str] = []
    for line in text.splitlines():
        if line.strip().startswith("//"):
            continue
        lines.append(line)
    return "\n".join(lines)


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

    def add(self, prompt_str: str, raw: bool = False, no_skip: bool = False) -> bool:
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
        )

    def remove(
        self,
        prompt_str: str,
        raw: bool = False,
        hard: bool = False,
        no_skip: bool = False,
    ) -> bool:
        """
        从该片段中移除提示词，支持双轨道同步更新。
        返回 True 表示执行了操作，False 表示跳过。
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

        text = strip_comments_for_prompt(text)

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
                self.node_id, "text", strip_comments_for_prompt(new_prompt_text)
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
    ) -> bool:
        """
        对指定节点执行 add/remove 操作，原地修改 prompt 和 workflow 文本。
        region 非空时操作限定在对应 region 内部；为空时操作全部文本。
        返回 True 表示执行了操作，False 表示跳过。
        """
        workflow_text = self.accessor.get_workflow_node_text(node_id)
        if workflow_text is None:
            return False
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

        stripped_workflow = strip_comments_for_prompt(workflow_text)
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

        def contains_prompt(area: str) -> bool:
            target_lower = prompt_str_arg.strip().lower()
            if raw:
                return target_lower in area.lower()
            for line in area.splitlines():
                if line.strip().startswith("//"):
                    continue
                if target_lower in line.lower():
                    return True
            return False

        new_workflow_text = None
        new_prompt_text = None

        if command == "add":
            if region_parts:
                before, region_line, marker_content, end_line, after = region_parts

                if contains_prompt(marker_content) and not no_skip:
                    _LOGGER.debug(
                        "Prompt '%s' already exists in marker region, skipping.",
                        prompt_str_arg,
                    )
                    return False

                stripped = marker_content.strip()
                if stripped:
                    if not stripped.endswith(","):
                        stripped += ","
                    new_content_prompt = f"{stripped}\n{prompt_str_arg},"
                else:
                    new_content_prompt = f"{prompt_str_arg},"

                new_workflow_text = (
                    before.rstrip()
                    + f"\n{region_line}\n"
                    + new_content_prompt
                    + f"\n{end_line}\n"
                    + after.lstrip()
                )

                if is_equivalent:
                    new_prompt_text_raw = (
                        before.rstrip()
                        + "\n"
                        + new_content_prompt
                        + "\n"
                        + after.lstrip()
                    )
                    new_prompt_text = strip_comments_for_prompt(new_prompt_text_raw)
                else:
                    target_match_content = strip_comments_for_prompt(
                        marker_content
                    ).strip()
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
                if contains_prompt(workflow_text) and not no_skip:
                    _LOGGER.debug(
                        "Prompt '%s' already exists in text, skipping.",
                        prompt_str_arg,
                    )
                    return False

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
                        + f"\n{region_line}\n"
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

                if not contains_prompt(marker_content):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in marker region (has_marker=True)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found in marker region, skipping.",
                            prompt_str_arg,
                        )
                        return False
                    new_content_prompt = marker_content
                else:
                    if raw:
                        new_content_prompt = marker_content.replace(prompt_str_arg, "")
                    else:
                        target_lower = prompt_str_arg.strip().lower()
                        lines = marker_content.split("\n")
                        new_lines: List[str] = []
                        for line in lines:
                            if target_lower in line.lower():
                                if effective_hard:
                                    pass
                                else:
                                    stripped = line.strip()
                                    if stripped.startswith("//"):
                                        new_lines.append(line)
                                    else:
                                        indent = line[: len(line) - len(line.lstrip())]
                                        new_lines.append(f"{indent}// {line.lstrip()}")
                            else:
                                new_lines.append(line)
                        new_content_prompt = "\n".join(new_lines)

                new_workflow_text = (
                    before.rstrip()
                    + f"\n{region_line}\n"
                    + new_content_prompt
                    + f"\n{end_line}\n"
                    + after.lstrip()
                )

                if is_equivalent:
                    new_prompt_text_raw = (
                        before.rstrip()
                        + "\n"
                        + new_content_prompt
                        + "\n"
                        + after.lstrip()
                    )
                    new_prompt_text = strip_comments_for_prompt(new_prompt_text_raw)
                else:
                    target_match_content = strip_comments_for_prompt(
                        marker_content
                    ).strip()
                    if target_match_content and target_match_content in prompt_text:
                        new_prompt_text = prompt_text.replace(
                            target_match_content, new_content_prompt.strip(), 1
                        )
                    else:
                        new_prompt_text = prompt_text
            else:
                if not contains_prompt(workflow_text):
                    _LOGGER.debug(
                        "remove: prompt '%s' not found in full text (has_marker=False)",
                        prompt_str_arg,
                    )
                    if not no_skip:
                        _LOGGER.debug(
                            "Prompt '%s' not found, skipping.", prompt_str_arg
                        )
                        return False
                    new_content_prompt = workflow_text
                else:
                    if raw:
                        new_content_prompt = workflow_text.replace(prompt_str_arg, "")
                    else:
                        target_lower = prompt_str_arg.strip().lower()
                        lines = workflow_text.split("\n")
                        new_lines = []
                        for line in lines:
                            if target_lower in line.lower():
                                if effective_hard:
                                    pass
                                else:
                                    stripped = line.strip()
                                    if stripped.startswith("//"):
                                        new_lines.append(line)
                                    else:
                                        indent = line[: len(line) - len(line.lstrip())]
                                        new_lines.append(f"{indent}// {line.lstrip()}")
                            else:
                                new_lines.append(line)
                        new_content_prompt = "\n".join(new_lines)

                new_workflow_text = new_content_prompt
                new_prompt_text = new_content_prompt

        new_prompt_text = strip_comments_for_prompt(new_prompt_text)

        self.accessor.set_prompt_input(node_id, "text", new_prompt_text)
        self.accessor.update_workflow_node_text(node_id, new_workflow_text)
        return True

    @staticmethod
    def _adjust_prompt_weight_in_text(
        text: str, target_prompt: str, new_weight: float
    ) -> Tuple[str, bool]:
        """
        在文本中找到并更新特定提示词的权重为 new_weight。
        支持匹配格式：(word:weight)、(word) 以及裸词 word。
        逐行处理，注释行（// 开头）被跳过不受影响。
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
        lines = text.splitlines(keepends=True)
        new_lines: List[str] = []
        target_lower = target_prompt.lower()

        for line in lines:
            if line.strip().startswith("//"):
                new_lines.append(line)
                continue

            # 粗检查：目标文本不在当前行则跳过正则
            if target_lower not in line.lower():
                new_lines.append(line)
                continue

            if pattern_with_weight.search(line):
                line = pattern_with_weight.sub(f"({target_prompt}:{new_weight})", line)
                modified = True
            elif pattern_with_brackets.search(line):
                line = pattern_with_brackets.sub(
                    f"({target_prompt}:{new_weight})", line
                )
                modified = True
            elif pattern_bare.search(line):
                line = pattern_bare.sub(f"({target_prompt}:{new_weight})", line)
                modified = True

            new_lines.append(line)

        return "".join(new_lines), modified

    # #endregion
