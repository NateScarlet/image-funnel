#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
import sqlite3
import threading
from contextlib import contextmanager, ExitStack
from dataclasses import dataclass
from datetime import datetime, timezone
import argparse
from typing import Dict, List, Tuple, Any, Optional, Iterator, Set, Generator, cast
import requests
from PIL import Image

from .db import SQLiteContext
from .danbooru import (
    DanbooruTagProvider,
    AkizukiDanbooruTagProvider,
    SQLiteDanbooruTagProvider,
    AkizukiDanbooruTagLoader,
    SQLiteDanbooruTagLoader,
)

# 从 comfyui 业务脚本中导入现成的 workflow 和 Lora 解析提取逻辑
from .__main__ import (
    extract_region_names,
    extract_lora_names,
    get_parser,
)
from .workflow_prompt_pair import WorkflowPromptPair
from .prompt_fragment import PromptFragment
from .prompt_locator import get_workflow_node_text
from .operation_history import OperationHistory
from .model_format import ModelFormatConfig, collect_inference_texts

_LOGGER = logging.getLogger(__name__)


def quote_if_needed(val: str) -> str:
    if " " in val:
        escaped = val.replace("\\", "\\\\").replace('"', '\\"')
        return f'"{escaped}"'
    return val


@dataclass
class AutocompleteSuggestion:
    text: str
    displayText: str
    description: str
    type: str
    style: str = ""


def _collect_active_parsers(
    parser: argparse.ArgumentParser, cwords: List[str]
) -> List[argparse.ArgumentParser]:
    # 递归收集当前已输入词列表所匹配的所有活跃 ArgumentParser 实例
    active: List[argparse.ArgumentParser] = [parser]
    current: argparse.ArgumentParser = parser

    words_to_match: List[str] = []
    for w in cwords:
        w_clean = w.lstrip("/").strip()
        if w_clean:
            words_to_match.append(w_clean)

    for w in words_to_match:
        found_subparser: Optional[argparse.ArgumentParser] = None
        # pyright: ignore[reportPrivateUsage,reportUnknownMemberType,reportUnknownVariableType]
        for action in current._actions:
            if isinstance(
                action,
                argparse._SubParsersAction,  # pyright: ignore[reportPrivateUsage]
            ):
                if w in action.choices:  # pyright: ignore[reportUnknownMemberType]
                    found_subparser = cast(
                        argparse.ArgumentParser,
                        action.choices[w],  # pyright: ignore[reportUnknownMemberType]
                    )
                    break
        if found_subparser:
            active.append(found_subparser)
            current = found_subparser
    return active


def _extract_prompt_tags(cwords: List[str], option_with_args: Set[str]) -> List[str]:
    # 动态排除命令行选项及其参数，提取可能已输入的提示词 tags
    if not cwords:
        return []

    tags: List[str] = []
    skip_next = False
    for w in cwords[1:]:
        if skip_next:
            skip_next = False
            continue

        if w in option_with_args:
            skip_next = True
            continue

        if w.startswith("-"):
            continue

        cleaned = w.strip().strip(",").strip('"').strip("'").strip()
        if cleaned:
            for part in cleaned.split(","):
                part_cleaned = part.strip()
                if part_cleaned:
                    tags.append(part_cleaned)
    return tags


def _parse_args_for_autocomplete(
    parser: argparse.ArgumentParser,
    target_command: str,
    cleaned_cwords: List[str],
    is_adjust_prompt_cmd: bool,
    query: str,
) -> Optional[argparse.Namespace]:
    # set-model-format 的 format 位置参数带 choices（anima/sdxl/disabled），追加的 dummy 值
    # 必然触发 argparse 校验失败（打印 usage 到 stderr 再 SystemExit）；该命令的补全
    # （ModelFormatProvider）不依赖 parsed_args，直接跳过解析避免 stderr 噪音。
    if target_command == "set-model-format":
        return None

    args_to_parse = (
        [target_command] + cleaned_cwords[1:] + ["dummy_prompt", "dummy_weight"]
        if target_command
        else cleaned_cwords + ["dummy_prompt", "dummy_weight"]
    )
    try:
        parsed_args, _ = parser.parse_known_args(args_to_parse)
    except SystemExit:
        parsed_args = None

    if is_adjust_prompt_cmd and parsed_args:
        text_val = getattr(parsed_args, "text", None)
        weight_val = getattr(parsed_args, "weight", None)
        if weight_val and weight_val != "dummy_weight":
            parsed_args = None
        elif text_val and text_val != "dummy_prompt" and not query:
            parsed_args = None
    return parsed_args


