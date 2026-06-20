#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import json
import urllib.request
from typing import Dict, List, Tuple, Any, Optional


def execute(query: str, variables: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """
    发送 GraphQL 请求的底座函数。如果环境变量缺失或请求失败，直接抛出异常。
    """
    graphql_url = os.environ.get("IMAGE_FUNNEL_GRAPHQL_URL")
    token = os.environ.get("IMAGE_FUNNEL_TOKEN")
    if not graphql_url:
        raise ValueError("Environment variable IMAGE_FUNNEL_GRAPHQL_URL is missing.")
    if not token:
        raise ValueError("Environment variable IMAGE_FUNNEL_TOKEN is missing.")

    payload: Dict[str, Any] = {"query": query}
    if variables is not None:
        payload["variables"] = variables

    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        graphql_url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        },
    )
    with urllib.request.urlopen(req) as f:
        res = json.loads(f.read().decode("utf-8"))
        if "errors" in res:
            raise ValueError(f"GraphQL returned errors: {res['errors']}")
        return res["data"]


def update_image_label(image_id: str, label: str) -> None:
    """
    通过 GraphQL 更新图片颜色标签。
    """
    query = """
    mutation UpdateImageMetadata($input: UpdateImageMetadataInput!) {
      updateImageMetadata(input: $input) {
        id
      }
    }
    """
    variables = {"input": {"id": image_id, "label": label}}
    execute(query, variables)


def fetch_images(filter_rating: Optional[int]) -> List[Tuple[str, str]]:
    """
    通过 GraphQL 查询指定目录下的图片，并可选按评分过滤。
    """
    directory_id = os.environ.get("IMAGE_FUNNEL_DIRECTORY_ID")
    if not directory_id:
        raise ValueError("Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing.")

    query = """
    query GetDirectoryImages($dirID: ID!, $filter: ImageFiltersInput) {
      node(id: $dirID) {
        ... on Directory {
          images(filterBy: $filter) {
            nodes {
              id
              relPath
            }
          }
        }
      }
    }
    """
    filter_input: Dict[str, Any] = {}
    if filter_rating is not None:
        filter_input["rating"] = [filter_rating]

    variables = {
        "dirID": directory_id,
        "filter": filter_input if filter_input else None,
    }
    data = execute(query, variables)
    node_data = data["node"]
    if not node_data:
        raise ValueError(f"No directory node found for ID: {directory_id}")

    images_data = node_data["images"]["nodes"]
    root_dir = os.environ.get("IMAGE_FUNNEL_ROOT_DIR")
    if not root_dir:
        raise ValueError("Environment variable IMAGE_FUNNEL_ROOT_DIR is missing.")
    targets: List[Tuple[str, str]] = []
    for img in images_data:
        img_id = img["id"]
        rel_path = img["relPath"]
        abs_path = os.path.normpath(os.path.join(root_dir, rel_path))
        targets.append((img_id, abs_path))
    return targets


def move_images(image_ids: List[str], dest_dir: str) -> None:
    """
    通过 GraphQL 移动指定目录下的匹配图片至目标目录。
    """
    directory_id = os.environ.get("IMAGE_FUNNEL_DIRECTORY_ID")
    if not directory_id:
        raise ValueError("Environment variable IMAGE_FUNNEL_DIRECTORY_ID is missing.")

    query = """
    mutation MoveImages($input: MoveImagesInput!) {
      moveImages(input: $input) {
        movedCount
        targetAbsoluteDirectory
      }
    }
    """
    variables = {
        "input": {
            "directoryId": directory_id,
            "filterBy": {"id": image_ids},
            "toDirectory": {"relativeToRoot": dest_dir},
        }
    }
    execute(query, variables)
