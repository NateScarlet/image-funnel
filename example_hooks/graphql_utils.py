#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import json
import urllib.request
from typing import Dict, List, Tuple, Any, Optional


class GraphQLClient:
    """GraphQL 客户端，封装 URL 与 Token，提供核心请求发送与业务数据获取能力。"""

    def __init__(self, url: str, token: str) -> None:
        self._url = url
        self._token = token

    @classmethod
    def from_env(cls) -> "GraphQLClient":
        """从环境变量快速创建客户端实例。"""
        url = os.environ.get("IMAGE_FUNNEL_GRAPHQL_URL")
        if not url:
            raise ValueError(
                "Environment variable IMAGE_FUNNEL_GRAPHQL_URL is missing."
            )
        token = os.environ.get("IMAGE_FUNNEL_TOKEN")
        if not token:
            raise ValueError("Environment variable IMAGE_FUNNEL_TOKEN is missing.")
        return cls(url, token)

    def execute(
        self, query: str, variables: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """发送 GraphQL 请求的底座方法。"""
        payload: Dict[str, Any] = {"query": query}
        if variables is not None:
            payload["variables"] = variables

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            self._url,
            data=data,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self._token}",
            },
        )
        with urllib.request.urlopen(req) as f:
            res = json.loads(f.read().decode("utf-8"))
            if "errors" in res:
                raise ValueError(f"GraphQL returned errors: {res['errors']}")
            return res["data"]

    def update_image_label(self, image_id: str, label: str) -> None:
        """通过 GraphQL 更新图片颜色标签。"""
        query = """
        mutation UpdateImageMetadata($input: UpdateImageMetadataInput!) {
          updateImageMetadata(input: $input) {
            id
          }
        }
        """
        variables = {"input": {"id": image_id, "label": label}}
        self.execute(query, variables)

    def fetch_images(
        self, directory_id: str, root_dir: str, filter_rating: Optional[int]
    ) -> List[Tuple[str, str]]:
        """通过 GraphQL 查询指定目录下的图片，并可选按评分过滤。"""
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
        data = self.execute(query, variables)
        node_data = data["node"]
        if not node_data:
            raise ValueError(f"No directory node found for ID: {directory_id}")

        images_data = node_data["images"]["nodes"]
        targets: List[Tuple[str, str]] = []
        for img in images_data:
            img_id = img["id"]
            rel_path = img["relPath"]
            abs_path = os.path.normpath(os.path.join(root_dir, rel_path))
            targets.append((img_id, abs_path))
        return targets

    def move_images(
        self, directory_id: str, image_ids: List[str], dest_dir: str
    ) -> None:
        """通过 GraphQL 移动指定目录下的匹配图片至目标目录。"""
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
        self.execute(query, variables)
