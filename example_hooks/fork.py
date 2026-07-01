#!/usr/bin/env python
# -*- coding: utf-8 -*-

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
import logging
import argparse
from typing import List, Optional

from graphql_utils import move_images, update_image_label

_LOGGER = logging.getLogger(__name__)


def parse_args():
    parser = argparse.ArgumentParser(description="Image Funnel Fork Hook Script")
    parser.add_argument("suffix", help="Suffix for the target directory")
    return parser.parse_args()


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
    suffix = args.suffix.strip()
    if not suffix:
        _LOGGER.error("Suffix cannot be empty.")
        sys.exit(1)

    image_ids_str: str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    label_to_set: Optional[str] = os.getenv("HOOK_IMAGE_SET_LABEL")

    try:
        image_ids: List[str] = json.loads(image_ids_str) if image_ids_str else []
    except Exception as e:
        _LOGGER.error(f"Failed to parse IMAGE_FUNNEL_IMAGE_IDS: {e}")
        sys.exit(1)

    if not image_ids:
        _LOGGER.error("No image IDs to process.")
        sys.exit(1)

    _LOGGER.debug("Received %d image(s) to fork with suffix: %s", len(image_ids), suffix)

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
            update_image_label(img_id, label_to_set)

    _LOGGER.debug(
        "Moving %d image(s) to relative path '%s' using GraphQL...",
        len(image_ids),
        dest_dir,
    )
    move_images(image_ids, dest_dir)

    print(f"processed {len(image_ids)} image(s) successfully.")
    sys.exit(0)


if __name__ == "__main__":
    main()
