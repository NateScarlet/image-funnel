#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# dependencies = [
#   "Pillow",
#   "requests",
# ]
# ///

import os
import sys

# 重新配置标准输出和标准错误流的编码和错误处理，在 Windows 环境下防止 'gbk' 无法编码特定 Unicode 字符（例如 \ufffd）抛出 UnicodeEncodeError
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
from dataclasses import dataclass
from typing import Dict, List, Tuple, Any, Optional, Set, Iterator, cast
import logging
import argparse
import requests

from graphql_utils import update_image_label, fetch_images
from workflow_prompt_pair import WorkflowPromptPair

_LOGGER = logging.getLogger(__name__)

_DEFAULT_START_REGION_PREFIX = "//#region hook-"
_DEFAULT_END_REGION_PREFIX = "//#endregion hook-"
_START_REGION_PREFIX = os.getenv(
    "HOOK_START_REGION_PREFIX", _DEFAULT_START_REGION_PREFIX
)
_END_REGION_PREFIX = os.getenv("HOOK_END_REGION_PREFIX", _DEFAULT_END_REGION_PREFIX)


def _write_action_override(action: str) -> None:
    """向 IMAGE_FUNNEL_ACTION 文件写入操作覆盖，通知 Runner 跳过默认行为"""
    action_path = os.getenv("IMAGE_FUNNEL_ACTION", "")
    if not action_path:
        raise ValueError("IMAGE_FUNNEL_ACTION environment variable is not set")
    with open(action_path, "w", encoding="utf-8") as f:
        f.write(action)


# 捕获 PIL 导入错误并给出清晰提示
try:
    from PIL import Image
except ImportError:
    print(
        "Error: Missing Pillow library. Please install it in your Python environment to handle image metadata:",
        file=sys.stderr,
    )
    print("      pip install Pillow", file=sys.stderr)
    sys.exit(1)

KNOWN_PRIMITIVE_TYPES = {
    "PrimitiveInt",
    "PrimitiveFloat",
    "PrimitiveString",
    "PrimitiveBoolean",
}
KNOWN_SWITCH_TYPES = {"Any Switch (rgthree)", "ComfySwitchNode"}


def is_node_disabled(node: Dict[str, Any]) -> bool:
    """
    检查节点是否被停用 (Mute/Bypass)。
    mode 值为 2 (Never/Mute) 或 4 (Bypass)。
    """
    return node.get("mode") in (2, 4)


def find_terminal_input(
    prompt: Dict[str, Any], node_id: str, input_key: str
) -> Tuple[str, str]:
    """
    在 prompt 结构中顺着连接线递归向下追溯，直到找到直接存储具体数值的叶子节点输入。
    返回 (src_node_id, src_input_key) 元组。
    """
    node: Dict[str, Any] = prompt.get(node_id, {})
    inputs: Dict[str, Any] = node.get("inputs", {})
    val: Any = inputs.get(input_key)

    # 如果该输入是一个连接，格式形如 [target_node_id, slot_index]
    if isinstance(val, list):
        val_list = cast(List[Any], val)
        if len(val_list) == 2 and isinstance(val_list[0], str):
            target_node_id: str = val_list[0]
            target_node: Dict[str, Any] = prompt.get(target_node_id, {})
            target_class: str = target_node.get("class_type", "")

            # 1. 针对精确匹配的 Primitive 节点，其真实值存储在 inputs.value 中
            if target_class in KNOWN_PRIMITIVE_TYPES:
                return find_terminal_input(prompt, target_node_id, "value")

            # 2. 针对精确匹配的 Switch 路由中转节点，我们只追溯其有效的输入端口
            elif target_class in KNOWN_SWITCH_TYPES:
                if target_class == "ComfySwitchNode":
                    # ComfySwitchNode 的输入中转端口是 on_false 和 on_true
                    for ik in ["on_false", "on_true"]:
                        if ik in target_node.get("inputs", {}):
                            res: Tuple[str, str] = find_terminal_input(
                                prompt, target_node_id, ik
                            )
                            if res:
                                return res
                elif target_class == "Any Switch (rgthree)":
                    # Any Switch (rgthree) 的输入中转端口以 any_ 开头
                    for ik, iv in target_node.get("inputs", {}).items():
                        if ik.startswith("any_"):
                            res: Tuple[str, str] = find_terminal_input(
                                prompt, target_node_id, ik
                            )
                            if res:
                                return res

            # 3. 针对其他可能中转信号的自定义节点，我们继续追溯其列表型端口
            else:
                for ik, iv in target_node.get("inputs", {}).items():
                    iv_list = cast(List[Any], iv) if isinstance(iv, list) else []
                    if len(iv_list) == 2:
                        res: Tuple[str, str] = find_terminal_input(
                            prompt, target_node_id, ik
                        )
                        if res:
                            return res

    return (node_id, input_key)


