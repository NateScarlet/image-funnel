"""受限沙箱环境的 tempfile 兼容层。

Windows 上 tempfile.mkdtemp 固定以 0o700 权限创建目录；在本项目的受限沙箱中，
该权限模式创建的目录内部不可访问（sqlite 打开、listdir 均被拒绝），
而以默认权限（0o777）创建的目录工作正常——访问控制由用户 TEMP 目录自身的 ACL 保证。

此模块借助 PYTHONPATH 的 sitecustomize 自动加载机制生效，
将 mkdtemp 重定向为以默认权限创建等价的随机临时目录，
使 tempfile.TemporaryDirectory 及其全部使用方无需修改即可正常运行。

临时目录基准在导入时定格：测试代码常用 patch.dict(os.environ, ..., clear=True)
清空环境变量，若此时才调用 gettempdir()，其候选列表会失去 TEMP/TMP 并兜底到 cwd，
导致临时目录散落在项目根目录（甚至被沙箱拒写）。导入时记录基准可让这一场景仍落在正确位置。
"""

import os
import tempfile
import uuid


def _capture_base_tempdir() -> str | None:
    """在环境变量尚完整时解析临时目录基准，供 mkdtemp 使用。

    返回 None 表示当前环境无法提供可用的临时目录，由调用方报错。
    这里要求基准目录已存在且不是 cwd——cwd 只是 gettempdir() 的最后兜底，
    并不代表一个真正的临时目录。
    """
    try:
        base_dir = tempfile.gettempdir()
    except FileNotFoundError:
        return None
    if os.path.abspath(base_dir) == os.path.abspath(os.getcwd()):
        return None
    if not os.path.isdir(base_dir):
        return None
    return base_dir


# 导入即定格：此时 TEMP/TMP 由外层脚本设置，尚未被测试代码清空
_BASE_TEMPDIR = _capture_base_tempdir()


def _sandbox_mkdtemp(suffix=None, prefix="tmp", dir=None):
    # 与原版语义一致：在目标目录下以随机名创建唯一临时目录并返回其路径，
    # 仅省略 0o700 权限参数，改用 os.makedirs 的默认权限；
    # TemporaryDirectory 可能显式传入 None 作为前缀，需回退到默认值
    base_dir = dir if dir is not None else (_BASE_TEMPDIR or tempfile.gettempdir())
    # gettempdir() 的候选列表以 cwd 兜底：当环境变量指向的临时目录不可用时，
    # 它会静默返回 cwd，致使临时目录散落在项目根目录。
    # 临时目录不可用必须显式失败，由调用方修正环境变量，而不是悄悄污染工作区
    if os.path.abspath(base_dir) == os.path.abspath(os.getcwd()):
        raise RuntimeError(
            f"拒绝在 cwd 创建临时目录（{base_dir}）："
            "请将 TEMP/TMP 指向可用的临时目录后再运行"
        )
    name = os.path.join(base_dir, (prefix or "tmp") + uuid.uuid4().hex + (suffix or ""))
    os.makedirs(name)
    return name


tempfile.mkdtemp = _sandbox_mkdtemp
