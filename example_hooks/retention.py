#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import logging
from datetime import datetime
from typing import Any, Dict, List, Optional

from graphql_utils import GraphQLClient

_LOGGER = logging.getLogger(__name__)


def _write_action_override(action: str) -> None:
    """向 IMAGE_FUNNEL_ACTION 指向的临时文件写入操作覆盖，供服务端 Runner 读取"""
    action_path = os.getenv("IMAGE_FUNNEL_ACTION")
    if not action_path:
        raise ValueError("Environment variable IMAGE_FUNNEL_ACTION is missing.")
    with open(action_path, "w", encoding="utf-8") as f:
        f.write(action)


def run_retention(client: GraphQLClient) -> None:
    # 快速失败：校验必需的环境变量
    rating_str = os.getenv("HOOK_IMAGE_RATING")
    if not rating_str:
        raise ValueError("Environment variable HOOK_IMAGE_RATING is missing.")
    max_retain_str = os.getenv("HOOK_MAX_RETAIN")
    if not max_retain_str:
        raise ValueError("Environment variable HOOK_MAX_RETAIN is missing.")

    try:
        rating = int(rating_str)
    except ValueError:
        _LOGGER.error("HOOK_IMAGE_RATING must be an integer, got: %s", rating_str)
        sys.exit(1)

    try:
        max_retain = int(max_retain_str)
    except ValueError:
        _LOGGER.error("HOOK_MAX_RETAIN must be an integer, got: %s", max_retain_str)
        sys.exit(1)

    if max_retain < 0:
        _LOGGER.error("HOOK_MAX_RETAIN must be non-negative, got: %d", max_retain)
        sys.exit(1)

    directory_id = os.getenv("IMAGE_FUNNEL_DIRECTORY_ID")
    if not directory_id:
        raise ValueError("Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing.")

    # 分页查询指定评分的图片及其修改时间
    page_size = 1000
    images_raw: List[Dict[str, Any]] = []
    cursor: Optional[str] = None

    while True:
        query = """
        query GetDirectoryImages($dirID: ID!, $filter: ImageFiltersInput, $first: Int, $after: String) {
          node(id: $dirID) {
            ... on Directory {
              images(filterBy: $filter, first: $first, after: $after) {
                nodes {
                  id
                  modTime
                  note {
                    content
                  }
                }
                pageInfo {
                  hasNextPage
                  endCursor
                }
              }
            }
          }
        }
        """
        variables: Dict[str, Any] = {
            "dirID": directory_id,
            "filter": {"rating": [rating]},
            "first": page_size,
        }
        if cursor is not None:
            variables["after"] = cursor

        _LOGGER.debug(
            "Querying images with rating %d in directory %s (after=%s)...",
            rating,
            directory_id,
            cursor,
        )
        data: Dict[str, Any] = client.execute(query, variables)
        node_data: Dict[str, Any] = data["node"]
        page_data: Dict[str, Any] = node_data["images"]
        images_raw.extend(page_data["nodes"])
        page_info: Dict[str, Any] = page_data["pageInfo"]

        if not page_info["hasNextPage"]:
            break
        cursor = page_info["endCursor"]

    count = len(images_raw)

    if count <= max_retain:
        _LOGGER.debug(
            "Image count (%d) does not exceed max retain (%d), skipping.",
            count,
            max_retain,
        )
        # 本次未移除任何图片：写入 KEEP 并保持静默（无 stdout），Runner 据此跳过成功通知
        _write_action_override("KEEP")
        sys.exit(0)

    # 按 modTime 升序排列（最旧的在前），超出部分移到回收站
    images_raw.sort(key=lambda img: datetime.fromisoformat(img["modTime"]))
    excess = images_raw[: count - max_retain]

    # 不移除存在关联笔记（非空 note.content）的图片
    excess = [img for img in excess if not img["note"]["content"]]
    if not excess:
        _LOGGER.debug("All excess images have non-empty notes, skipping trash.")
        # 本次未移除任何图片：写入 KEEP 并保持静默（无 stdout），Runner 据此跳过成功通知
        _write_action_override("KEEP")
        sys.exit(0)

    excess_ids: List[str] = [img["id"] for img in excess]

    _LOGGER.debug(
        "Trashing %d excess image(s) (retaining %d/%d).",
        len(excess_ids),
        max_retain,
        count,
    )

    trash_query = """
    mutation TrashImages($input: TrashImagesInput!) {
      trashImages(input: $input) {
        movedCount
        historyId
      }
    }
    """
    trash_variables: Dict[str, Any] = {
        "input": {
            "directoryId": directory_id,
            "filterBy": {"id": excess_ids, "rating": [rating]},
            "message": f"retention 自动移除 {rating} 星图片（保留 {max_retain}/{count}）",
        }
    }
    result: Dict[str, Any] = client.execute(trash_query, trash_variables)
    moved: int = result["trashImages"]["movedCount"]

    print(f"已清理 {moved} 张较旧 {rating} 星图片，保留最新 {max_retain} 张")
    sys.exit(0)


if __name__ == "__main__":
    client = GraphQLClient.from_env()
    run_retention(client)
