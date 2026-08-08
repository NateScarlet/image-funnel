#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
import argparse
from dataclasses import dataclass
from typing import List, Optional

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


def get_fork_autocomplete_suggestions(
    root_dir: str, dir_rel_path: str, query: str = ""
) -> List[AutocompleteSuggestion]:
    """根据根目录路径、当前目录相对路径和用户查询生成 /fork 指令的后缀补全建议。"""
    if not root_dir or not os.path.exists(root_dir):
        return []

    suggestions: List[AutocompleteSuggestion] = []
    q = query.strip().strip("\"'").lower()

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


def run_autocomplete() -> None:
    """自动完成执行入口，从环境变量读取上下文参数并输出 JSONL 建议项目。"""
    root_dir = os.getenv("IMAGE_FUNNEL_ROOT_DIR", "")
    dir_rel_path = os.getenv("IMAGE_FUNNEL_DIRECTORY_REL_PATH", "")
    query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")

    suggestions = get_fork_autocomplete_suggestions(root_dir, dir_rel_path, query)
    for item in suggestions:
        print(item.to_jsonl())


# #endregion


def parse_args():
    parser = argparse.ArgumentParser(description="Image Funnel Fork Hook Script")
    parser.add_argument(
        "suffix", nargs="?", default="TODO", help="Suffix for the target directory"
    )
    return parser.parse_args()


def run_fork(client: GraphQLClient) -> None:
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
        "Received %d image(s) to fork with suffix: %s", len(image_ids), suffix
    )

    # 确定目标相对路径
    dir_rel_path = os.getenv("IMAGE_FUNNEL_DIRECTORY_REL_PATH", "")
    if not dir_rel_path.strip():
        # 根目录下触发，在当前目录下新建子目录
        dest_dir = suffix
    else:
        # 子目录下触发，创建同级目录
        dir_name = os.path.basename(dir_rel_path)
        parent_dir_rel_path = os.path.dirname(dir_rel_path)
        dest_dir = os.path.join(parent_dir_rel_path, f"{dir_name},{suffix}")
    dest_dir = os.path.normpath(dest_dir)

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
        run_fork(client)
