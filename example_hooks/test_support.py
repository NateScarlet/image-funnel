"""测试进程的共享环境准备。

部分测试用 ``patch.dict(os.environ, ..., clear=True)`` 模拟 hook 的极简运行环境。
环境变量被清空后，``tempfile.gettempdir()`` 的候选列表会兜底到当前工作目录，
且结果被缓存到进程级 ``tempfile.tempdir``——后续测试的临时目录会全部散落到
仓库根目录。这里在环境尚完整时（模块导入阶段）先定格基准目录，使同一进程内
的临时目录始终落在 TEMP/TMP 指向的位置。

各测试模块在导入阶段显式调用 ``pin_tempdir()``，保证定格先于任何
``patch.dict(..., clear=True)`` 生效。
"""

import os
import tempfile


def pin_tempdir() -> None:
    """定格 tempfile 的基准目录；已在别处（如 sitecustomize）定格时保持不变。"""
    if tempfile.tempdir is not None:
        return

    base_dir = tempfile.gettempdir()
    # 基准目录不可用或等于 cwd 说明 TEMP/TMP 缺失：此时不能继续，否则临时
    # 目录会写进工作区。快速失败以提示调用者修正环境，而不是静默污染仓库。
    if os.path.abspath(base_dir) == os.path.abspath(os.getcwd()) or not os.path.isdir(
        base_dir
    ):
        raise RuntimeError(
            f"测试进程没有可用的临时目录（{base_dir}）："
            "请将 TEMP/TMP 指向已存在的目录后再运行"
        )
    tempfile.tempdir = base_dir