def convert_comfy_date_format_to_python(comfy_fmt: str) -> Tuple[str, str]:
    """
    将 ComfyUI 的日期占位符格式 (如 yyyyMMdd_hhmmss) 转换为 Python datetime 格式和对应的正则表达式。
    """
    replacements = [
        ("yyyy", "%Y", r"\d{4}"),
        ("yy", "%y", r"\d{2}"),
        ("MM", "%m", r"\d{2}"),
        ("dd", "%d", r"\d{2}"),
        ("hh", "%H", r"\d{2}"),
        ("mm", "%M", r"\d{2}"),
        ("ss", "%S", r"\d{2}"),
    ]
    py_fmt = comfy_fmt
    regex_pattern = comfy_fmt
    for comfy_key, py_val, regex_val in replacements:
        py_fmt = py_fmt.replace(comfy_key, py_val)
        regex_pattern = regex_pattern.replace(comfy_key, regex_val)
    return py_fmt, regex_pattern


def get_workflow_node_text(workflow: Dict[str, Any], node_id_str: str) -> Optional[str]:
    """
    在 UI 结构 workflow 中获取特定节点的文本 widget 数值。
    """

    for node in workflow.get("nodes", []):
        if str(node.get("id")) == node_id_str:
            widgets_values = node.get("widgets_values")
            if isinstance(widgets_values, list) and widgets_values:
                if isinstance(widgets_values[0], str):
                    return widgets_values[0]

    if ":" in node_id_str:
        _, child_id = node_id_str.split(":", 1)
        subgraphs = workflow.get("definitions", {}).get("subgraphs", [])
        for subgraph in subgraphs:
            for node in subgraph.get("nodes", []):
                if str(node.get("id")) == child_id:
                    widgets_values = node.get("widgets_values")
                    if isinstance(widgets_values, list) and widgets_values:
                        if isinstance(widgets_values[0], str):
                            return widgets_values[0]

    return None


def get_region_markers(region_name: str) -> Tuple[str, str]:
    """
    根据区域名称拼装 marker 字符串。
    使用 HOOK_START_REGION_PREFIX / HOOK_END_REGION_PREFIX 环境变量作为前缀，追加区域名。
    """
    return _START_REGION_PREFIX + region_name, _END_REGION_PREFIX + region_name


def find_nodes_with_region(
    prompt: Dict[str, Any], workflow: Dict[str, Any], region_name: str
) -> List[str]:
    """
    查找所有包含指定区域 marker 的 CLIPTextEncode 节点 ID。
    """
    start_marker, end_marker = get_region_markers(region_name)
    result: List[str] = []
    for nid, node in prompt.items():
        node_dict = cast(Dict[str, Any], node)
        if node_dict.get("class_type") == "CLIPTextEncode":
            wf_text = get_workflow_node_text(workflow, nid)
            if wf_text and start_marker in wf_text and end_marker in wf_text:
                result.append(nid)
    return result


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


def find_clip_text_nodes(prompt: Dict[str, Any], start_val: Any) -> List[str]:
    """
    递归追溯 conditioning 链路，找到所有叶子 CLIPTextEncode 节点 ID
    """
    if not isinstance(start_val, list):
        return []

    val_list = cast(List[Any], start_val)
    if len(val_list) != 2:
        return []

    node_id = str(val_list[0])
    node = cast(Dict[str, Any], prompt.get(node_id, {}))
    if not node:
        return []

    class_type = str(node.get("class_type", ""))
    if class_type == "CLIPTextEncode":
        return [node_id]

    results: List[str] = []
    inputs = cast(Dict[str, Any], node.get("inputs", {}))

    if class_type == "ConditioningConcat":
        for k in ["conditioning_to", "conditioning_from"]:
            if k in inputs:
                results.extend(find_clip_text_nodes(prompt, inputs[k]))
    elif class_type == "ConditioningCombine":
        for k in ["conditioning1", "conditioning2"]:
            if k in inputs:
                results.extend(find_clip_text_nodes(prompt, inputs[k]))
    else:
        for k, v in inputs.items():
            if "conditioning" in k.lower() and isinstance(v, list):
                results.extend(find_clip_text_nodes(prompt, v))

    return results