def _load_workflow_data(
    image_paths: List[str],
    parsed_args: Optional[argparse.Namespace],
) -> Tuple[Dict[str, str], Optional[Dict[str, Any]], Optional[Dict[str, Any]]]:
    seen_prompts: Dict[str, str] = {}
    workflow: Optional[Dict[str, Any]] = None
    prompt_meta: Optional[Dict[str, Any]] = None

    for path in image_paths:
        if not os.path.isfile(path):
            continue
        try:
            with Image.open(path) as img:
                p_str = img.info.get("prompt")
                w_str = img.info.get("workflow")
                if p_str and w_str:
                    prompt_meta = json.loads(p_str)
                    workflow = json.loads(w_str)
                    break
        except OSError:
            continue

    if not (workflow and prompt_meta):
        return seen_prompts, workflow, prompt_meta

    is_neg = getattr(parsed_args, "neg", False) if parsed_args else False
    is_all = getattr(parsed_args, "all", False) if parsed_args else False
    regions_raw = getattr(parsed_args, "region", None) if parsed_args else None
    nodes_raw = getattr(parsed_args, "node", None) if parsed_args else None
    regions_arg = (
        cast(List[str], regions_raw)
        if isinstance(regions_raw, list)
        else cast(List[str], [])
    )
    nodes_arg = (
        cast(List[str], nodes_raw)
        if isinstance(nodes_raw, list)
        else cast(List[str], [])
    )

    pair = WorkflowPromptPair(workflow, prompt_meta)
    fragments_to_process: List[Tuple[PromptFragment, str]] = []

    if is_all:
        clip_nodes = [
            nid
            for nid, node in prompt_meta.items()
            if cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
        ]
        for nid in clip_nodes:
            fragments_to_process.append((PromptFragment(pair, nid), f"节点: {nid}"))
    else:
        raw_targets: List[Tuple[str, str]] = []
        for nid in nodes_arg:
            raw_targets.append(("node", nid))
        for rname in regions_arg:
            raw_targets.append(("region", rname))

        if not raw_targets:
            default_region = "negative" if is_neg else "positive"
            raw_targets.append(("region", default_region))

        for target_type, target_value in raw_targets:
            nodes = nodes_arg if target_type == "node" else None
            regions = [target_value] if target_type == "region" else None

            for fragment in pair.locate_prompts(
                nodes=nodes, regions=regions, is_neg=is_neg
            ):
                label = (
                    f"区域: {target_value}"
                    if target_type == "region"
                    else f"节点: {target_value}"
                )
                fragments_to_process.append((fragment, label))

    for fragment, label in fragments_to_process:
        content = fragment.text
        if not content:
            continue

        for line in content.splitlines():
            stripped = line.strip()
            if not stripped:
                continue
            if stripped.startswith("//"):
                continue
            cleaned = stripped.rstrip(",").rstrip("，").strip()
            if not cleaned:
                continue
            if cleaned not in seen_prompts:
                seen_prompts[cleaned] = label

    return seen_prompts, workflow, prompt_meta


class AutocompleteContext:
    """自动完成输入参数和状态上下文"""

    def __init__(
        self,
        target_command: str,
        *,
        query: str = "",
        prev_word: str = "",
        cwords: Optional[List[str]] = None,
        image_paths: Optional[List[str]] = None,
        parsed_args: Optional[argparse.Namespace] = None,
        seen_prompts: Optional[Dict[str, str]] = None,
        workflow: Optional[Dict[str, Any]] = None,
        prompt_meta: Optional[Dict[str, Any]] = None,
        parser: argparse.ArgumentParser,
    ):
        self.target_command: str = target_command
        self.query: str = query
        self.prev_word: str = prev_word
        self.cwords: List[str] = cwords or []
        self.image_paths: List[str] = image_paths or []
        self.cleaned_cwords: List[str] = [
            w for w in self.cwords if not (w.startswith("<") and w.endswith(">"))
        ]

        self.is_add_cmd: bool = target_command == "add"
        self.is_remove_cmd: bool = target_command == "remove"
        self.is_adjust_prompt_cmd: bool = (
            target_command == "adjust" and "prompt" in self.cleaned_cwords
        )

        # 动态解析选项参数（解析器由入口注入）
        active_parsers = _collect_active_parsers(parser, self.cwords)
        self.option_with_args: Set[str] = set()
        self.option_without_args: Set[str] = set()

        for p in active_parsers:
            # pyright: ignore[reportPrivateUsage,reportUnknownMemberType,reportUnknownVariableType]
            for action in p._actions:
                if not action.option_strings:
                    continue
                if action.nargs == 0:
                    for opt in action.option_strings:
                        self.option_without_args.add(opt)
                else:
                    for opt in action.option_strings:
                        self.option_with_args.add(opt)

        self.is_real_option_prev = False
        if self.prev_word in self.option_with_args:
            try:
                self.is_real_option_prev = (
                    "--" not in self.cleaned_cwords
                    or self.cleaned_cwords.index("--")
                    > self.cleaned_cwords.index(self.prev_word)
                )
            except ValueError:
                self.is_real_option_prev = True

        self._parsed_args: Optional[argparse.Namespace] = parsed_args
        self._seen_prompts: Dict[str, str] = seen_prompts or {}
        self._workflow: Optional[Dict[str, Any]] = workflow
        self._prompt_meta: Optional[Dict[str, Any]] = prompt_meta

    @property
    def parsed_args(self) -> Optional[argparse.Namespace]:
        return self._parsed_args

    @property
    def seen_prompts(self) -> Dict[str, str]:
        return self._seen_prompts

    @property
    def workflow_loaded(self) -> bool:
        return self._workflow is not None and self._prompt_meta is not None

    @property
    def workflow(self) -> Optional[Dict[str, Any]]:
        return self._workflow

    @property
    def prompt_meta(self) -> Optional[Dict[str, Any]]:
        return self._prompt_meta


