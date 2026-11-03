#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
分支目录命名的共用规则。

`/branch` 指令与 ComfyUI `/remix --branch` 必须把「当前目录 + 后缀」换算成同一份
目标目录：根目录下为 `<suffix>`，子目录下为同级 `<当前目录名>,<suffix>`。
规则只此一处，避免两侧各写一份后逐渐漂移。
"""

import os


def branch_directory_rel_path(directory_rel_path: str, suffix: str) -> str:
    """按 /branch 的命名规则推算目标目录的相对路径。

    directory_rel_path 是当前目录相对根目录的路径（根目录传空串）；返回值同样
    相对根目录，使用平台默认分隔符。
    """
    normalized = (
        os.path.normpath(directory_rel_path.strip())
        if directory_rel_path.strip()
        else ""
    )
    if normalized == ".":
        normalized = ""

    if not normalized:
        # 根目录下触发：目标目录即为后缀本身
        return suffix

    # 子目录下触发：与当前目录同级，名称为「当前目录名,后缀」
    dir_name = os.path.basename(normalized)
    parent_rel = os.path.dirname(normalized)
    return os.path.normpath(os.path.join(parent_rel, f"{dir_name},{suffix}"))


def branch_directory_exists(
    root_dir: str, directory_rel_path: str, suffix: str
) -> bool:
    """判断按命名规则推算出的分支目录是否已存在（补全「已存在」标记用）。"""
    if not root_dir:
        return False
    target = branch_directory_rel_path(directory_rel_path, suffix)
    return os.path.isdir(os.path.join(root_dir, target))
