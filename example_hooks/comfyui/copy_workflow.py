#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
复制增强钩子：提取图片内置的 ComfyUI 工作流，执行与入列一致的输出目录调整，
以单行 JSON 信封输出到 stdout，供应用写入剪贴板。

经统一 runner 以「模块名回退直跑」方式启动（uv run runner.py comfyui.copy_workflow），
与 autocomplete 单次执行同模式。核心逻辑不读取环境变量，
依赖（请求上下文 + 元数据加载器）由最外层入口构建并注入。
"""

import json
import os
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Tuple, cast

from .filename_manager import FilenameManager
from .output_directory import get_relative_output_dir
from .png_metadata import load_prompt_and_workflow
from .workflow_prompt_pair import WorkflowPromptPair

# 元数据加载器协议：图片路径 -> (prompt, workflow)，某一侧缺失时对应元素为 None
MetadataLoader = Callable[
    [str], Tuple[Optional[Dict[str, Any]], Optional[Dict[str, Any]]]
]


@dataclass(frozen=True)
class CopyRequest:
    """一次复制增强请求的上下文（由入口构造并注入）"""

    image_paths: List[str]
    comfyui_output_dir: str  # COMFYUI_OUTPUT_DIR，ComfyUI 输出根目录
    hook_output_dir: str  # HOOK_OUTPUT_DIR，目标目录覆盖配置


@dataclass(frozen=True)
class CopyResult:
    """复制增强结果：content 为写入剪贴板的文本"""

    content: str
    description: str


def build_copy_content(
    request: CopyRequest, load_metadata: MetadataLoader
) -> Optional[CopyResult]:
    """构建复制增强内容。

    返回 None 表示对当前图片不适用（无 ComfyUI 元数据），调用方应保持 stdout 为空；
    配置或环境错误直接抛出异常（快速失败），不静默降级。
    """
    if len(request.image_paths) != 1:
        raise ValueError(
            "copy enhancement expects exactly one image, got "
            f"{len(request.image_paths)}"
        )
    image_path = request.image_paths[0]

    prompt, workflow = load_metadata(image_path)
    # 无对应元数据的图片不属于 ComfyUI 格式：输出空内容即表示放弃
    if not prompt or not workflow or "nodes" not in workflow:
        return None

    pair = WorkflowPromptPair(workflow, prompt)
    if request.hook_output_dir == ":inherit:":
        # 与入列侧语义一致：完全关闭目录自动调整，复制原始未调整的工作流
        description = "已复制原始 ComfyUI 工作流"
    else:
        rel_dir = get_relative_output_dir(
            image_path, request.comfyui_output_dir, request.hook_output_dir
        )
        FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        ).adjust_output_directory(rel_dir)
        description = "已复制 ComfyUI 工作流（输出目录已调整）"

    return CopyResult(
        content=json.dumps(pair.workflow, ensure_ascii=False),
        description=description,
    )


def _parse_string_list(raw: str, env_name: str) -> List[str]:
    """解析 JSON 字符串数组（可信边界校验：要求恰好是字符串列表，否则报错）"""
    image_paths: List[Any] = json.loads(raw)  # JSON 非法时抛出 ValueError
    for item in image_paths:
        if not isinstance(item, str):
            raise ValueError(f"{env_name} must be a JSON array of strings.")
    return cast(List[str], image_paths)


def build_request_from_env() -> CopyRequest:
    """单次模式：入口从环境变量构造请求上下文（缺失即报错，快速失败）。"""
    raw_paths = os.environ.get("IMAGE_FUNNEL_IMAGE_PATHS")
    if not raw_paths:
        raise ValueError("Environment variable IMAGE_FUNNEL_IMAGE_PATHS is missing.")
    return CopyRequest(
        image_paths=_parse_string_list(raw_paths, "IMAGE_FUNNEL_IMAGE_PATHS"),
        comfyui_output_dir=os.environ.get("COMFYUI_OUTPUT_DIR", ""),
        hook_output_dir=os.environ.get("HOOK_OUTPUT_DIR", ""),
    )


def main() -> None:
    request = build_request_from_env()
    result = build_copy_content(request, load_prompt_and_workflow)
    if result is None:
        # 不适用：stdout 保持为空，服务端视为「无增强内容」，前端降级复制文件本体
        return
    print(
        json.dumps(
            {"content": result.content, "description": result.description},
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