class AutocompleteProvider:
    """自动完成建议提供者基类"""

    @property
    def is_exclusive(self) -> bool:
        """是否独占，如果提供成功则不再继续后续 Provider 补全"""
        return False

    def can_provide(self, context: AutocompleteContext) -> bool:
        raise NotImplementedError

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        raise NotImplementedError


def get_node_title_and_type(
    node_id: str, prompt_meta: Dict[str, Any], workflow: Optional[Dict[str, Any]]
) -> Tuple[str, str]:
    prompt_node = prompt_meta.get(node_id, {})
    class_type = prompt_node.get("class_type", "")

    # 1. 优先从 prompt 的 _meta 拿 title
    title = prompt_node.get("_meta", {}).get("title")
    if title:
        return title, class_type

    if not workflow:
        return class_type or "Unknown Node", class_type

    # 2. 如果是普通顶级节点
    if ":" not in node_id:
        for node in workflow.get("nodes", []):
            if str(node.get("id")) == node_id:
                w_title = node.get("title")
                if w_title:
                    return w_title, class_type
        return class_type or "Unknown Node", class_type

    # 3. 如果是子图节点 (parent_id:child_id)
    parent_id, child_id = node_id.split(":", 1)
    parent_title = parent_id
    parent_type = None

    # 找父节点信息
    for node in workflow.get("nodes", []):
        if str(node.get("id")) == parent_id:
            parent_title = node.get("title") or node.get("type") or parent_id
            parent_type = node.get("type")
            break

    child_title = child_id
    if parent_type:
        subgraphs = workflow.get("definitions", {}).get("subgraphs", [])
        for subgraph in subgraphs:
            if subgraph.get("id") == parent_type:
                for node in subgraph.get("nodes", []):
                    if str(node.get("id")) == child_id:
                        child_title = node.get("title") or node.get("type") or child_id
                        break
                break

    title = f"{parent_title} -> {child_title}"
    return title, class_type


class NodeProvider(AutocompleteProvider):
    """节点 ID 建议提供者"""

    @property
    def is_exclusive(self) -> bool:
        return True

    def can_provide(self, context: AutocompleteContext) -> bool:
        return context.prev_word == "--node" and context.is_real_option_prev

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        if not context.prompt_meta:
            return

        # 找出之前已经用过的节点 ID
        seen_nodes: Set[str] = set()
        if context.parsed_args:
            nodes_val = cast(
                Optional[List[Any]], getattr(context.parsed_args, "node", None)
            )
            if isinstance(nodes_val, list):
                for val in nodes_val:
                    if val:
                        seen_nodes.add(str(val))

        # 兜底：直接从 cleaned_cwords 中提取
        for idx, w in enumerate(context.cleaned_cwords):
            if w == "--node":
                if idx + 1 < len(context.cleaned_cwords):
                    val = context.cleaned_cwords[idx + 1]
                    if val and not val.startswith("-"):
                        seen_nodes.add(val)

        # 确定当前命令支持 of node 类型的过滤逻辑
        is_cfg = "cfg" in context.cleaned_cwords
        is_aspect = "aspect" in context.cleaned_cwords

        for node_id, node_data in sorted(
            context.prompt_meta.items(), key=lambda x: x[0]
        ):
            class_type = node_data.get("class_type", "")
            inputs = node_data.get("inputs", {})

            # 校验是否为 comfyui.py 脚本所支持类型的节点
            supported = False
            if is_cfg:
                supported = "KSampler" in class_type and "cfg" in inputs
            elif is_aspect:
                supported = "width" in inputs and "height" in inputs
            else:
                supported = class_type == "CLIPTextEncode"

            if not supported:
                continue

            # 如果有 query，需要前缀或子串过滤
            if context.query and context.query.lower() not in node_id.lower():
                continue

            # 获取节点标题和类型
            title, _ = get_node_title_and_type(
                node_id, context.prompt_meta, context.workflow
            )

            # 优先使用 workflow 节点文本（含注释），fallback 到 prompt 文本
            description = ""
            if context.workflow:
                wf_text = get_workflow_node_text(context.workflow, node_id)
                if wf_text:
                    description = wf_text
            if not description:
                if isinstance(inputs, dict):
                    text_val = cast(Dict[str, Any], inputs).get("text")
                    if isinstance(text_val, str):
                        description = text_val
                    elif text_val is not None:
                        description = str(cast(object, text_val))

            display_name = f"#{node_id} {title} ({class_type})"

            style = ""
            if node_id in seen_nodes:
                style = "muted"

            yield AutocompleteSuggestion(
                text=node_id,
                displayText=display_name,
                description=description,
                type="node",
                style=style,
            )


class RegionProvider(AutocompleteProvider):
    """区域建议提供者"""

    @property
    def is_exclusive(self) -> bool:
        return True

    def can_provide(self, context: AutocompleteContext) -> bool:
        return context.prev_word == "--region" and context.is_real_option_prev

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        regions: Iterator[str] = (
            extract_region_names([context.workflow]) if context.workflow else iter([])
        )
        for r in regions:
            if not context.query or context.query.lower() in r.lower():
                yield AutocompleteSuggestion(
                    text=r,
                    displayText=r,
                    description=f"区域: {r}",
                    type="region",
                )


