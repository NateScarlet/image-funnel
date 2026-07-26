#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import json
import urllib.error
import urllib.request
from contextlib import contextmanager
from typing import Callable, Dict, Generator, List, Tuple, Any, Optional


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

    def execute(self, query: str, variables: Dict[str, Any]) -> Dict[str, Any]:
        """发送 GraphQL 请求的底座方法。"""
        payload: Dict[str, Any] = {"query": query, "variables": variables}

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            self._url,
            data=data,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self._token}",
            },
        )
        try:
            with urllib.request.urlopen(req) as f:
                res = json.loads(f.read().decode("utf-8"))
                if "errors" in res:
                    raise ValueError(f"GraphQL returned errors: {res['errors']}")
                return res["data"]
        except urllib.error.HTTPError as e:
            body = e.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"GraphQL HTTP {e.code}: {body}") from e

    # #region 通知相关方法

    def send_notification(
        self,
        channel: str,
        title: str,
        *,
        tag: str = "",
        body: str = "",
        priority: str = "LOW",
    ) -> Dict[str, Any]:
        """发送通知到指定频道。tag 未指定时由服务端自动生成 UUID。返回 `sendNotification` 的 data。

        支持 <UUID>.<后缀> 格式，可将 UUID 作为命名空间组织相关通知。
        """
        query = """
        mutation SendNotification($input: SendNotificationInput!) {
          sendNotification(input: $input) {
            didCreate
            notification {
              id
              tag
            }
          }
        }
        """
        variables: Dict[str, Any] = {
            "input": {
                "channel": channel,
                "title": title,
                "body": body,
                "priority": priority,
            }
        }
        if tag:
            variables["input"]["tag"] = tag
        return self.execute(query, variables)["sendNotification"]

    def unsend_notification(self, tag: str) -> None:
        """撤回指定 tag 的通知。"""
        query = """
        mutation UnsendNotification($input: UnsendNotificationInput!) {
          unsendNotification(input: $input) {
            notification {
              id
            }
          }
        }
        """
        variables = {"input": {"tag": tag}}
        self.execute(query, variables)

    # #endregion

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


# #region 进度通知上下文管理器

# 进度更新回调类型：接收当前进度、总数和描述消息
ProgressUpdate = Callable[[int, int, str], None]


@contextmanager
def progress_notification(
    client: GraphQLClient,
    base_uuid: str,
    channel: str,
    title: str,
) -> Generator[ProgressUpdate, None, None]:
    """进度通知上下文管理器。

    使用 <base_uuid>.progress 作为通知标签，方便编排多个相关通知。

    用法:
        with progress_notification(client, "550e8400-e29b-41d4-a716-446655440000", "hooks", "处理中") as update:
            update(1, 10, "开始处理...")
            do_work()
            update(2, 10, "继续处理...")
    """
    tag = f"{base_uuid}.progress"

    def update(current: int, step_total: int, message: str) -> None:
        """更新进度通知正文，以相同 tag 覆盖之前的通知。"""
        body = f"{message} ({current}/{step_total})"
        client.send_notification(channel, title, tag=tag, body=body, priority="LOW")

    # 进入时创建初始进度通知
    client.send_notification(channel, title, tag=tag, body="准备中...", priority="LOW")

    try:
        yield update
    finally:
        # 退出时清理进度通知，最终结果由后端 Runner 自动发送
        client.unsend_notification(tag)


# #endregion