def get_target_clip_node(prompt: Dict[str, Any], is_neg: bool) -> Optional[str]:
    """
    定位目标 CLIPTextEncode 节点。
    优先顺着所有 KSampler 节点的 positive/negative 输入线索追溯；若无，降级为文本最长的 CLIPTextEncode 节点。
    """
    ksampler_ids = [
        nid
        for nid, node in prompt.items()
        if "KSampler" in cast(Dict[str, Any], node).get("class_type", "")
    ]
    clip_nodes: List[str] = []
    target_port = "negative" if is_neg else "positive"

    for kid in ksampler_ids:
        inputs = cast(
            Dict[str, Any], cast(Dict[str, Any], prompt[kid]).get("inputs", {})
        )
        if target_port in inputs:
            clip_nodes.extend(find_clip_text_nodes(prompt, inputs[target_port]))

    clip_nodes = list(set(clip_nodes))

    if not clip_nodes:
        clip_nodes = [
            nid
            for nid, node in prompt.items()
            if cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
        ]

        if not clip_nodes:
            return None

        # 读取环境变量中配置的常见正反向关键词
        keywords_str = os.getenv(
            "HOOK_NEGATIVE_KEYWORDS" if is_neg else "HOOK_POSITIVE_KEYWORDS", ""
        )
        if not keywords_str:
            if is_neg:
                keywords_str = "worst quality, low quality, score_1, score_2, score_3"
            else:
                keywords_str = "masterpiece, best quality, score_7"

        keywords = [k.strip().lower() for k in keywords_str.split(",") if k.strip()]

        best_node_id: Optional[str] = None
        max_matches = -1
        max_len = -1
        for nid in clip_nodes:
            node = cast(Dict[str, Any], prompt.get(nid, {}))
            text_val = cast(Dict[str, Any], node.get("inputs", {})).get("text", "")
            if isinstance(text_val, str):
                text_normalized = text_val.lower().replace("_", " ")
                matches = sum(
                    1 for kw in keywords if kw.replace("_", " ") in text_normalized
                )

                # 优先匹配关键词数量最多的输入，相同时取最长文本
                if matches > max_matches:
                    max_matches = matches
                    max_len = len(text_val)
                    best_node_id = nid
                elif matches == max_matches:
                    if len(text_val) > max_len:
                        max_len = len(text_val)
                        best_node_id = nid

        return best_node_id

    best_node_id: Optional[str] = None
    max_len = -1
    for nid in clip_nodes:
        node = cast(Dict[str, Any], prompt.get(nid, {}))
        text_val = cast(Dict[str, Any], node.get("inputs", {})).get("text", "")
        if isinstance(text_val, str):
            if len(text_val) > max_len:
                max_len = len(text_val)
                best_node_id = nid

    return best_node_id


def resolve_target_to_nodes(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    target_type: str,
    target_value: str,
    is_neg: bool,
) -> List[Tuple[str, str, str, bool]]:
    """
    将单个目标（node 或 region）解析为具体的节点列表。
    返回 [(node_id, start_marker, end_marker, use_markers), ...]。
    """
    if target_type == "node":
        if target_value not in prompt:
            _LOGGER.warning(f"Node {target_value} not found in prompt, skipping.")
            return []
        return [(target_value, "", "", False)]
    else:  # region
        start_marker, end_marker = get_region_markers(target_value)
        matching_nodes = find_nodes_with_region(prompt, workflow, target_value)
        if matching_nodes:
            return [(nid, start_marker, end_marker, True) for nid in matching_nodes]
        else:
            fallback_nid = get_target_clip_node(prompt, is_neg)
            if fallback_nid:
                return [(fallback_nid, start_marker, end_marker, True)]
            else:
                _LOGGER.warning(
                    f"Failed to locate target node for region '{target_value}'"
                )
                return []


def is_under_directory(child_path: str, parent_path: str) -> bool:
    """
    判断 child_path 是否在 parent_path 内部（或为同一路径）。
    支持 Windows 下不区分大小写的包含关系判断。
    """
    child_norm = os.path.normcase(os.path.abspath(child_path))
    parent_norm = os.path.normcase(os.path.abspath(parent_path))
    try:
        common = os.path.normcase(os.path.commonpath([parent_norm, child_norm]))
        return common == parent_norm
    except ValueError:
        return False


def get_relative_output_dir(
    image_path: str, comfyui_output_dir_env: str, hook_output_dir: str
) -> str:
    """
    计算并返回目标输出目录相对于 ComfyUI 输出根目录的相对路径（使用正斜杠）。
    如果路径校验失败，或者找不到根目录，则抛出 ValueError。
    """
    image_path_abs = os.path.abspath(image_path)

    # 1. 寻找 comfyui_output_dir (输出根目录)
    if comfyui_output_dir_env:
        comfyui_output_dir = os.path.abspath(comfyui_output_dir_env)
        # 验证图片是否在 comfyui_output_dir 下
        if not is_under_directory(image_path_abs, comfyui_output_dir):
            raise ValueError(
                f"Image '{image_path}' is not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
            )
    else:
        # 从右向左寻找最后一个名为 "output" 的目录
        curr = os.path.dirname(image_path_abs)
        comfyui_output_dir = None
        while True:
            parent, name = os.path.split(curr)
            if not name:  # 到了根目录
                break
            if name.lower() == "output":
                comfyui_output_dir = curr
                break
            curr = parent

        if not comfyui_output_dir:
            raise ValueError(
                f"Could not find any directory named 'output' in the path of image '{image_path}'"
            )

    # 2. 决定目标目录 target_dir
    if hook_output_dir:
        if os.path.isabs(hook_output_dir):
            # 如果是绝对目录，必须在 comfyui_output_dir 内，否则报错
            target_dir = os.path.abspath(hook_output_dir)
            if not is_under_directory(target_dir, comfyui_output_dir):
                raise ValueError(
                    f"HOOK_OUTPUT_DIR '{hook_output_dir}' is an absolute path but not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
                )
        else:
            # 默认相对于输出目录
            target_dir = os.path.abspath(
                os.path.join(comfyui_output_dir, hook_output_dir)
            )
    else:
        target_dir = os.path.dirname(image_path_abs)

    # 3. 计算相对路径
    rel_dir = os.path.relpath(target_dir, comfyui_output_dir)
    rel_dir = rel_dir.replace("\\", "/")
    return rel_dir