class ModelFormatProvider(AutocompleteProvider):
    """/set-model-format 指令参数智能补全：参数 1 推荐 Checkpoint 模型，参数 2 推荐 format 类型。"""

    def __init__(self, config: ModelFormatConfig) -> None:
        # 模型格式配置（含显式映射与默认格式）由入口注入，用于展示各模型当前生效格式
        self.config = config

    @property
    def is_exclusive(self) -> bool:
        return True

    def can_provide(self, context: AutocompleteContext) -> bool:
        # 生产环境 cwords[0] 为 "/set-model-format"（带前导斜杠），当前输入词在 query 中，
        # 因此以去斜杠的 target_command 判定命令，而非直接比对 cleaned_cwords。
        return context.target_command == "set-model-format"

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        # 已完成的参数（正在输入的词在 query 中，不在 cwords 里）
        arg_tokens = context.cleaned_cwords[1:]
        if len(arg_tokens) == 0:
            # 参数 1：Checkpoint 模型名
            seen_models: Set[str] = set()
            if context.prompt_meta:
                for node_raw in context.prompt_meta.values():
                    if isinstance(node_raw, dict):
                        node = cast(Dict[str, Any], node_raw)
                        inputs_raw = node.get("inputs")
                        if isinstance(inputs_raw, dict):
                            inputs = cast(Dict[str, Any], inputs_raw)
                            for k in ("ckpt_name", "model_name", "unet_name"):
                                v = inputs.get(k)
                                if isinstance(v, str) and v.strip():
                                    seen_models.add(v.strip())

            # 逐模型收集提示词文本，用于在无显式配置时推理生效格式
            inference_texts = collect_inference_texts(context.prompt_meta)

            for model_name in sorted(seen_models):
                if not context.query or context.query.lower() in model_name.lower():
                    fmt, source = self.config.resolve_format_with_source(
                        model_name, inference_texts.get(model_name, "")
                    )
                    yield AutocompleteSuggestion(
                        text=quote_if_needed(model_name),
                        displayText=model_name,
                        description=(
                            f"Workflow 中的 Checkpoint 模型 · 当前格式: "
                            f"{fmt} ({source.value})"
                        ),
                        type="model",
                    )
            return

        if len(arg_tokens) == 1:
            # 参数 2：格式类型
            for fmt_option in ("anima", "sdxl", "disabled"):
                if not context.query or context.query.lower() in fmt_option.lower():
                    if fmt_option == "anima":
                        desc = "Danbooru 空格/保留 score_*"
                    elif fmt_option == "sdxl":
                        desc = "SDXL 下划线格式"
                    else:
                        desc = "跳过格式化，保留原始文本"
                    yield AutocompleteSuggestion(
                        text=fmt_option,
                        displayText=fmt_option,
                        description=f"提示词格式: {desc}",
                        type="format",
                    )


class LoraProvider(AutocompleteProvider):
    """Lora 建议提供者"""

    @property
    def is_exclusive(self) -> bool:
        return True

    def can_provide(self, context: AutocompleteContext) -> bool:
        return context.prev_word == "lora"

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        loras: Iterator[str] = (
            extract_lora_names([context.prompt_meta])
            if context.prompt_meta
            else iter([])
        )
        for l in loras:
            if not context.query or context.query.lower() in l.lower():
                yield AutocompleteSuggestion(
                    text=quote_if_needed(l),
                    displayText=l,
                    description=f"Lora: {l}",
                    type="lora",
                )


class WorkflowPromptProvider(AutocompleteProvider):
    """工作流已存在提示词推荐"""

    def can_provide(self, context: AutocompleteContext) -> bool:
        if not (context.is_remove_cmd or context.is_adjust_prompt_cmd):
            return False

        is_option_input = (
            context.query.startswith("-") and "--" not in context.cleaned_cwords
        )
        if is_option_input or context.is_real_option_prev:
            return False

        return context.parsed_args is not None

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        for cleaned, label in sorted(context.seen_prompts.items()):
            if not context.query or context.query.lower() in cleaned.lower():
                yield AutocompleteSuggestion(
                    text=quote_if_needed(cleaned),
                    displayText=cleaned,
                    description=f"来自{label}中的提示词",
                    type="prompt",
                )


def _has_explicit_target(context: AutocompleteContext) -> bool:
    """判断命令行是否已显式指定插入目标（--region 或 --node）。"""
    parsed = context.parsed_args
    if parsed is not None:
        return bool(getattr(parsed, "region", None)) or bool(
            getattr(parsed, "node", None)
        )
    # 解析失败时回退扫描结束标记 -- 之前的选项 token
    for w in context.cleaned_cwords[1:]:
        if w == "--":
            break
        if w in ("--region", "--node"):
            return True
    return False


