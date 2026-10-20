# 只允许使用项目测试脚本运行测试

import os
import tempfile
import unittest
from unittest.mock import patch

from test_support import pin_tempdir

pin_tempdir()


class TestTempdirPinning(unittest.TestCase):
    def test_tempdir_does_not_fall_back_to_cwd_after_env_cleared(self) -> None:
        """清空 TEMP/TMP 后基准目录仍是测试临时目录，而不是仓库根目录。"""
        with patch.dict(os.environ, {}, clear=True):
            base_dir = tempfile.gettempdir()
        self.assertNotEqual(os.path.abspath(base_dir), os.path.abspath(os.getcwd()))

    def test_make_temp_dir_inside_cleared_env_stays_out_of_cwd(self) -> None:
        """环境被清空时创建的临时目录同样不应落在仓库根目录。"""
        with patch.dict(os.environ, {}, clear=True):
            tmp_dir = tempfile.mkdtemp()
        try:
            parent = os.path.dirname(tmp_dir)
            self.assertNotEqual(os.path.abspath(parent), os.path.abspath(os.getcwd()))
        finally:
            os.rmdir(tmp_dir)
