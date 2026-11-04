#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
输出目录推算的共用模块。

根据图片所在路径与 COMFYUI_OUTPUT_DIR / HOOK_OUTPUT_DIR 配置，
推算目标输出目录相对 ComfyUI 输出根目录的 rel_dir。
供入列主流程（__main__.py）与复制增强脚本（copy_workflow.py）共同导入。
"""

import os
from typing import Optional

from branch_directory import branch_directory_rel_path


def is_under_directory(child_path: str, parent_path: str) -> bool:
    """
    判断 child_path 是否在 parent_path 内部（或为同一路径）。
    支持 Windows 下不区分大小写的包含关系判断。
    """
    child_norm = os.path.normcase(os.path.abspath(child_path))
    parent_norm = os.path.normcase(os.path.abspath(parent_path))
    try:
        common = os.path.normcase(os.path.commonpath([parent_norm, child_norm]))
        return common == parent_norm
    except ValueError:
        return False


def _resolve_comfyui_output_dir(image_path: str, comfyui_output_dir_env: str) -> str:
    """确定 ComfyUI 输出根目录：显式配置优先，否则从图片路径向左找最后一个 output 目录。"""
    image_path_abs = os.path.abspath(image_path)

    if comfyui_output_dir_env:
        comfyui_output_dir = os.path.abspath(comfyui_output_dir_env)
        # 验证图片是否在 comfyui_output_dir 下
        if not is_under_directory(image_path_abs, comfyui_output_dir):
            raise ValueError(
                f"Image '{image_path}' is not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
            )
        return comfyui_output_dir

    # 从右向左寻找最后一个名为 "output" 的目录
    curr = os.path.dirname(image_path_abs)
    while True:
        parent, name = os.path.split(curr)
        if not name:  # 到了根目录
            break
        if name.lower() == "output":
            return curr
        curr = parent

    raise ValueError(
        f"Could not find any directory named 'output' in the path of image '{image_path}'"
    )


def get_relative_output_dir(
    image_path: str, comfyui_output_dir_env: str, hook_output_dir: str
) -> str:
    """
    计算并返回目标输出目录相对于 ComfyUI 输出根目录的相对路径（使用正斜杠）。
    如果路径校验失败，或者找不到根目录，则抛出 ValueError。
    """
    image_path_abs = os.path.abspath(image_path)
    comfyui_output_dir = _resolve_comfyui_output_dir(image_path, comfyui_output_dir_env)

    # 决定目标目录 target_dir
    if hook_output_dir:
        if os.path.isabs(hook_output_dir):
            # 如果是绝对目录，必须在 comfyui_output_dir 内，否则报错
            target_dir = os.path.abspath(hook_output_dir)
            if not is_under_directory(target_dir, comfyui_output_dir):
                raise ValueError(
                    f"HOOK_OUTPUT_DIR '{hook_output_dir}' is an absolute path but not under COMFYUI_OUTPUT_DIR '{comfyui_output_dir}'"
                )
        else:
            # 默认相对于输出目录
            target_dir = os.path.abspath(
                os.path.join(comfyui_output_dir, hook_output_dir)
            )
    else:
        target_dir = os.path.dirname(image_path_abs)

    # 计算相对路径
    rel_dir = os.path.relpath(target_dir, comfyui_output_dir)
    rel_dir = rel_dir.replace("\\", "/")
    return rel_dir


def get_source_rel_dir(image_path: str, comfyui_output_dir_env: str) -> str:
    """返回源图当前所在目录相对 ComfyUI 输出根目录的路径（使用正斜杠）。

    用于在调整输出目录前剥离上一代目标目录；图片位于输出根目录时返回 "."。
    路径校验失败或找不到输出根目录时抛出 ValueError。
    """
    comfyui_output_dir = _resolve_comfyui_output_dir(image_path, comfyui_output_dir_env)
    rel_dir = os.path.relpath(
        os.path.dirname(os.path.abspath(image_path)), comfyui_output_dir
    )
    return rel_dir.replace("\\", "/")


def get_branch_output_rel_dir(
    image_path: str, comfyui_output_dir_env: str, suffix: str
) -> str:
    """按 /branch 命名规则推算生成结果的新分支目录（相对 ComfyUI 输出根目录）。

    图片所在的目录层级同样相对 ComfyUI 输出根目录计算，因此根目录下的图得到
    `<suffix>`，子目录下的图得到同级 `<当前目录名>,<suffix>`。
    """
    image_dir_rel_path = get_source_rel_dir(image_path, comfyui_output_dir_env)
    if image_dir_rel_path == ".":
        image_dir_rel_path = ""

    rel_dir = branch_directory_rel_path(image_dir_rel_path, suffix)
    _validate_branch_rel_dir(rel_dir, suffix)
    return rel_dir.replace("\\", "/")


def _validate_branch_rel_dir(rel_dir: str, suffix: str) -> None:
    """校验分支目录相对路径不表达绝对路径、盘符或向上层级。

    `os.path.join(base, "C:foo")` 会直接丢弃 base，因此不能只对拼接结果做包含性
    判断——必须在拼接前按分隔符逐段验证，否则盘符相对形式会绕过输出根目录边界。
    """
    normalized = rel_dir.replace("\\", "/")
    if os.path.isabs(normalized) or os.path.splitdrive(normalized)[0]:
        raise ValueError(
            f"--branch suffix '{suffix}' must be a relative directory name"
        )
    for segment in normalized.split("/"):
        if segment in ("", ".", "..") or os.path.splitdrive(segment)[0]:
            raise ValueError(
                f"--branch suffix '{suffix}' must be a relative directory name"
            )


def resolve_output_rel_dir(
    image_path: str,
    comfyui_output_dir_env: str,
    hook_output_dir: str,
    branch_suffix: Optional[str],
) -> Optional[str]:
    """决定本次提交的目标目录相对路径；返回 None 表示不做目录调整。

    `--branch` 一旦出现就显式优先于 HOOK_OUTPUT_DIR，连 `:inherit:` 也被覆盖：
    它表达的是单条指令更明确的落盘意图。没有 `--branch` 时保持原有 HOOK_OUTPUT_DIR
    语义（`:inherit:` 关闭自动调整）。
    """
    if branch_suffix is not None:
        return get_branch_output_rel_dir(
            image_path, comfyui_output_dir_env, branch_suffix
        )
    if hook_output_dir == ":inherit:":
        return None
    return get_relative_output_dir(image_path, comfyui_output_dir_env, hook_output_dir)