class RegionOptionProvider(AutocompleteProvider):
    """目标区域选项建议提供者：未指定插入目标时优先建议可用的 --region 选项"""

    @property
    def is_exclusive(self) -> bool:
        return True

    def can_provide(self, context: AutocompleteContext) -> bool:
        if not context.is_add_cmd:
            return False

        # 正在输入查询词时交给语义搜索补全
        if context.query.strip():
            return False

        # 已键入提示词时不建议区域，保持基于已输入标签的关联联想
        if _extract_prompt_tags(context.cleaned_cwords, context.option_with_args):
            return False

        # 目标已确定（--region/--node）或工作流无可用区域时，直接进入关联联想
        if _has_explicit_target(context):
            return False

        workflow = context.workflow
        if not context.workflow_loaded or workflow is None:
            return False

        regions = list(extract_region_names([workflow]))
        return len(regions) > 0

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        # can_provide 已保证工作流加载且存在可用区域
        assert context.workflow is not None
        for r in extract_region_names([context.workflow]):
            yield AutocompleteSuggestion(
                text=f"--region {r}",
                displayText=r,
                description=f"区域: {r}",
                type="region",
            )


def _format_relative_time(created_at: str) -> str:
    """将 ISO 时间戳转为人类可读的相对时间描述"""
    try:
        dt = datetime.fromisoformat(created_at)
    except ValueError:
        return "之前"
    now = datetime.now(timezone.utc)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    diff = now - dt
    seconds = int(diff.total_seconds())
    if seconds < 60:
        return "刚刚"
    minutes = seconds // 60
    if minutes < 60:
        return f"{minutes}分钟前"
    hours = minutes // 60
    if hours < 24:
        return f"{hours}小时前"
    days = hours // 24
    return f"{days}天前"


def _build_seen_lower(seen_prompts: dict[str, str]) -> set[str]:
    """从 seen_prompts 提取所有提示词的小写集合，用于过滤。"""
    return {
        s.strip('"').strip("'").lower()
        for p in seen_prompts
        for s in p.split(",")
        if s.strip()
    }


