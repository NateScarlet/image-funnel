#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "Pillow",
#   "requests",
# ]
# ///

import logging
import os
import sys
import importlib


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
        print("Usage: uv run __main__.py <module> [args...]", file=sys.stderr)
        sys.exit(1)

    module_name = sys.argv[1]
    sys.argv = [module_name] + sys.argv[2:]

    # 优先尝试 {module}.__main__（适用于 comfyui 这类包入口），
    # 失败则直接导入模块自身（适用于 fork、comfyui.autocomplete 等）
    try:
        module = importlib.import_module(f"{module_name}.__main__")
    except ModuleNotFoundError:
        try:
            module = importlib.import_module(module_name)
        except ModuleNotFoundError:
            print(
                f"Error: Cannot import module '{module_name}' or '{module_name}.__main__'",
                file=sys.stderr,
            )
            sys.exit(1)

    module.main()


if __name__ == "__main__":
    main()
