#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# dependencies = [
#   "Pillow",
#   "requests",
# ]
# ///

import sys
import importlib


def main() -> None:
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