class DanbooruProvider(AutocompleteProvider):
    """Danbooru 语义与关联补全推荐"""

    def __init__(
        self, provider: DanbooruTagProvider, history: OperationHistory
    ) -> None:
        self.provider = provider
        self.history = history

    def can_provide(self, context: AutocompleteContext) -> bool:
        if not context.is_add_cmd:
            return False

        is_option_input = context.prev_word.startswith("-")
        if context.prev_word in context.option_without_args:
            is_option_input = False

        is_real_option_arg_prev = False
        if is_option_input:
            is_real_option_arg_prev = context.prev_word in context.option_with_args

        return not is_real_option_arg_prev and not is_option_input

    def provide(self, context: AutocompleteContext) -> Iterator[AutocompleteSuggestion]:
        def is_in_workflow(text: str) -> bool:
            cleaned_text = (
                text.strip('"').strip("'").replace(r"\(", "(").replace(r"\)", ")")
            )
            return cleaned_text in context.seen_prompts

        def apply_styles(
            suggestions: List[AutocompleteSuggestion],
            added: Set[str],
            added_times: dict[str, str],
        ) -> Iterator[AutocompleteSuggestion]:
            for s in suggestions:
                if is_in_workflow(s.text):
                    s.style = "muted"
                    s.description = f"(已有) {s.description}"
                elif not s.style and s.text in added:
                    s.style = "muted"
                    created_at = added_times.get(s.text, "")
                    relative = (
                        _format_relative_time(created_at) if created_at else "之前"
                    )
                    s.description = f"({relative}已请求) {s.description}"
                yield s

        # 统一处理历史查询和样式应用的辅助函数
        def _yield_styled(
            suggestions: List[AutocompleteSuggestion],
        ) -> Iterator[AutocompleteSuggestion]:
            added: Set[str] = self.history.get_added_prompts(
                [s.text for s in suggestions]
            )
            added_times = self.history.get_added_prompt_times(
                [s.text for s in suggestions]
            )
            yield from apply_styles(suggestions, added, added_times)

        def _yield_history_suggestions(
            seen_lower: set[str],
            *,
            exclude_lower: set[str] = set(),
        ) -> Iterator[AutocompleteSuggestion]:
            """从操作历史生成标签建议，过滤掉已存在的标签和排除列表中的标签。"""
            try:
                history_prompts = self.history.get_all_added_prompts()
                if not history_prompts:
                    return
                count = 0
                for prompt, created_at in history_prompts:
                    if prompt.lower() in seen_lower or prompt.lower() in exclude_lower:
                        continue
                    relative = (
                        _format_relative_time(created_at) if created_at else "之前"
                    )
                    yield AutocompleteSuggestion(
                        text=quote_if_needed(prompt),
                        displayText=prompt,
                        description=f"({relative}历史添加) {prompt}",
                        type="danbooru",
                        style="muted",
                    )
                    count += 1
                    if count >= 20:
                        break
            except sqlite3.Error as e:
                _LOGGER.warning("Failed to get history prompts: %s", e, exc_info=True)
                yield AutocompleteSuggestion(
                    text="",
                    displayText="⚠ 历史标签获取失败",
                    description=f"{e}",
                    type="error",
                    style="",
                )

        if context.query.strip():
            # 用户正在打字，执行前缀语义搜索
            try:
                tags = self.provider.search(context.query)
                suggestions: List[AutocompleteSuggestion] = []
                for item in tags:
                    display = (
                        f"{item.tag} ({item.cn_name})" if item.cn_name else item.tag
                    )
                    desc = item.wiki if item.wiki else "Danbooru 标签"
                    escaped_tag = item.tag.replace("(", r"\(").replace(")", r"\)")
                    suggestions.append(
                        AutocompleteSuggestion(
                            text=quote_if_needed(escaped_tag),
                            displayText=display,
                            description=desc,
                            type="danbooru",
                        )
                    )
            except requests.RequestException as e:
                _LOGGER.warning(
                    "Failed to provide Danbooru suggestions: %s", e, exc_info=True
                )
                yield AutocompleteSuggestion(
                    text="",
                    displayText="⚠ Danbooru 搜索失败",
                    description=f"{e}",
                    type="error",
                    style="",
                )
                return

            # 完全匹配提升到最前（上游 embedding 排序可能不够准确）
            q_lower = context.query.strip().lower()
            suggestions.sort(
                key=lambda s: (
                    0
                    if s.text.lower().strip('"').strip("'") == q_lower
                    or s.displayText.lower().strip('"').strip("'") == q_lower
                    else 1
                )
            )

            yield from _yield_styled(suggestions)
        else:
            # 当前词未输入，执行关联联想
            prompt_tags = _extract_prompt_tags(
                context.cleaned_cwords, context.option_with_args
            )
            if not prompt_tags and context.workflow_loaded:
                # 如果前面没有其他提示词，则用目标区域的提示词作为查询 tags！
                raw_tags: List[str] = []
                for p in context.seen_prompts.keys():
                    p_cleaned = p.strip().strip(",").strip()
                    if p_cleaned:
                        for part in p_cleaned.split(","):
                            part_cleaned = part.strip()
                            if part_cleaned and len(part_cleaned) < 50:
                                raw_tags.append(part_cleaned)
                prompt_tags = sorted(list(set(raw_tags)))

            if prompt_tags:
                try:
                    target_categories = ["General", "Artist", "Meta"]
                    tags = self.provider.related(
                        prompt_tags, target_categories=target_categories
                    )
                    suggestions: List[AutocompleteSuggestion] = []
                    for item in tags:
                        display = (
                            f"{item.tag} ({item.cn_name})" if item.cn_name else item.tag
                        )
                        desc = item.wiki if item.wiki else "Danbooru 关联标签"
                        escaped_tag = item.tag.replace("(", r"\(").replace(")", r"\)")
                        suggestions.append(
                            AutocompleteSuggestion(
                                text=quote_if_needed(escaped_tag),
                                displayText=display,
                                description=desc,
                                type="danbooru",
                            )
                        )
                        if len(suggestions) >= 20:
                            break
                except requests.RequestException as e:
                    _LOGGER.warning(
                        "Failed to provide Danbooru related suggestions: %s",
                        e,
                        exc_info=True,
                    )
                    yield AutocompleteSuggestion(
                        text="",
                        displayText="⚠ Danbooru 关联搜索失败",
                        description=f"{e}",
                        type="error",
                        style="",
                    )
                    return

                yield from _yield_styled(suggestions)

                # 作为关联联想的补充：产出历史标签建议，过滤掉关联联想已显示的标签
                related_lower = {
                    s.text.strip('"').strip("'").lower() for s in suggestions
                }
                yield from _yield_history_suggestions(
                    _build_seen_lower(context.seen_prompts),
                    exclude_lower=related_lower,
                )
            else:
                # 没有关联查询标签时，从操作历史中获取之前添加过的标签
                yield from _yield_history_suggestions(
                    _build_seen_lower(context.seen_prompts)
                )


@dataclass(frozen=True)
class AutocompleteRequest:
    """一次自动补全请求的完整上下文（由入口构造并注入，不读环境变量）。"""

    target_command: str
    query: str
    prev_word: str
    cwords: List[str]
    image_paths: List[str]
    root_dir: str
    directory_rel_path: str


@dataclass(frozen=True)
class AutocompleteServices:
    """自动补全依赖（由最外层入口构建并注入）：解析器与补全 provider 列表。"""

    parser: argparse.ArgumentParser
    providers: List[AutocompleteProvider]


def _db_path(root_dir: str, directory_rel_path: str) -> str:
    """目录级 SQLite 数据库文件路径（与 SQLiteContext.from_env 的推导一致）。"""
    return os.path.join(root_dir, directory_rel_path, ".io.github.natescarlet.hook.db")


def _show_nsfw_from_env() -> bool:
    """从 spawn 注入的环境变量读取 Danbooru 是否包含 NSFW 的静态配置（入口职责）。"""
    return os.environ.get("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower() in (
        "true",
        "1",
        "yes",
        "on",
    )


def build_request_from_env(target_command: str) -> AutocompleteRequest:
    """单次模式：入口从环境变量构造请求上下文（缺失环境变量即报错中止，快速失败）。"""
    return AutocompleteRequest(
        target_command=target_command,
        query=os.environ["IMAGE_FUNNEL_AUTOCOMPLETE_QUERY"],
        prev_word=os.environ["IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD"],
        cwords=cast(
            List[str], json.loads(os.environ["IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS"])
        ),
        image_paths=cast(List[str], json.loads(os.environ["IMAGE_FUNNEL_IMAGE_PATHS"])),
        root_dir=os.environ["IMAGE_FUNNEL_ROOT_DIR"],
        directory_rel_path=os.environ["IMAGE_FUNNEL_DIRECTORY_REL_PATH"],
    )


