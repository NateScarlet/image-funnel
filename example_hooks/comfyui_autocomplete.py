# /// script
# dependencies = [
#   "requests",
#   "Pillow",
# ]
# ///

#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys

# 重新配置标准输出和标准错误流的编码和错误处理，在 Windows 环境下防止 'gbk' 无法编码特定 Unicode 字符抛出 UnicodeEncodeError
if sys.platform.startswith("win"):
    reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
    if reconfigure_stdout is not None:
        try:
            reconfigure_stdout(encoding="utf-8", errors="replace")
        except Exception:
            pass
    reconfigure_stderr = getattr(sys.stderr, "reconfigure", None)
    if reconfigure_stderr is not None:
        try:
            reconfigure_stderr(encoding="utf-8", errors="replace")
        except Exception:
            pass

import json
import logging
from dataclasses import dataclass
import argparse
from typing import Dict, List, Tuple, Any, Optional, Iterator, Set, cast
import requests
from PIL import Image

# 从 comfyui 业务脚本中导入现成的 workflow 和 Lora 解析提取逻辑
from comfyui import (
    resolve_target_to_nodes,
    get_workflow_node_text,
    extract_region_names_from_images,
    _extract_lora_names,  # pyright: ignore[reportPrivateUsage]
    get_parser,
    quote_if_needed,
)