def submit_workflow(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
    jobs: int,
    image_path: str,
) -> Tuple[bool, bool]:
    """
    更新种子和文件名后提交工作流到 ComfyUI。
    返回 (any_success, has_error)。
    """
    pair = WorkflowPromptPair(workflow, prompt)
    any_success = False
    has_error = False
    for q_idx in range(jobs):
        if jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
        if pair.update_seeds() == 0:
            _LOGGER.error(
                f"Failed to update any seeds for image: {image_path}. Cannot queue duplicate workflow without changing seeds."
            )
            has_error = True
            break
        pair.update_output_filenames()
        if pair.submit(comfyui_url):
            any_success = True
        else:
            has_error = True
    return any_success, has_error


def extract_region_names_from_images(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 workflow 元数据中扫描区域标记，逐个 yield 区域名称。
    区域标记由 `_START_REGION_PREFIX` 定义，存储在 CLIPTextEncode 节点的文本 widget 中。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        try:
            with Image.open(path) as img:
                workflow_str = img.info.get("workflow")
                if not workflow_str:
                    continue
                workflow = json.loads(workflow_str)
                for node in workflow.get("nodes", []):
                    widgets_values = node.get("widgets_values")
                    if not isinstance(widgets_values, list):
                        continue
                    widget_values_list: list[Any] = cast("list[Any]", widgets_values)
                    for raw_val in widget_values_list:
                        if not isinstance(raw_val, str):
                            continue
                        val_text: str = raw_val
                        if _START_REGION_PREFIX not in val_text:
                            continue
                        for line in val_text.splitlines():
                            stripped = line.strip()
                            if stripped.startswith(_START_REGION_PREFIX):
                                name = stripped[len(_START_REGION_PREFIX) :].strip()
                                if name and name not in seen:
                                    seen.add(name)
                                    yield name
        except Exception:
            continue


def _extract_lora_names(image_paths: List[str]) -> Iterator[str]:
    """
    从图片的 prompt 元数据中提取所有 lora 文件名（不含扩展名），逐个 yield。
    委托 WorkflowPromptPair.collect_lora_names 处理。
    """
    seen: Set[str] = set()
    for path in image_paths:
        if not os.path.isfile(path):
            continue
        try:
            with Image.open(path) as img:
                prompt_str = img.info.get("prompt")
                if not prompt_str:
                    continue
                prompt_data: Dict[str, Any] = json.loads(prompt_str)
                for n in WorkflowPromptPair.collect_lora_names(prompt_data):
                    name_no_ext, _ = os.path.splitext(n)
                    if name_no_ext not in seen:
                        seen.add(name_no_ext)
                        yield name_no_ext
        except Exception:
            continue


@dataclass
class AutocompleteSuggestion:
    text: str
    displayText: str
    description: str
    type: str


def quote_if_needed(val: str) -> str:
    if " " in val:
        escaped = val.replace("\\", "\\\\").replace('"', '\\"')
        return f'"{escaped}"'
    return val


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
        # 用户指明：不需要超时，应该是前端取消时才取消。
        # 因此，此处直接 requests.post(api_url, json=payload) 而不提供 timeout 参数。
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

            yield AutocompleteSuggestion(
                text=quote_if_needed(tag),
                displayText=display,
                description=desc,
                type="danbooru",
            )
    except Exception as e:
        _LOGGER.warning("Failed to fetch Danbooru suggestions: %s", e, exc_info=True)


def autocomplete(
    target_command: Optional[str] = None,
) -> Iterator[AutocompleteSuggestion]:
    """
    autocomplete 子命令：读取环境变量，生成自动完成建议。
    调用方（main）负责遍历并输出 JSONL。
    """
    query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")

    image_paths_str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "[]")
    try:
        image_paths: List[str] = json.loads(image_paths_str)
    except Exception:
        image_paths = []

    prev_word = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD", "")

    if prev_word == "--region":
        regions = extract_region_names_from_images(image_paths)
        for r in regions:
            if not query or query.lower() in r.lower():
                yield AutocompleteSuggestion(
                    text=r,
                    displayText=r,
                    description=f"区域: {r}",
                    type="region",
                )

    elif prev_word == "lora":
        loras = _extract_lora_names(image_paths)
        for l in loras:
            if not query or query.lower() in l.lower():
                yield AutocompleteSuggestion(
                    text=quote_if_needed(l),
                    displayText=l,
                    description=f"Lora: {l}",
                    type="lora",
                )

    # 针对指令的参数自动完成
    cwords_str = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS", "[]")
    try:
        cwords: List[str] = json.loads(cwords_str)
    except Exception:
        cwords = []

    # 过滤 Docopt 静态占位符如 <region>, <node-id> 等，避免被误解析为 prompt 位置参数
    cleaned_cwords = [w for w in cwords if not (w.startswith("<") and w.endswith(">"))]

    # 检查是否为 remove 或者是 adjust prompt 指令，且当前是补全 prompt 参数的时机
    # 如果已输入了 "--" 区分标志，那么后面的所有输入（即使以 "-" 开头）都是普通 prompt 位置参数，而不是选项本身
    is_remove_cmd = target_command == "remove"
    is_adjust_prompt_cmd = target_command == "adjust" and "prompt" in cleaned_cwords
    is_option_input = query.startswith("-") and "--" not in cleaned_cwords

    # 检查 prev_word 是否真的是一个生效的选项参数键
    # 如果 CWORDS 中有 "--" 且该 "--" 位于 prev_word 之前，则它不是生效的选项键，其后输入的也是位置参数
    is_real_option_prev = False
    if prev_word in [
        "--region",
        "--node",
        "-j",
        "--jobs",
        "--max-match",
        "--skip-add",
        "--neg",
    ]:
        try:
            is_real_option_prev = "--" not in cleaned_cwords or cleaned_cwords.index(
                "--"
            ) > cleaned_cwords.index(prev_word)
        except ValueError:
            is_real_option_prev = True

    if (
        (is_remove_cmd or is_adjust_prompt_cmd)
        and not is_real_option_prev
        and not is_option_input
    ):
        # 将 args_to_parse 构建为以 target_command 为首词，再加上除了指令名之外的其余输入词，最后垫底占位符参数以绕过必需参数校验
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

        # 对于 adjust prompt，如果 text 已经输入完毕，或已开始输入 weight 参数，则跳过提示词自动完成
        if is_adjust_prompt_cmd and parsed_args:
            text_val = getattr(parsed_args, "text", None)
            weight_val = getattr(parsed_args, "weight", None)
            if weight_val and weight_val != "dummy_weight":
                parsed_args = None
            elif text_val and text_val != "dummy_prompt" and not query:
                parsed_args = None

        if parsed_args is None:
            return

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

            seen_prompts: Set[str] = set()
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
                        seen_prompts.add(cleaned)
                        if not query or query.lower() in cleaned.lower():
                            yield AutocompleteSuggestion(
                                text=quote_if_needed(cleaned),
                                displayText=cleaned,
                                description=f"来自{label}中的提示词",
                                type="prompt",
                            )

    is_add_cmd = target_command == "add"
    if is_add_cmd and not is_real_option_prev and not is_option_input:
        danbooru_url = os.getenv("DANBOORU_SEARCH_URL", "").strip()
        if danbooru_url:
            for s in _fetch_danbooru_suggestions(query, danbooru_url):
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

    args = parse_args()
    _LOGGER.debug("args %s", (args,))

    if args.command == "autocomplete":
        target_cmd = getattr(args, "target_command", None)
        for s in autocomplete(target_cmd):
            print(
                json.dumps(
                    {
                        "text": s.text,
                        "displayText": s.displayText,
                        "description": s.description,
                        "type": s.type,
                    },
                    ensure_ascii=False,
                )
            )
        return

    # 解析 --max-match：默认从 HOOK_MAX_MATCH 环境变量读取，未设置则为 4
    max_match = args.max_match
    if max_match is None:
        max_match_str = os.getenv("HOOK_MAX_MATCH", "4")
        max_match = int(max_match_str)  # 非法值让 ValueError 向上传播
    if max_match < 0:
        raise ValueError(f"--max-match must be non-negative, got: {max_match}")

    image_paths_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "")
    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    comfyui_url: str = os.getenv("COMFYUI_URL", "http://127.0.0.1:8188")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    # 过滤器星级检查（针对 add/remove 指令的选做项）
    required_rating_str = os.getenv("HOOK_IMAGE_RATING")
    required_rating: Optional[int] = None
    if required_rating_str:
        required_rating = int(required_rating_str)

    jobs = args.jobs if args.jobs is not None else int(os.getenv("HOOK_JOBS", "1"))

    try:
        image_paths: List[str] = json.loads(image_paths_str) if image_paths_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_PATHS: {e}")
        sys.exit(1)

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS: {e}")
        sys.exit(1)

    # 汇总最终要处理 of (image_id, path) 列表
    targets: List[Tuple[str, str]] = []
    if args.command in ["add", "remove", "adjust"]:
        _LOGGER.debug(f"Command is {args.command}, fetching images via GraphQL...")
        targets = fetch_images(required_rating)
    else:
        # queue 场景
        if image_paths:
            for idx, path in enumerate(image_paths):
                img_id = image_ids[idx] if idx < len(image_ids) else ""
                targets.append((img_id, path))

    if max_match > 0 and len(targets) > max_match:
        _LOGGER.warning(
            "Skipping: matched %d images exceeds --max-match limit of %d",
            len(targets),
            max_match,
        )
        _write_action_override("KEEP")
        sys.exit(0)

    if not targets:
        _write_action_override("KEEP")
        trigger = os.getenv("IMAGE_FUNNEL_TRIGGER", "")
        if not trigger:
            raise ValueError("Environment variable IMAGE_FUNNEL_TRIGGER is missing")
        is_non_manual = trigger not in ["image_dispatch", "note_dispatch"]
        if is_non_manual:
            _LOGGER.info("No images found to process. Skipping.")
            sys.exit(0)
        else:
            _LOGGER.error("No images found to process.")
            sys.exit(1)

    _LOGGER.debug(
        "Found %d image(s) to process, command: %s", len(targets), args.command
    )

    has_errors = False
    success_count = 0

    for idx, (img_id, path) in enumerate(targets):
        if not os.path.exists(path):
            _LOGGER.error(f"File does not exist: {path}")
            has_errors = True
            continue
        _LOGGER.debug("[%d/%d] Processing image: %s", idx + 1, len(targets), path)

        prompt: Dict[str, Any]
        workflow: Dict[str, Any]

        try:
            with Image.open(path) as img:
                info: Any = img.info
                prompt_str: Optional[str] = info.get("prompt")
                workflow_str: Optional[str] = info.get("workflow")

                if not prompt_str:
                    _LOGGER.error(
                        f"This PNG image does not contain prompt metadata from ComfyUI: {path}"
                    )
                    has_errors = True
                    continue

                # 必须同时包含 prompt 且 workflow 有效，否则由于无法更新种子会导致重复生成
                if not workflow_str:
                    _LOGGER.error(
                        f"This PNG image does not contain workflow metadata from ComfyUI: {path}"
                    )
                    has_errors = True
                    continue

                prompt = json.loads(prompt_str)
                workflow_data: Any = json.loads(workflow_str)

                if not isinstance(workflow_data, dict) or "nodes" not in workflow_data:
                    _LOGGER.error(
                        f"This PNG image contains an invalid ComfyUI workflow (missing 'nodes'): {path}"
                    )
                    has_errors = True
                    continue
                workflow = cast(Dict[str, Any], workflow_data)
        except Exception as e:
            _LOGGER.error(f"Failed to read PNG properties: {e}")
            has_errors = True
            continue

        try:
            hook_output_dir = os.getenv("HOOK_OUTPUT_DIR", "")
            if hook_output_dir == ":inherit:":
                _LOGGER.debug(
                    "HOOK_OUTPUT_DIR is ':inherit:', skipping output directory adjustment."
                )
            else:
                comfyui_output_dir_env = os.getenv("COMFYUI_OUTPUT_DIR", "")
                rel_dir = get_relative_output_dir(
                    path, comfyui_output_dir_env, hook_output_dir
                )
                pair = WorkflowPromptPair(workflow, prompt)
                pair.adjust_output_directory(rel_dir)
        except Exception as e:
            _LOGGER.error(f"Failed to adjust output directory: {e}")
            has_errors = True
            continue

        if args.command == "queue":
            any_success, submit_error = submit_workflow(
                prompt, workflow, comfyui_url, jobs, path
            )
            if submit_error:
                has_errors = True
            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

        elif args.command == "adjust":
            node_ids: Optional[List[str]] = getattr(args, "node", None)

            if args.adjust_type == "cfg":
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_cfg_variants(args.weight, node_ids)
            elif args.adjust_type in ("lora", "l"):
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_lora_variants(args.name, args.weight)
            elif args.adjust_type == "aspect":
                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_aspect_variants(args.ratio, node_ids)
            elif args.adjust_type in ("prompt", "p"):
                is_neg = args.neg
                raw_targets = []
                if args.node:
                    for nid in args.node:
                        raw_targets.append(("node", nid))
                if args.region:
                    for rname in args.region:
                        raw_targets.append(("region", rname))
                if not raw_targets:
                    default_region = "negative" if is_neg else "positive"
                    raw_targets.append(("region", default_region))

                target_nodes: List[Tuple[str, str, str, bool]] = []
                for target_type, target_value in raw_targets:
                    target_nodes.extend(
                        resolve_target_to_nodes(
                            prompt, workflow, target_type, target_value, is_neg
                        )
                    )

                pair = WorkflowPromptPair(workflow, prompt)
                variant_gen = pair.generate_prompt_variants(
                    target_nodes, args.text, args.weight, args.skip_add
                )
            else:
                raise ValueError(f"unexpected adjust type '{args.adjust_type}'")

            enable_seed_update = args.update_seed or (jobs > 1)
            any_image_success = False
            variant_count = 0

            for _ in variant_gen:
                variant_count += 1
                if variant_count > 1:
                    enable_seed_update = True

                for q_idx in range(jobs):
                    if jobs > 1 or variant_count > 1:
                        _LOGGER.debug(
                            "  -> Queueing variant %d run %d/%d",
                            variant_count,
                            q_idx + 1,
                            jobs,
                        )
                    if enable_seed_update:
                        if pair.update_seeds() == 0:
                            _LOGGER.error(
                                f"Failed to update any seeds for image: {path}. Cannot queue duplicate workflow without changing seeds."
                            )
                            has_errors = True
                            break
                    pair.update_output_filenames()
                    if pair.submit(comfyui_url):
                        any_image_success = True
                    else:
                        has_errors = True

            if variant_count == 0 and args.no_skip:
                _LOGGER.debug(
                    "No variants generated for image %s, sending original workflow (--no-skip).",
                    path,
                )
                pair = WorkflowPromptPair(workflow, prompt)
                for q_idx in range(jobs):
                    if jobs > 1:
                        _LOGGER.debug(
                            "  -> Queueing original run %d/%d", q_idx + 1, jobs
                        )
                    if enable_seed_update:
                        if pair.update_seeds() == 0:
                            _LOGGER.error(
                                f"Failed to update any seeds for image: {path}. Cannot queue duplicate workflow without changing seeds."
                            )
                            has_errors = True
                            break
                    pair.update_output_filenames()
                    if pair.submit(comfyui_url):
                        any_image_success = True
                    else:
                        has_errors = True
            elif variant_count == 0:
                _LOGGER.debug(
                    "No variants generated for image %s, skipping submission.", path
                )
                _write_action_override("KEEP")

            if any_image_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

        elif args.command in ["add", "remove"]:
            is_neg = args.neg

            # 构建目标列表：(type, value)
            raw_targets: List[Tuple[str, str]] = []
            if args.node:
                for nid in args.node:
                    raw_targets.append(("node", nid))
            if args.region:
                for rname in args.region:
                    raw_targets.append(("region", rname))

            if not raw_targets:
                default_region = "negative" if is_neg else "positive"
                raw_targets.append(("region", default_region))

            hard = getattr(args, "hard", False)
            any_processed = False

            if args.command == "add":
                prompt_str_arg = " ".join(args.prompt)
                pair = WorkflowPromptPair(workflow, prompt)
                for target_type, target_value in raw_targets:
                    nodes = resolve_target_to_nodes(
                        prompt, workflow, target_type, target_value, is_neg
                    )
                    if not nodes:
                        continue
                    # add 只取第一个匹配节点
                    node_id, start_marker, end_marker, use_markers = nodes[0]

                    if pair.process_double_track(
                        node_id,
                        args.command,
                        prompt_str_arg,
                        start_marker,
                        end_marker,
                        args.raw,
                        args.no_skip,
                        hard,
                        use_markers,
                    ):
                        any_processed = True
                        break
                    else:
                        any_processed = True  # 跳过也算成功

                if not any_processed:
                    has_errors = True
                    _LOGGER.error("No target was successfully processed for add.")
                    continue

            else:  # remove
                # 展开所有目标为具体节点列表
                nodes_to_process: List[Tuple[str, str, str, bool]] = []

                if args.all:
                    clip_nodes = [
                        nid
                        for nid, node in prompt.items()
                        if cast(Dict[str, Any], node).get("class_type")
                        == "CLIPTextEncode"
                    ]
                    for nid in clip_nodes:
                        nodes_to_process.append((nid, "", "", False))
                else:
                    for target_type, target_value in raw_targets:
                        nodes_to_process.extend(
                            resolve_target_to_nodes(
                                prompt, workflow, target_type, target_value, is_neg
                            )
                        )

                # 为每个提示词生成原始、下划线、空格三种变体，去重
                remove_prompts: Set[str] = set()
                for p in args.prompt:
                    remove_prompts.update((p, p.replace("_", " "), p.replace(" ", "_")))
                _LOGGER.debug("Remove prompts (variants): %s", remove_prompts)

                pair = WorkflowPromptPair(workflow, prompt)
                for node_id, start_marker, end_marker, use_markers in nodes_to_process:
                    for prompt_str_arg in remove_prompts:
                        _LOGGER.debug(
                            "Trying to remove '%s' from node %s",
                            prompt_str_arg,
                            node_id,
                        )
                        if pair.process_double_track(
                            node_id,
                            args.command,
                            prompt_str_arg,
                            start_marker,
                            end_marker,
                            args.raw,
                            args.no_skip,
                            hard,
                            use_markers,
                        ):
                            _LOGGER.debug(
                                "Removed '%s' from node %s", prompt_str_arg, node_id
                            )
                            any_processed = True
                        else:
                            _LOGGER.debug(
                                "Skipped '%s' in node %s (not found or skipped)",
                                prompt_str_arg,
                                node_id,
                            )

                if not any_processed:
                    _LOGGER.debug("No prompts were removed, skipping submission.")
                    continue

            # 提交到 ComfyUI
            any_success, submit_error = submit_workflow(
                prompt, workflow, comfyui_url, jobs, path
            )
            if submit_error:
                has_errors = True
            if any_success:
                success_count += 1
                if label_to_set and img_id:
                    update_image_label(img_id, label_to_set)

    print(f"processed {success_count}/{len(targets)} image(s) successfully.")

    if success_count == 0:
        _write_action_override("KEEP")

    if has_errors:
        sys.exit(1)
    else:
        sys.exit(0)