def build_request_from_params(params: Dict[str, Any]) -> AutocompleteRequest:
    """serve 模式：入口从 JSON-RPC 请求参数构造请求上下文。"""
    cwords = cast(List[str], params.get("cwords", []) or [])
    target_command = ""
    if cwords:
        first = cwords[0].lstrip("/").strip()
        if first:
            target_command = first
    return AutocompleteRequest(
        target_command=target_command,
        query=cast(str, params.get("query", "") or ""),
        prev_word=cast(str, params.get("prevWord", "") or ""),
        cwords=cwords,
        image_paths=cast(List[str], params.get("imagePaths", []) or []),
        root_dir=cast(str, params.get("rootDir", "") or ""),
        directory_rel_path=cast(str, params.get("directoryRelPath", "") or ""),
    )


@contextmanager
def build_providers(
    root_dir: str,
    directory_rel_path: str,
    danbooru_url: str,
    show_nsfw: bool,
    target_command: str,
) -> Generator[List[AutocompleteProvider], None, None]:
    """由入口构建补全 provider 列表，并用 ExitStack 管理底层的 DB 上下文生命周期。

    target_command 用于按指令按需构建轻量依赖：仅 /set-model-format 需要加载
    模型格式配置，从而避免其他指令的补全因缺失 IMAGE_FUNNEL_DATA_DIR 而整体失败。
    """
    with ExitStack() as stack:
        providers: List[AutocompleteProvider] = []
        if target_command == "set-model-format":
            providers.append(ModelFormatProvider(ModelFormatConfig.load()))
        providers.extend(
            [
                RegionProvider(),
                LoraProvider(),
                NodeProvider(),
                WorkflowPromptProvider(),
                RegionOptionProvider(),
            ]
        )
        if danbooru_url:
            db_ctx = stack.enter_context(
                SQLiteContext(_db_path(root_dir, directory_rel_path))
            )
            history = OperationHistory(db_ctx)
            raw_loader = AkizukiDanbooruTagLoader(danbooru_url)
            cache_loader = SQLiteDanbooruTagLoader(raw_loader, db_ctx)
            akizuki = AkizukiDanbooruTagProvider(
                danbooru_url, loader=cache_loader, show_nsfw=show_nsfw
            )
            danbooru_tag_provider = SQLiteDanbooruTagProvider(
                akizuki, db_ctx, danbooru_url
            )
            providers.append(DanbooruProvider(danbooru_tag_provider, history))
        yield providers


def autocomplete(
    request: AutocompleteRequest,
    services: AutocompleteServices,
) -> Iterator[AutocompleteSuggestion]:
    """生成自动完成建议：上下文与依赖全部由调用方（入口）注入，自身不读取任何环境变量。"""
    cleaned_cwords = [
        w for w in request.cwords if not (w.startswith("<") and w.endswith(">"))
    ]
    is_adjust_prompt_cmd = (
        request.target_command == "adjust" and "prompt" in cleaned_cwords
    )

    parsed_args = _parse_args_for_autocomplete(
        services.parser,
        request.target_command,
        cleaned_cwords,
        is_adjust_prompt_cmd,
        request.query,
    )

    seen_prompts, workflow, prompt_meta = _load_workflow_data(
        request.image_paths, parsed_args
    )

    context = AutocompleteContext(
        target_command=request.target_command,
        query=request.query,
        prev_word=request.prev_word,
        cwords=request.cwords,
        image_paths=request.image_paths,
        parsed_args=parsed_args,
        seen_prompts=seen_prompts,
        workflow=workflow,
        prompt_meta=prompt_meta,
        parser=services.parser,
    )

    for provider in services.providers:
        if provider.can_provide(context):
            has_suggestions = False
            for suggestion in provider.provide(context):
                has_suggestions = True
                yield suggestion
            if has_suggestions and provider.is_exclusive:
                break


# JSON-RPC 常驻补全协议方法名
JSON_RPC_METHOD = "autocomplete"
JSON_RPC_CANCEL_METHOD = "$/cancelRequest"
# stdin 关闭后等待进行中任务收尾的时长
_SERVE_SHUTDOWN_TIMEOUT = 2.0
# 常驻模式下多个请求线程并发写 stdout，需要串行化保证响应行不交错
_SERVE_WRITE_LOCK = threading.Lock()


def _suggestion_to_dict(s: AutocompleteSuggestion) -> Dict[str, str]:
    """将建议项序列化为前端建议结构（JSONL 与 JSON-RPC result 共用）。"""
    return {
        "text": s.text,
        "displayText": s.displayText,
        "description": s.description,
        "type": s.type,
        "style": s.style,
    }


def _write_response(
    writer: Any, req_id: Any, suggestions: List[AutocompleteSuggestion]
) -> None:
    """将建议序列化为 JSON-RPC 响应写入 stdout（多线程下串行写，避免行交错）。"""
    result = [_suggestion_to_dict(s) for s in suggestions]
    with _SERVE_WRITE_LOCK:
        writer.write(
            json.dumps(
                {"jsonrpc": "2.0", "id": req_id, "result": result}, ensure_ascii=False
            )
            + "\n"
        )
        writer.flush()


