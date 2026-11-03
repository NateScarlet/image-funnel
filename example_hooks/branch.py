#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
import argparse
from dataclasses import dataclass
from typing import List, Optional, Set

from branch_directory import branch_directory_exists, branch_directory_rel_path
from comfyui.cancellable_http import HttpError
from comfyui.cancellation import CancelToken
from comfyui.danbooru import DanbooruTagProvider, SemanticLayerUnavailable
from comfyui.danbooru_env import build_tag_provider_from_env
from comfyui.model_format import format_prompt_text
from graphql_utils import GraphQLClient

_LOGGER = logging.getLogger(__name__)


# #region 自动完成数据结构与逻辑


@dataclass
class AutocompleteSuggestion:
    text: str
    displayText: str
    description: str
    type: str = "directory"
    style: str = ""

    def to_jsonl(self) -> str:
        return json.dumps(
            {
                "text": self.text,
                "displayText": self.displayText,
                "description": self.description,
                "type": self.type,
                "style": self.style,
            },
            ensure_ascii=False,
        )


def _directory_suggestions(
    root_dir: str, dir_rel_path: str, q: str
) -> List[AutocompleteSuggestion]:
    """按 /branch 现有命名规则给出「已存在的目录」候选（行为保持不变）。

    q 是已归一化（小写、剥引号）的前缀；根目录下目标为 `<suffix>`，
    子目录下目标为同级 `<当前目录名>,<suffix>`。
    """
    suggestions: List[AutocompleteSuggestion] = []

    norm_rel = os.path.normpath(dir_rel_path.strip()) if dir_rel_path.strip() else ""
    if norm_rel == ".":
        norm_rel = ""

    if not norm_rel:
        # 根目录下触发：目标目录即为 suffix，建议根目录下的已有直接子目录
        try:
            entries = sorted(os.listdir(root_dir))
        except OSError as e:
            _LOGGER.error("Failed to list root directory %s: %s", root_dir, e)
            return []

        for entry in entries:
            abs_item = os.path.join(root_dir, entry)
            if os.path.isdir(abs_item):
                if q and not entry.lower().startswith(q):
                    continue
                suggestions.append(
                    AutocompleteSuggestion(
                        text=entry,
                        displayText=entry,
                        description=f"已存在目录：{entry}",
                        type="directory",
                    )
                )
    else:
        # 子目录下触发：同级目录规则为 当前目录名,<suffix>
        dir_name = os.path.basename(norm_rel)
        parent_rel = os.path.dirname(norm_rel)
        parent_abs = os.path.normpath(os.path.join(root_dir, parent_rel))

        if not os.path.exists(parent_abs):
            return []

        try:
            entries = sorted(os.listdir(parent_abs))
        except OSError as e:
            _LOGGER.error("Failed to list parent directory %s: %s", parent_abs, e)
            return []

        prefix = f"{dir_name},"
        prefix_len = len(prefix)

        for entry in entries:
            abs_item = os.path.join(parent_abs, entry)
            if os.path.isdir(abs_item) and entry.startswith(prefix):
                suffix = entry[prefix_len:]
                if not suffix:
                    continue
                if q and not suffix.lower().startswith(q):
                    continue
                suggestions.append(
                    AutocompleteSuggestion(
                        text=suffix,
                        displayText=suffix,
                        description=f"已存在同级目录：{entry}",
                        type="directory",
                    )
                )

    return suggestions


def _tag_suggestions(
    root_dir: str,
    dir_rel_path: str,
    query: str,
    provider: DanbooruTagProvider,
    cancel_token: CancelToken,
    taken_texts: Set[str],
) -> List[AutocompleteSuggestion]:
    """给出 Danbooru 标签候选。

    复用与 `/remix --add` 相同的搜索管线（字面匹配 + 语义层 + NSFW 开关）。该管线
    对空查询的定义是「无结果」；`/branch` 不依赖当前图片工作流，也没有提示词/操作
    历史这类种子来源，因此空 suffix 时只有目录候选，这里不会产出标签条目。

    插入文本复用提交路径的 sdxl 格式化（补全文本必须与落盘形态一致）；展示
    保留标签名与中文名，描述保留释义。taken_texts 是与已有目录候选重复的文本，
    直接跳过（目录条目已带「已存在」语义）。
    """
    try:
        tags = provider.search(query, cancel_token=cancel_token)
    except HttpError as e:
        _LOGGER.warning(
            "Failed to provide branch tag suggestions: %s", e, exc_info=True
        )
        return [
            AutocompleteSuggestion(
                text="",
                displayText="⚠ Danbooru 搜索失败",
                description=f"{e}",
                type="error",
            )
        ]

    suggestions: List[AutocompleteSuggestion] = []
    unavailable: List[SemanticLayerUnavailable] = []
    for item in tags:
        # 语义层降级提示项与标签分流：字面结果照常返回，提示项在末尾单独给出
        if isinstance(item, SemanticLayerUnavailable):
            unavailable.append(item)
            continue
        text = format_prompt_text(item.tag, "sdxl")
        if text in taken_texts:
            continue
        display = f"{item.tag} ({item.cn_name})" if item.cn_name else item.tag
        description = item.wiki if item.wiki else "Danbooru 标签"
        style = ""
        # 按 /branch 命名规则算出的目标目录已存在时加标记；含非法文件名字符的
        # 候选原样给出，能否落成目录交给后续移动时报错暴露
        if branch_directory_exists(root_dir, dir_rel_path, text):
            style = "muted"
            description = f"(已存在) {description}"
        suggestions.append(
            AutocompleteSuggestion(
                text=text,
                displayText=display,
                description=description,
                type="danbooru",
                style=style,
            )
        )

    for item in unavailable:
        suggestions.append(
            AutocompleteSuggestion(
                text="",
                displayText="⚠ Danbooru 语义搜索不可用",
                description=item.reason,
                type="error",
            )
        )

    return suggestions


