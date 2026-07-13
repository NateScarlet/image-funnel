#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
from dataclasses import dataclass
import argparse
from typing import Dict, List, Tuple, Any, Optional, Iterator, Set, cast
import requests
from PIL import Image

# 从 comfyui 业务脚本中导入现成的 workflow 和 Lora 解析提取逻辑
from .__main__ import (
    extract_region_names,
    extract_lora_names,
    get_parser,
)
from .workflow_prompt_pair import WorkflowPromptPair
from .prompt_fragment import PromptFragment
from .config import ComfyUIConfig

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


def _fetch_danbooru_suggestions(
    query: str, search_url: str
) -> Iterator[AutocompleteSuggestion]:
    if not query.strip():
        return

    search_url = search_url.rstrip("/")
    api_url = f"{search_url}/api/search"

    show_nsfw_env = os.getenv("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower()
    show_nsfw = show_nsfw_env in ("true", "1", "yes", "on")

    payload = {
        "query": query,
        "top_k": 20,
        "limit": 20,
        "popularity_weight": 0.15,
        "show_nsfw": show_nsfw,
        "use_segmentation": False,
    }

    _LOGGER.debug(
        "Fetching Danbooru suggestions for query: %r from URL: %r",
        query,
        api_url,
    )
    try:
        response = requests.post(api_url, json=payload)
        _LOGGER.debug("Danbooru response status: %d", response.status_code)
        response.raise_for_status()
        res_json = response.json()
        results = res_json.get("results", [])
        _LOGGER.debug("Danbooru search returned %d items", len(results))
        for item in results:
            tag = item.get("tag", "")
            if not tag:
                continue
            cn_name = item.get("cn_name", "")
            wiki = item.get("wiki", "")

            display = f"{tag} ({cn_name})" if cn_name else tag
            desc = wiki if wiki else "Danbooru 标签"

            # 自动转义 tag 中的括号，避免被 ComfyUI/Stable Diffusion 权重解析器误识别
            escaped_tag = tag.replace("(", r"\(").replace(")", r"\)")

            yield AutocompleteSuggestion(
                text=quote_if_needed(escaped_tag),
                displayText=display,
                description=desc,
                type="danbooru",
            )
    except requests.RequestException as e:
        _LOGGER.warning("Failed to fetch Danbooru suggestions: %s", e, exc_info=True)
        yield AutocompleteSuggestion(
            text="",
            displayText="⚠ Danbooru 搜索失败",
            description=f"{e}",
            type="error",
            style="",
        )


def _fetch_danbooru_related(
    tags: List[str], search_url: str
) -> Iterator[AutocompleteSuggestion]:
    if not tags:
        return

    search_url = search_url.rstrip("/")
    api_url = f"{search_url}/api/related"

    show_nsfw_env = os.getenv("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower()
    show_nsfw = show_nsfw_env in ("true", "1", "yes", "on")

    payload = {
        "tags": tags,
        "limit": 20,
        "show_nsfw": show_nsfw,
    }

    try:
        _LOGGER.debug(
            "Fetching Danbooru related tags for: %r from URL: %r", tags, api_url
        )
        response = requests.post(api_url, json=payload)
        _LOGGER.debug("Danbooru related response status: %d", response.status_code)
        response.raise_for_status()
        res_json = response.json()
        results = res_json.get("results", [])
        _LOGGER.debug("Danbooru related search returned %d items", len(results))
        for item in results:
            tag = item.get("tag", "")
            if not tag:
                continue
            cn_name = item.get("cn_name", "")
            wiki = item.get("wiki", "")

            display = f"{tag} ({cn_name})" if cn_name else tag
            desc = wiki if wiki else "Danbooru 关联标签"

            # 自动转义 tag 中的括号，避免被 ComfyUI/Stable Diffusion 权重解析器误识别
            escaped_tag = tag.replace("(", r"\(").replace(")", r"\)")

            yield AutocompleteSuggestion(
                text=quote_if_needed(escaped_tag),
                displayText=display,
                description=desc,
                type="danbooru",
            )
    except requests.RequestException as e:
        _LOGGER.warning("Failed to fetch Danbooru related tags: %s", e, exc_info=True)
        yield AutocompleteSuggestion(
            text="",
            displayText="⚠ Danbooru 搜索失败",
            description=f"{e}",
            type="error",
            style="",
        )


def _parse_args_for_autocomplete(
    target_command: Optional[str],
    cleaned_cwords: List[str],
    is_adjust_prompt_cmd: bool,
    query: str,
) -> Optional[argparse.Namespace]:
    args_to_parse = (
        [target_command] + cleaned_cwords[1:] + ["dummy_prompt", "dummy_weight"]
        if target_command
        else cleaned_cwords + ["dummy_prompt", "dummy_weight"]
    )
    try:
        parser = get_parser()
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
        target_command: Optional[str],
        *,
        query: str = "",
        prev_word: str = "",
        cwords: Optional[List[str]] = None,
        image_paths: Optional[List[str]] = None,
        parsed_args: Optional[argparse.Namespace] = None,
        seen_prompts: Optional[Dict[str, str]] = None,
        workflow: Optional[Dict[str, Any]] = None,
        prompt_meta: Optional[Dict[str, Any]] = None,
    ):
        self.target_command: Optional[str] = target_command
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

        # 动态解析选项参数
        parser = get_parser()
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

            # 获取节点 CLIP 文本内容作为描述
            clip_text = ""
            if isinstance(inputs, dict):
                inputs_dict = cast(Dict[str, Any], inputs)
                text_val = inputs_dict.get("text")
                if isinstance(text_val, str):
                    clip_text = text_val
                elif text_val is not None:
                    clip_text = str(cast(object, text_val))

            description = clip_text

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


class DanbooruProvider(AutocompleteProvider):
    """Danbooru 语义与关联补全推荐"""

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
        danbooru_url = os.getenv("DANBOORU_SEARCH_URL", "").strip()
        if not danbooru_url:
            return

        def is_in_workflow(text: str) -> bool:
            cleaned_text = (
                text.strip('"').strip("'").replace(r"\(", "(").replace(r"\)", ")")
            )
            return cleaned_text in context.seen_prompts

        if context.query.strip():
            # 用户正在打字，执行前缀语义搜索
            for s in _fetch_danbooru_suggestions(context.query, danbooru_url):
                if is_in_workflow(s.text):
                    s.style = "muted"
                yield s
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
                for s in _fetch_danbooru_related(prompt_tags, danbooru_url):
                    if is_in_workflow(s.text):
                        s.style = "muted"
                    yield s


def autocomplete(
    target_command: Optional[str] = None,
    *,
    query: str = "",
    prev_word: str = "",
    cwords: Optional[List[str]] = None,
    image_paths: Optional[List[str]] = None,
) -> Iterator[AutocompleteSuggestion]:
    """
    autocomplete 子命令：读取环境变量，生成自动完成建议。
    调用方（main）负责输出 JSONL。
    """
    if not query:
        query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")

    if not prev_word:
        prev_word = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD", "")

    if cwords is None:
        cwords_str = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS", "[]")
        try:
            cwords = cast(List[str], json.loads(cwords_str))
        except json.JSONDecodeError as e:
            _LOGGER.error("Failed to parse IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS: %s", e)
            raise

    if image_paths is None:
        image_paths_str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "[]")
        try:
            image_paths = cast(List[str], json.loads(image_paths_str))
        except json.JSONDecodeError as e:
            _LOGGER.error("Failed to parse IMAGE_FUNNEL_IMAGE_PATHS: %s", e)
            raise

    cleaned_cwords = [w for w in cwords if not (w.startswith("<") and w.endswith(">"))]
    is_adjust_prompt_cmd = target_command == "adjust" and "prompt" in cleaned_cwords

    parsed_args = _parse_args_for_autocomplete(
        target_command, cleaned_cwords, is_adjust_prompt_cmd, query
    )

    seen_prompts, workflow, prompt_meta = _load_workflow_data(image_paths, parsed_args)

    context = AutocompleteContext(
        target_command=target_command,
        query=query,
        prev_word=prev_word,
        cwords=cwords,
        image_paths=image_paths,
        parsed_args=parsed_args,
        seen_prompts=seen_prompts,
        workflow=workflow,
        prompt_meta=prompt_meta,
    )
    providers: List[AutocompleteProvider] = [
        RegionProvider(),
        LoraProvider(),
        NodeProvider(),
        WorkflowPromptProvider(),
        DanbooruProvider(),
    ]

    for provider in providers:
        if provider.can_provide(context):
            has_suggestions = False
            for suggestion in provider.provide(context):
                has_suggestions = True
                yield suggestion
            if has_suggestions and provider.is_exclusive:
                break


def main() -> None:
    config = ComfyUIConfig.from_env()
    log_level = getattr(logging, config.logging_level, logging.WARNING)
    logging.basicConfig(
        level=log_level,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    if len(sys.argv) > 1:
        target_cmd = sys.argv[1]
        for s in autocomplete(target_cmd):
            print(
                json.dumps(
                    {
                        "text": s.text,
                        "displayText": s.displayText,
                        "description": s.description,
                        "type": s.type,
                        "style": s.style,
                    },
                    ensure_ascii=False,
                )
            )
        sys.exit(0)
    else:
        _LOGGER.error("Missing target command for autocomplete.")
        sys.exit(1)


if __name__ == "__main__":
    main()
