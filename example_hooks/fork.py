#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
import argparse
from typing import List, Optional

from graphql_utils import GraphQLClient

_LOGGER = logging.getLogger(__name__)


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
    client = GraphQLClient.from_env()
    run_fork(client)