_LOGGER = logging.getLogger(__name__)


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

    payload = {
        "query": query,
        "top_k": 20,
        "limit": 20,
        "popularity_weight": 0.15,
        "show_nsfw": True,
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
    except Exception as e:
        _LOGGER.warning("Failed to fetch Danbooru suggestions: %s", e, exc_info=True)


def _fetch_danbooru_related(
    tags: List[str], search_url: str
) -> Iterator[AutocompleteSuggestion]:
    if not tags:
        return

    search_url = search_url.rstrip("/")
    api_url = f"{search_url}/api/related"

    payload = {
        "tags": tags,
        "limit": 20,
        "show_nsfw": True,
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
    except Exception as e:
        _LOGGER.warning("Failed to fetch Danbooru related tags: %s", e, exc_info=True)


def autocomplete(
    target_command: Optional[str] = None,
) -> Iterator[AutocompleteSuggestion]:
    """
    autocomplete 子命令：读取环境变量，生成自动完成建议。
    调用方（main）负责输出 JSONL。
    """
    query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")

    image_paths_str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "[]")
    try:
        image_paths: List[str] = json.loads(image_paths_str)
    except Exception:
        image_paths = []

    cwords_str = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS", "[]")
    cwords: List[str] = []
    try:
        cwords = json.loads(cwords_str)
    except Exception:
        pass

    # 过滤 Docopt 静态占位符如 <region>, <node-id> 等，避免被误解析为 prompt 位置参数
    cleaned_cwords = [w for w in cwords if not (w.startswith("<") and w.endswith(">"))]

    is_add_cmd = target_command == "add"
    prev_word = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD", "")

    # 动态分析当前活跃命令行路径下的选项参数，避免硬编码
    parser = get_parser()
    active_parsers = _collect_active_parsers(parser, cwords)
    option_with_args: Set[str] = set()
    option_without_args: Set[str] = set()

    for p in active_parsers:
        # pyright: ignore[reportPrivateUsage,reportUnknownMemberType,reportUnknownVariableType]
        for action in p._actions:
            if not action.option_strings:
                continue
            if action.nargs == 0:
                for opt in action.option_strings:
                    option_without_args.add(opt)
            else:
                for opt in action.option_strings:
                    option_with_args.add(opt)

    # 检查 prev_word 是否真的是一个生效的选项参数键
    # 如果 CWORDS 中有 "--" 且该 "--" 位于 prev_word 之前，则它不是生效的选项键，其后输入的也是位置参数
    is_real_option_prev = False
    if prev_word in (option_with_args | option_without_args):
        try:
            is_real_option_prev = "--" not in cleaned_cwords or cleaned_cwords.index(
                "--"
            ) > cleaned_cwords.index(prev_word)
        except ValueError:
            is_real_option_prev = True

    # 1. 保留原本针对 regions 和 Lora 选项的静态/目录级补全建议
    if prev_word == "--region" and is_real_option_prev:
        regions = extract_region_names_from_images(image_paths)
        for r in regions:
            if not query or query.lower() in r.lower():
                yield AutocompleteSuggestion(
                    text=r,
                    displayText=r,
                    description=f"区域: {r}",
                    type="region",
                )
        return

    if prev_word == "lora":
        loras = _extract_lora_names(image_paths)
        for l in loras:
            if not query or query.lower() in l.lower():
                yield AutocompleteSuggestion(
                    text=quote_if_needed(l),
                    displayText=l,
                    description=f"Lora: {l}",
                    type="lora",
                )
        return

    # 2. 尝试解析目标区域并提取 workflow 内已经存在的提示词
    is_remove_cmd = target_command == "remove"
    is_adjust_prompt_cmd = target_command == "adjust" and "prompt" in cleaned_cwords

    seen_prompts: Dict[str, str] = {}
    workflow_loaded = False
    parsed_args = None

    # 白名单设计：仅在支持提示词补全或需要工作流联想的指令下，才解析参数并加载工作流
    need_workflow = (is_remove_cmd or is_adjust_prompt_cmd) or (is_add_cmd and not query.strip())

    if need_workflow:
        args_to_parse = (
            [target_command] + cleaned_cwords[1:] + ["dummy_prompt", "dummy_weight"]
            if target_command
            else cleaned_cwords + ["dummy_prompt", "dummy_weight"]
        )

        try:
            parser = get_parser()
            parsed_args, _ = parser.parse_known_args(args_to_parse)
        except (Exception, SystemExit):
            parsed_args = None

        if is_adjust_prompt_cmd and parsed_args:
            text_val = getattr(parsed_args, "text", None)
            weight_val = getattr(parsed_args, "weight", None)
            if weight_val and weight_val != "dummy_weight":
                parsed_args = None
            elif text_val and text_val != "dummy_prompt" and not query:
                parsed_args = None

        # 从图像加载已存在的提示词
        if parsed_args is not None or is_add_cmd:
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

            # 加载第一张有效图片的 workflow 和 prompt
            workflow = None
            prompt_meta = None
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
                except Exception:
                    continue

            if workflow and prompt_meta:
                nodes_to_process: List[Tuple[str, str, str, bool, str]] = []
                if is_all:
                    clip_nodes = [
                        nid
                        for nid, node in prompt_meta.items()
                        if cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
                    ]
                    for nid in clip_nodes:
                        nodes_to_process.append((nid, "", "", False, f"节点: {nid}"))
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
                        resolved = resolve_target_to_nodes(
                            prompt_meta, workflow, target_type, target_value, is_neg
                        )

                        for nid, start_marker, end_marker, use_markers in resolved:
                            label = (
                                f"区域: {target_value}"
                                if target_type == "region"
                                else f"节点: {target_value}"
                            )
                            nodes_to_process.append(
                                (nid, start_marker, end_marker, use_markers, label)
                            )

                workflow_loaded = True
                for (
                    node_id,
                    start_marker,
                    end_marker,
                    use_markers,
                    label,
                ) in nodes_to_process:
                    workflow_text = get_workflow_node_text(workflow, node_id)
                    if not workflow_text:
                        continue

                    if use_markers:
                        start_idx = workflow_text.find(start_marker)
                        end_idx = workflow_text.find(end_marker)
                        if start_idx != -1 and end_idx != -1 and start_idx < end_idx:
                            content = workflow_text[start_idx + len(start_marker) : end_idx]
                        else:
                            content = workflow_text
                    else:
                        content = workflow_text

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

    # 声明内嵌辅助函数，用于标记样式
    def is_in_workflow(text: str) -> bool:
        cleaned_text = (
            text.strip('"').strip("'").replace(r"\(", "(").replace(r"\)", ")")
        )
        return cleaned_text in seen_prompts

    # 2. 如果是 remove 或 adjust prompt 且处于 prompt 补全阶段，我们建议已存在的提示词项
    if (is_remove_cmd or is_adjust_prompt_cmd) and parsed_args:
        for cleaned, label in sorted(seen_prompts.items()):
            if not query or query.lower() in cleaned.lower():
                yield AutocompleteSuggestion(
                    text=quote_if_needed(cleaned),
                    displayText=cleaned,
                    description=f"来自{label}中的提示词",
                    type="prompt",
                )

    # 3. 如果是 add 命令，我们调用 DanbooruSearch
    if is_add_cmd:
        is_option_input = prev_word.startswith("-")
        # 如果前一个词是已知的不带参数的开关选项，其后依旧应该补全位置参数，因此重置 option 标志
        if prev_word in option_without_args:
            is_option_input = False

        is_real_option_arg_prev = False
        if is_option_input:
            is_real_option_arg_prev = prev_word in option_with_args

        if not is_real_option_arg_prev and not is_option_input:
            danbooru_url = os.getenv("DANBOORU_SEARCH_URL", "").strip()
            if danbooru_url:
                if query.strip():
                    # 用户正在打字，执行前缀语义搜索
                    for s in _fetch_danbooru_suggestions(query, danbooru_url):
                        if is_in_workflow(s.text):
                            s.style = "muted"
                        yield s
                else:
                    # 当前词未输入，执行关联联想
                    prompt_tags = _extract_prompt_tags(cleaned_cwords, option_with_args)
                    if not prompt_tags and workflow_loaded:
                        # 如果前面没有其他提示词，则用目标区域的提示词作为查询 tags！
                        raw_tags: List[str] = []
                        for p in seen_prompts.keys():
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


def main() -> None:
    # 从 HOOK_LOGGING_LEVEL 环境变量读取日志级别，默认 WARNING
    log_level_str = os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper()
    log_level = getattr(logging, log_level_str, logging.WARNING)
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
