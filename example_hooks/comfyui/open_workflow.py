#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
「在工作流页面打开」钩子：读取选中图片内置的 ComfyUI 工作流，执行与复制增强一致的
输出目录调整与提示词模型格式重排，然后连同按图片路径派生的名称 POST 到 comfyui-nodes
扩展的 ``/io.github.natescarlet/open`` 路由，由最近连接的网页端直接打开，免去复制粘贴。

经统一 runner 以「模块名回退直跑」方式启动（uv run runner.py comfyui.open_workflow），
与 copy_workflow 单次执行同模式。核心逻辑不读取环境变量，依赖（请求上下文 + 元数据
加载器 + 发送函数）由最外层入口构建并注入。
"""

import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Tuple, cast

from .copy_workflow import prepare_workflow
from .png_metadata import load_prompt_and_workflow

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

# 发送函数协议：把调整后的工作流连同派生名称 POST 到目标 ComfyUI。
# 名称需从图片路径派生，故接收整个请求上下文而非仅 URL。抛异常表示送达失败（快速失败）
SendWorkflow = Callable[[OpenRequest, Dict[str, Any]], None]


def build_workflow_name(image_path: str) -> str:
    """按 `<目录名>__<图片名>` 拼装网页端显示的工作流名称。

    comfyui-nodes 只透传该字段、不解析格式，因此命名规则由本钩子定义。
    """
    directory = os.path.basename(os.path.dirname(image_path))
    image_name = os.path.splitext(os.path.basename(image_path))[0]
    return f"{directory}__{image_name}"


def send_workflow(request: OpenRequest, workflow: Dict[str, Any]) -> None:
    """把工作流与名称 POST 到 comfyui-nodes 的打开路由。

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
            response.read()
    except urllib.error.HTTPError as error:
        if error.code == 404:
            raise ValueError(ROUTE_MISSING_ERROR) from error
        raise
    except urllib.error.URLError as error:
        raise ValueError(UNREACHABLE_ERROR) from error


def open_workflow(
    request: OpenRequest, load_metadata: MetadataLoader, send: SendWorkflow
) -> str:
    """把选中图片的工作流送达网页端打开，返回一句给用户看的结果描述。

    无 ComfyUI 元数据的图片不属于本钩子范围，快速失败；配置或网络错误直接抛出
    异常，由入口以非零退出码上报（快速失败），不静默降级。
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
    )
    send(request, adjusted)
    return "已在 ComfyUI 页面打开该图片的工作流"


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
        comfyui_url=os.environ.get("COMFYUI_URL", "http://127.0.0.1:8188"),
        comfyui_output_dir=os.environ.get("COMFYUI_OUTPUT_DIR", ""),
        hook_output_dir=os.environ.get("HOOK_OUTPUT_DIR", ""),
    )


def main() -> None:
    request = build_request_from_env()
    result = open_workflow(request, load_prompt_and_workflow, send_workflow)
    print(result)


if __name__ == "__main__":
    main()