def get_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="ComfyUI Funnel Hook Script")
    parser.add_argument(
        "--max-match",
        type=int,
        default=None,
        metavar="N",
        help="最大匹配图片数量，默认使用 HOOK_MAX_MATCH 环境变量值或 4，0 代表不限制",
    )
    subparsers = parser.add_subparsers(
        dest="command", required=True, help="Sub-commands"
    )

    queue_parser = subparsers.add_parser(
        "queue", help="Queue the image back to ComfyUI"
    )
    queue_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )

    add_parser = subparsers.add_parser(
        "add", help="Add prompt to image metadata and queue"
    )
    add_parser.add_argument(
        "--no-skip",
        "-S",
        action="store_true",
        help="Do not skip if prompt already exists",
    )
    add_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    add_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    add_parser.add_argument(
        "--raw", action="store_true", help="Add raw text without trailing comma"
    )
    add_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; priority after node",
    )
    add_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; highest priority",
    )
    add_parser.add_argument("prompt", nargs="+", help="The prompt text to add")

    remove_parser = subparsers.add_parser(
        "remove", help="Remove prompt from image metadata and queue"
    )
    remove_parser.add_argument(
        "--no-skip", "-S", action="store_true", help="Do not skip if prompt not found"
    )
    remove_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    remove_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    remove_parser.add_argument(
        "--raw",
        action="store_true",
        help="Remove raw text substring without line matching",
    )
    remove_parser.add_argument(
        "--hard",
        action="store_true",
        help="Directly delete the text instead of commenting it out",
    )
    remove_parser.add_argument(
        "--all",
        action="store_true",
        help="Remove from all CLIP input nodes, ignoring region/node constraints",
    )
    remove_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; searches all matching areas",
    )
    remove_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; searches all matching nodes",
    )
    remove_parser.add_argument("prompt", nargs="+", help="The prompt text to remove")

    # adjust command
    adjust_parser = subparsers.add_parser(
        "adjust", help="Adjust prompt weights or existing Lora weights"
    )
    adjust_subparsers = adjust_parser.add_subparsers(
        dest="adjust_type", required=True, help="Adjustment types"
    )

    # 1. adjust lora
    lora_parser = adjust_subparsers.add_parser(
        "lora", help="Adjust existing Lora weights"
    )
    lora_parser.add_argument("name", help="Target Lora name (substring match)")
    lora_parser.add_argument(
        "weight",
        help="Weight value or range (e.g. 0.8, 0.5:1.0:0.1, x-0.1:x+0.2:0.1, +-0.3:0.1)",
    )
    lora_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    lora_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    lora_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )

    # 2. adjust prompt
    prompt_parser = adjust_subparsers.add_parser("prompt", help="Adjust prompt weights")
    prompt_parser.add_argument("text", help="Target prompt text to adjust")
    prompt_parser.add_argument(
        "weight",
        help="Weight value or range (e.g. 1.2, 0.8:1.2:0.1, x-0.1:x+0.2:0.1, +-0.3:0.1)",
    )
    prompt_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    prompt_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    prompt_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    prompt_parser.add_argument(
        "--skip-add",
        action="store_true",
        help="Skip adding the prompt if it does not exist",
    )
    prompt_parser.add_argument(
        "--neg",
        action="store_true",
        help="When no region or node matches, use negative keyword matching instead of positive",
    )
    prompt_parser.add_argument(
        "--region",
        action="append",
        default=None,
        metavar="name",
        help="Target region name, can be specified multiple times; priority after node",
    )
    prompt_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target node ID, can be specified multiple times; highest priority",
    )

    # 3. adjust cfg
    cfg_parser = adjust_subparsers.add_parser("cfg", help="Adjust KSampler CFG scale")
    cfg_parser.add_argument(
        "weight",
        help="CFG value or range (e.g. 7.0, 5.0:9.0:0.5, x-0.5:x+0.5:0.5, +-1.0:0.5)",
    )
    cfg_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    cfg_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    cfg_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    cfg_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target KSampler node ID, can be specified multiple times; if omitted, adjusts all KSampler nodes",
    )

    # 5. autocomplete
    autocomplete_parser = subparsers.add_parser(
        "autocomplete", help="Provide autocomplete suggestions for directive parameters"
    )
    autocomplete_parser.add_argument(
        "target_command",
        nargs="?",
        default=None,
        help="The actual runtime sub-command being autocompleted",
    )

    # 4. adjust aspect
    aspect_parser = adjust_subparsers.add_parser(
        "aspect", help="Adjust image aspect ratio (total pixels unchanged)"
    )
    aspect_parser.add_argument(
        "ratio",
        help="Target ratio or range (e.g. 16:9, +1, -1, +-1, +-2:2, swap)",
    )
    aspect_parser.add_argument(
        "-j",
        "--jobs",
        type=int,
        default=None,
        metavar="N",
        help="发送工作流次数，默认使用 HOOK_JOBS 环境变量值",
    )
    aspect_parser.add_argument(
        "--update-seed",
        "-u",
        action="store_true",
        help="Force enable seed updating",
    )
    aspect_parser.add_argument(
        "--no-skip",
        action="store_true",
        help="Do not skip ComfyUI submission even if no changes were made",
    )
    aspect_parser.add_argument(
        "--node",
        action="append",
        default=None,
        metavar="node-id",
        help="Target latent node ID containing width and height inputs, can be specified multiple times; if omitted, adjusts all latent nodes",
    )

    return parser


def parse_args(args: Optional[List[str]] = None):
    return get_parser().parse_args(args)


if __name__ == "__main__":
    main()