def get_branch_autocomplete_suggestions(
    root_dir: str,
    dir_rel_path: str,
    query: str,
    tag_provider: Optional[DanbooruTagProvider],
    cancel_token: CancelToken,
) -> List[AutocompleteSuggestion]:
    """根据根目录路径、当前目录相对路径和用户查询生成 /branch 的后缀补全建议。

    候选顺序：先「已存在目录」（行为与历史一致），随后是 Danbooru 标签候选。
    标签补全不依赖当前图片的 workflow / 目标节点；未配置任何数据来源时
    tag_provider 为 None，退化为纯目录补全。

    空查询在搜索管线里的定义是「无结果」，而 /branch 没有工作流/历史种子可回退，
    所以空 suffix（suffix 未写）时只给出目录候选；输入部分文本后才出现标签候选。
    """
    if not root_dir or not os.path.exists(root_dir):
        return []

    q = query.strip().strip("\"'").lower()
    suggestions = _directory_suggestions(root_dir, dir_rel_path, q)

    if tag_provider is not None:
        suggestions.extend(
            _tag_suggestions(
                root_dir,
                dir_rel_path,
                query,
                tag_provider,
                cancel_token,
                {item.text for item in suggestions},
            )
        )

    return suggestions


def _at_first_positional_arg(cwords: List[str]) -> bool:
    """判断光标是否处于第一个位置参数位（suffix）。

    `/branch` 只有 suffix 一个可选位置参数：已完成词里除指令名与 `<>` 占位符之外
    还有词，说明 suffix 已经填过，之后的位置没有可补全的内容。
    """
    tokens = [w for w in cwords if not (w.startswith("<") and w.endswith(">"))]
    return len(tokens) <= 1


def run_autocomplete() -> None:
    """自动完成执行入口，从环境变量读取上下文参数并输出 JSONL 建议项目。

    Danbooru 依赖（搜索地址、NSFW 开关、数据目录、嵌入端点与结果缓存）由
    comfyui.danbooru_env 统一从环境解析；未配置数据来源时只给目录候选。
    """
    root_dir = os.getenv("IMAGE_FUNNEL_ROOT_DIR", "")
    dir_rel_path = os.getenv("IMAGE_FUNNEL_DIRECTORY_REL_PATH", "")
    query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")
    cwords: List[str] = json.loads(os.environ["IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS"])

    if not _at_first_positional_arg(cwords):
        return

    # 单次执行模式没有取消通道：显式构造一个新建的令牌
    with build_tag_provider_from_env(root_dir, dir_rel_path) as tag_provider:
        suggestions = get_branch_autocomplete_suggestions(
            root_dir, dir_rel_path, query, tag_provider, CancelToken()
        )
    for item in suggestions:
        print(item.to_jsonl())


# #endregion


def parse_args():
    parser = argparse.ArgumentParser(description="Image Funnel Branch Hook Script")
    parser.add_argument(
        "suffix", nargs="?", default="TODO", help="Suffix for the target directory"
    )
    return parser.parse_args()


def run_branch(client: GraphQLClient) -> None:
    args = parse_args()
    suffix = args.suffix.strip() if args.suffix else "TODO"
    if not suffix:
        suffix = "TODO"

    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except json.JSONDecodeError as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS: {e}")
        sys.exit(1)

    if not image_ids:
        _LOGGER.error("No image IDs to process.")
        sys.exit(1)

    _LOGGER.debug(
        "Received %d image(s) to branch with suffix: %s", len(image_ids), suffix
    )

    # 确定目标相对路径：命名规则与 ComfyUI `--branch` 共用同一份实现
    dest_dir = branch_directory_rel_path(
        os.getenv("IMAGE_FUNNEL_DIRECTORY_REL_PATH", ""), suffix
    )

    if label_to_set:
        for img_id in image_ids:
            client.update_image_label(img_id, label_to_set)

    directory_id = os.getenv("IMAGE_FUNNEL_DIRECTORY_ID")
    if not directory_id:
        raise ValueError("Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing.")

    _LOGGER.debug(
        "Moving %d image(s) to relative path '%s' using GraphQL...",
        len(image_ids),
        dest_dir,
    )
    client.move_images(directory_id, image_ids, dest_dir)

    print(f"移动了 {len(image_ids)} 张图片至「{dest_dir}」")
    sys.exit(0)


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "autocomplete":
        run_autocomplete()
        sys.exit(0)
    else:
        client = GraphQLClient.from_env()
        run_branch(client)
