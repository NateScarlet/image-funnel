#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
「在工作流页面打开」钩子：读取选中图片内置的 ComfyUI 工作流，执行与复制增强一致的
输出目录调整与提示词模型格式重排，然后连同按图片路径派生的名称 POST 到 comfyui-nodes
扩展的 ``/io.github.natescarlet/open`` 路由，由最近连接的网页端直接打开，免去复制粘贴。
送达成功后激活目标 ComfyUI 窗口，并用响应里返回的 sid 确认切到的是接收页面。

经统一 runner 以「模块名回退直跑」方式启动（uv run runner.py comfyui.open_workflow），
与 copy_workflow 单次执行同模式。核心逻辑不读取环境变量，依赖（请求上下文 + 元数据
加载器 + 发送函数 + 激活确认入口）由最外层入口构建并注入。
"""

import datetime
import json
import os
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Tuple, cast

from .copy_workflow import prepare_workflow
from .focus import (
    DEFAULT_COMFYUI_URL,
    FocusOutcome,
    activate_and_confirm,
    describe_outcome,
    query_focus,
)
from .png_metadata import load_prompt_and_workflow
from .window import default_window_platform

# comfyui-nodes 暴露的「在工作流页面打开」路由（与后端 OPEN_ROUTE_PATH 一致）
OPEN_ROUTE_PATH = "/io.github.natescarlet/open"

# 目标 404：路由不存在，说明 comfyui-nodes 扩展未安装/未启用
ROUTE_MISSING_ERROR = "目标 ComfyUI 未安装或未启用 comfyui-nodes 扩展（缺少该打开路由）"

# 连接失败：网络层根本没能连上，和「连上了但没有路由」是两回事，文案区分
UNREACHABLE_ERROR = f"连不上 ComfyUI：无法访问 {OPEN_ROUTE_PATH}"


@dataclass(frozen=True)
class OpenRequest:
    """一次「在工作流页面打开」请求的上下文（由入口构造并注入）"""

    image_path: str
    comfyui_url: str
    comfyui_output_dir: str  # COMFYUI_OUTPUT_DIR，ComfyUI 输出根目录
    hook_output_dir: str  # HOOK_OUTPUT_DIR，目标目录覆盖配置


# 元数据加载器协议：图片路径 -> (prompt, workflow)，某一侧缺失时对应元素为 None
MetadataLoader = Callable[
    [str], Tuple[Optional[Dict[str, Any]], Optional[Dict[str, Any]]]
]

# 发送函数协议：把调整后的工作流连同派生名称 POST 到目标 ComfyUI，返回本次送达的
# 目标页面 sid（旧版扩展不返回时为空）。名称需从图片路径派生，故接收整个请求上下文
# 而非仅 URL。抛异常表示送达失败（快速失败）
SendWorkflow = Callable[[OpenRequest, Dict[str, Any]], Optional[str]]

# 激活确认入口协议：open_workflow 与 focus 共用同一份实现，凭 sid 激活并确认接收页面
Activation = Callable[[Optional[str]], FocusOutcome]


def build_workflow_name(image_path: str) -> str:
    """按 `<目录名>__<图片名>` 拼装网页端显示的工作流名称。

    comfyui-nodes 只透传该字段、不解析格式，因此命名规则由本钩子定义。
    """
    directory = os.path.basename(os.path.dirname(image_path))
    image_name = os.path.splitext(os.path.basename(image_path))[0]
    return f"{directory}__{image_name}"


def _parse_sid(body: bytes) -> Optional[str]:
    """从打开路由的响应里取出目标 sid。

    旧版扩展成功时返回空响应体，此时没有 sid 可用，只能激活而无法确认。
    响应体不符合契约时同样按「拿不到 sid」处理：工作流已经送达，激活仍然有价值，
    不能因为响应形状异常把已送达的结果改判为失败。
    """
    if not body.strip():
        return None
    try:
        payload: Any = json.loads(body)
    except ValueError:
        return None
    if not isinstance(payload, dict):
        return None
    sid = cast(Dict[str, Any], payload).get("sid")
    if not isinstance(sid, str) or not sid:
        return None
    return sid


def send_workflow(request: OpenRequest, workflow: Dict[str, Any]) -> Optional[str]:
    """把工作流与名称 POST 到 comfyui-nodes 的打开路由，返回目标页面 sid。

    失败按两类区分并抛出可读异常：目标返回 404 说明路由不存在（扩展未装/未启用），
    其余网络错误说明连不上 ComfyUI。两者文案不同，因为用户要采取的行动不同。
    """
    url = f"{request.comfyui_url.rstrip('/')}{OPEN_ROUTE_PATH}"
    data: bytes = json.dumps(
        {
            "workflow": workflow,
            "name": build_workflow_name(request.image_path),
        },
        ensure_ascii=False,
    ).encode("utf-8")
    req: urllib.request.Request = urllib.request.Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req) as response:
            body = response.read()
    except urllib.error.HTTPError as error:
        if error.code == 404:
            raise ValueError(ROUTE_MISSING_ERROR) from error
        raise
    except urllib.error.URLError as error:
        raise ValueError(UNREACHABLE_ERROR) from error
    return _parse_sid(body)


def open_workflow(
    request: OpenRequest,
    load_metadata: MetadataLoader,
    send: SendWorkflow,
    activate: Activation,
    now: datetime.datetime,
) -> str:
    """把选中图片的工作流送达网页端打开，返回一句给用户看的结果描述。

    无 ComfyUI 元数据的图片不属于本钩子范围，快速失败；配置或网络错误直接抛出
    异常，由入口以非零退出码上报（快速失败），不静默降级。送达后把响应里的 sid
    交给与 focus 相同的激活确认入口；确认结果不影响「工作流已送达」这一事实。
    """
    prompt, workflow = load_metadata(request.image_path)
    if not prompt or not workflow or "nodes" not in workflow:
        raise ValueError(
            f"图片没有内嵌 ComfyUI 工作流，无法直接打开：{request.image_path}"
        )

    adjusted = prepare_workflow(
        request.image_path,
        prompt,
        workflow,
        request.comfyui_output_dir,
        request.hook_output_dir,
        now,
    )
    sid = send(request, adjusted)
    outcome = activate(sid)
    return f"已在 ComfyUI 页面打开该图片的工作流（{describe_outcome(outcome)}）"


def _parse_single_image_path(raw: str, env_name: str) -> str:
    """解析 IMAGE_FUNNEL_IMAGE_PATHS，要求恰好是一张图片路径。"""
    items: List[Any] = json.loads(raw)
    for item in items:
        if not isinstance(item, str):
            raise ValueError(f"{env_name} must be a JSON array of strings.")
    paths = cast(List[str], items)
    if len(paths) != 1:
        raise ValueError(f"open workflow expects exactly one image, got {len(paths)}")
    return paths[0]


def build_request_from_env() -> OpenRequest:
    """单次模式：入口从环境变量构造请求上下文（缺失即报错，快速失败）。"""
    raw_paths = os.environ.get("IMAGE_FUNNEL_IMAGE_PATHS")
    if not raw_paths:
        raise ValueError("Environment variable IMAGE_FUNNEL_IMAGE_PATHS is missing.")
    return OpenRequest(
        image_path=_parse_single_image_path(raw_paths, "IMAGE_FUNNEL_IMAGE_PATHS"),
        comfyui_url=os.environ.get("COMFYUI_URL", DEFAULT_COMFYUI_URL),
        comfyui_output_dir=os.environ.get("COMFYUI_OUTPUT_DIR", ""),
        hook_output_dir=os.environ.get("HOOK_OUTPUT_DIR", ""),
    )


def build_activation(comfyui_url: str) -> Activation:
    """在最外层构建激活确认入口：平台与查询依赖都在这里一次性装配"""
    platform = default_window_platform()

    def activate(sid: Optional[str]) -> FocusOutcome:
        return activate_and_confirm(comfyui_url, sid, platform, query_focus, time.sleep)

    return activate


def main() -> None:
    request = build_request_from_env()
    result = open_workflow(
        request,
        load_prompt_and_workflow,
        send_workflow,
        build_activation(request.comfyui_url),
        datetime.datetime.now(),
    )
    print(result)


if __name__ == "__main__":
    main()
