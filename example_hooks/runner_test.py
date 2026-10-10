# 只允许使用项目测试脚本运行测试

import os
import subprocess
import sys
import tempfile
import unittest

# runner.py 是独立部署的单文件入口，测试通过子进程以真实命令行方式调用它，
# 覆盖「模块解析 + 进程退出码 + stderr 内容」这整条对外接缝。
RUNNER_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "runner.py")


class RunnerTestCase(unittest.TestCase):
    """构造一个临时 hooks 根目录，其中放置用于验证模块解析的假 hook 模块。"""

    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name
        self.addCleanup(self._tmp.cleanup)

        # pkg：正常包，入口在 pkg/__main__.py
        self._write("pkg/__init__.py", "")
        self._write("pkg/__main__.py", "print('PACKAGE_MAIN_RAN')\n")

        # plain：无 __main__ 子模块的独立模块，应直接以 __main__ 身份执行
        self._write("plain.py", "print('PLAIN_RAN')\n")

        # boom_http：复刻 open_workflow 的真实故障形态——包内子模块，导入时无副作用、
        # 仅在被当作 __main__ 执行时才失败（open_workflow 的 main() 在 __main__ 保护下）。
        # 旧 runner 先探测 {module}.__main__ 抛 ImportError，再回退执行，
        # 形成 RuntimeWarning + __path__ + "During handling" 的多层噪音。
        self._write(
            "boom_pkg/http_fail.py",
            "import urllib.error\n"
            "\n"
            "def main() -> None:\n"
            "    raise urllib.error.HTTPError('http://x/y', 409, 'Conflict', {}, None)\n"
            "\n"
            "if __name__ == '__main__':\n"
            "    main()\n",
        )

        # boom_import：模块内部因缺少依赖抛 ImportError，真实原因必须原样上抛。
        # 同样只在被当作 __main__ 执行时才失败。
        self._write(
            "boom_import.py",
            "def main() -> None:\n"
            "    raise ImportError('no module named definitely_missing_dep')\n"
            "\n"
            "if __name__ == '__main__':\n"
            "    main()\n",
        )

        # boom_pkg：包入口内部抛业务错误
        self._write("boom_pkg/__init__.py", "")
        self._write(
            "boom_pkg/__main__.py", "raise ValueError('inner failure from package')\n"
        )

        # side_effect：无 __main__ 子模块但其模块体带副作用（走「回退直跑」路径）。
        # 旧 runner 会先为探测 {module}.__main__ 而导入该模块（执行一次模块体），
        # 再回退执行第二次，导致副作用发生两遍。
        self._write(
            "side_effect.py",
            "import os\n"
            "p = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'side_effect.log')\n"
            "with open(p, 'a', encoding='utf-8') as f:\n"
            "    f.write('run\\n')\n",
        )

    def _write(self, rel_path: str, content: str) -> None:
        abs_path = os.path.join(self.root, rel_path)
        os.makedirs(os.path.dirname(abs_path), exist_ok=True)
        with open(abs_path, "w", encoding="utf-8") as f:
            f.write(content)

    def _run(self, *args: str) -> "subprocess.CompletedProcess[str]":
        """在临时根目录下调用 runner，返回完成的进程结果。"""
        env: dict[str, str] = dict(os.environ)
        env["PYTHONPATH"] = self.root
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        return subprocess.run(
            [sys.executable, RUNNER_PATH, *args],
            cwd=self.root,
            env=env,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )

    # #region 正常解析

    def test_runs_package_main(self) -> None:
        result = self._run("pkg")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("PACKAGE_MAIN_RAN", result.stdout)

    def test_runs_module_without_main_submodule(self) -> None:
        """没有 __main__ 子模块的模块应直接以 __main__ 身份执行"""
        result = self._run("plain")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("PLAIN_RAN", result.stdout)
        # 正常的回退直跑不应产生 runpy 的 sys.modules 警告噪音
        self.assertNotIn("RuntimeWarning", result.stderr)

    def test_runs_submodule_entry_point(self) -> None:
        """以 comfyui.open_workflow 方式启动的子模块入口"""
        self._write("pkg/entry.py", "print('SUBMODULE_ENTRY_RAN')\n")
        result = self._run("pkg.entry")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("SUBMODULE_ENTRY_RAN", result.stdout)

    # #endregion

    # #region 模块体只执行一次

    def test_module_body_executes_exactly_once(self) -> None:
        """回退直跑路径下模块体只能执行一次（不得因探测 __main__ 而多跑一遍）"""
        result = self._run("side_effect")
        self.assertEqual(0, result.returncode, result.stderr)

        log_path = os.path.join(self.root, "side_effect.log")
        with open(log_path, encoding="utf-8") as f:
            runs = f.read().split()
        self.assertEqual(1, len(runs), "模块体被执行了多次")

    # #endregion

    # #region 错误传播

    def test_http_error_is_not_masked(self) -> None:
        """包内子模块抛 HTTPError 时，原始错误必须可见且不被探测噪音污染"""
        result = self._run("boom_pkg.http_fail")
        self.assertEqual(1, result.returncode)
        self.assertIn("HTTPError", result.stderr)
        self.assertIn("409", result.stderr)
        # 不应出现 __main__ 探测失败造成的异常链噪音
        self.assertNotIn("__path__", result.stderr)
        self.assertNotIn("During handling of the above exception", result.stderr)
        self.assertNotIn("Cannot run module", result.stderr)

    def test_internal_import_error_is_not_masked(self) -> None:
        """目标模块自身抛出的 ImportError 不得被误判为「模块不存在」"""
        result = self._run("boom_import")
        self.assertEqual(1, result.returncode)
        self.assertIn("definitely_missing_dep", result.stderr)
        self.assertNotIn("Cannot run module", result.stderr)

    def test_package_inner_failure_keeps_cause(self) -> None:
        result = self._run("boom_pkg")
        self.assertEqual(1, result.returncode)
        self.assertIn("inner failure from package", result.stderr)

    def test_missing_module_reports_clear_error(self) -> None:
        result = self._run("definitely_missing_module")
        self.assertEqual(1, result.returncode)
        self.assertIn("definitely_missing_module", result.stderr)

    def test_no_module_argument_prints_usage(self) -> None:
        result = self._run()
        self.assertEqual(1, result.returncode)
        self.assertIn("Usage", result.stderr)

    # #endregion


if __name__ == "__main__":
    unittest.main()
