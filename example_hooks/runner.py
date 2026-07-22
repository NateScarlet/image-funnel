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
import runpy


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

    # 优先尝试以 __main__ 身份运行 {module}.__main__，
    # 失败则直接以 __main__ 身份运行模块自身
    try:
        runpy.run_module(f"{module_name}.__main__", run_name="__main__", alter_sys=True)
    except ImportError:
        try:
            runpy.run_module(module_name, run_name="__main__", alter_sys=True)
        except ImportError:
            print(
                f"Error: Cannot run module '{module_name}' or '{module_name}.__main__'",
                file=sys.stderr,
            )
            sys.exit(1)
    except Exception as e:
        # 目标模块执行出错时，清除 runpy 内部清理 sys.modules 失败产生的次要错误，
        # 避免异常链污染原始错误消息
        if e.__context__ is not None:
            e.__context__ = None
        raise


if __name__ == "__main__":
    main()
