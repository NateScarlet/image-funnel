"""受限沙箱环境的 tempfile 兼容层。

Windows 上 tempfile.mkdtemp 固定以 0o700 权限创建目录；在本项目的受限沙箱中，
该权限模式创建的目录内部不可访问（sqlite 打开、listdir 均被拒绝），
而以默认权限（0o777）创建的目录工作正常——访问控制由用户 TEMP 目录自身的 ACL 保证。

此模块借助 PYTHONPATH 的 sitecustomize 自动加载机制生效，
将 mkdtemp 重定向为以默认权限创建等价的随机临时目录，
使 tempfile.TemporaryDirectory 及其全部使用方无需修改即可正常运行。
"""
import os
import tempfile
import uuid


_orig_mkdtemp = tempfile.mkdtemp


def _sandbox_mkdtemp(suffix=None, prefix="tmp", dir=None):
    # 与原版语义一致：在目标目录下以随机名创建唯一临时目录并返回其路径，
    # 仅省略 0o700 权限参数，改用 os.makedirs 的默认权限；
    # TemporaryDirectory 可能显式传入 None 作为前缀，需回退到默认值
    base_dir = dir if dir is not None else tempfile.gettempdir()
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
