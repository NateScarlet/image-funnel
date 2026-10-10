#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "Pillow",
#   "numpy",
#   "requests",
#   "pyarrow",
# ]
# ///

import importlib.util
import logging
import os
import sys
import runpy


def resolve_target(module_name: str) -> str:
    """决定实际要执行的模块全名。

    约定：目标若是包则执行其 ``__main__``（包的标准入口形式）；
    否则目标自身即入口，直接以 ``__main__`` 身份执行。

    这里只探测目标本身——探测 ``{module}.__main__`` 会先导入父模块并执行其模块体，
    产生「模块被跑两遍」的副作用和成串异常链噪音，因此必须避开。
    目标不存在时返回空串，由调用方报告。
    """
    try:
        spec = importlib.util.find_spec(module_name)
    except (ImportError, AttributeError, ValueError):
        # 目标的父包不存在或不可导入，等同于目标不存在
        return ""
    if spec is None:
        return ""
    if spec.submodule_search_locations is None:
        return module_name

    entry = f"{module_name}.__main__"
    try:
        entry_spec = importlib.util.find_spec(entry)
    except (ImportError, AttributeError, ValueError):
        return ""
    return entry if entry_spec is not None else ""


def main() -> None:
    # 在子模块导入前配置全局日志，子模块通过 logging.getLogger(__name__) 即可输出
    log_level_str = os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper()
    log_level = getattr(logging, log_level_str, logging.WARNING)
    logging.basicConfig(
        level=log_level,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    if len(sys.argv) < 2:
        print("Usage: uv run runner.py <module> [args...]", file=sys.stderr)
        sys.exit(1)

    module_name = sys.argv[1]
    sys.argv = [module_name] + sys.argv[2:]

    target = resolve_target(module_name)
    if not target:
        print(
            f"Error: Cannot run module '{module_name}': no such module or package entry",
            file=sys.stderr,
        )
        sys.exit(1)

    # 目标模块自身抛出的异常原样传播，交由 Python 打印原始 traceback（含真实原因），
    # 不在此处捕获改写，避免掩盖调用者需要看到的错误
    runpy.run_module(target, run_name="__main__", alter_sys=True)


if __name__ == "__main__":
    main()