def _write_error_response(writer: Any, req_id: Any, message: str) -> None:
    """将请求失败以 JSON-RPC 错误响应上报给调用方（快速失败，不静默返回空建议）。"""
    with _SERVE_WRITE_LOCK:
        writer.write(
            json.dumps(
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {"code": -32000, "message": message},
                },
                ensure_ascii=False,
            )
            + "\n"
        )
        writer.flush()


class _AutocompleteTask:
    """单个自动补全请求的任务：独立线程运行，支持取消标记（尽力而为中断）。"""

    def __init__(
        self,
        req_id: Any,
        request: AutocompleteRequest,
        parser: argparse.ArgumentParser,
        danbooru_url: str,
        show_nsfw: bool,
        writer: Any,
        active: Dict[Any, "_AutocompleteTask"],
        active_lock: threading.Lock,
    ) -> None:
        self.req_id = req_id
        self.request = request
        self.parser = parser
        self.danbooru_url = danbooru_url
        self.show_nsfw = show_nsfw
        self.writer = writer
        self.active = active
        self.active_lock = active_lock
        self._canceled = threading.Event()
        self._thread = threading.Thread(
            target=self._run, name=f"autocomplete-{req_id}", daemon=True
        )

    def cancel(self) -> None:
        """标记取消：响应写入前检查，取消后的请求不再返回结果。"""
        self._canceled.set()

    def start(self) -> None:
        self._thread.start()

    def join(self) -> None:
        self._thread.join(timeout=_SERVE_SHUTDOWN_TIMEOUT)

    def _run(self) -> None:
        suggestions: List[AutocompleteSuggestion] = []
        failed = False
        try:
            with build_providers(
                self.request.root_dir,
                self.request.directory_rel_path,
                self.danbooru_url,
                self.show_nsfw,
                self.request.target_command,
            ) as providers:
                services = AutocompleteServices(parser=self.parser, providers=providers)
                suggestions = list(autocomplete(self.request, services))
        except Exception:
            _LOGGER.error("Autocomplete request %s failed", self.req_id, exc_info=True)
            failed = True
        finally:
            with self.active_lock:
                self.active.pop(self.req_id, None)
        if self._canceled.is_set():
            return
        if failed:
            _write_error_response(
                self.writer, self.req_id, "autocomplete request failed"
            )
            return
        _write_response(self.writer, self.req_id, suggestions)


def serve() -> None:
    """JSON-RPC 常驻补全服务：stdin 读请求，stdout 写响应，直到 stdin 关闭。"""
    active: Dict[Any, _AutocompleteTask] = {}
    active_lock = threading.Lock()

    # 进程级静态配置：从 spawn 注入的环境变量读取一次（最外层入口的职责）
    parser = get_parser()
    danbooru_url = os.environ.get("DANBOORU_SEARCH_URL", "").strip()
    show_nsfw = _show_nsfw_from_env()

    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            continue
        try:
            msg = cast(Dict[str, Any], json.loads(line))
        except json.JSONDecodeError:
            _LOGGER.warning("Ignoring invalid JSON-RPC line: %s", line)
            continue

        if msg.get("method") == JSON_RPC_CANCEL_METHOD:
            cancel_id = msg.get("params", {}).get("id")
            with active_lock:
                task = active.pop(cancel_id, None)
            if task is not None:
                task.cancel()
            continue

        if msg.get("method") != JSON_RPC_METHOD or "id" not in msg:
            continue

        req_id = msg["id"]
        params = cast(Dict[str, Any], msg.get("params", {}) or {})
        request = build_request_from_params(params)
        # 依赖随请求的目录上下文构建；初始化失败在此抛出，进程退出由应用重启（快速失败）
        task = _AutocompleteTask(
            req_id,
            request,
            parser,
            danbooru_url,
            show_nsfw,
            sys.stdout,
            active,
            active_lock,
        )
        with active_lock:
            active[req_id] = task
        task.start()

    # stdin 关闭：等待进行中的任务收尾，避免响应未写完进程就退出
    with active_lock:
        tasks = list(active.values())
    for task in tasks:
        task.join()


def main() -> None:
    if len(sys.argv) > 1:
        if sys.argv[1] == "serve":
            serve()
            sys.exit(0)

        target_cmd = sys.argv[1]

        # 单次模式：入口从环境变量构造请求上下文与依赖（快速失败：缺失配置即报错退出）
        request = build_request_from_env(target_cmd)
        with build_providers(
            request.root_dir,
            request.directory_rel_path,
            os.environ.get("DANBOORU_SEARCH_URL", "").strip(),
            _show_nsfw_from_env(),
            target_cmd,
        ) as providers:
            services = AutocompleteServices(parser=get_parser(), providers=providers)
            for s in autocomplete(request, services):
                print(json.dumps(_suggestion_to_dict(s), ensure_ascii=False))
        sys.exit(0)
    else:
        _LOGGER.error("Missing target command for autocomplete.")
        sys.exit(1)


if __name__ == "__main__":
    main()
