import json
import logging
import os
import subprocess
import unittest
from typing import Any, List, Tuple
from unittest.mock import MagicMock, patch

logging.disable(logging.CRITICAL)

from krita_open import (
    KritaRequest,
    activate_krita_window,
    build_request_from_env,
    find_krita_exe,
    find_krita_window,
    focus_window,
    open_in_krita,
    simulate_alt_keypress,
    window_process_name,
    spawn_krita,
)


class TestKritaOpenHook(unittest.TestCase):

    def setUp(self):
        self.env_patcher = patch.dict(
            os.environ,
            {
                "IMAGE_FUNNEL_IMAGE_PATHS": json.dumps(
                    [r"C:\img\a.png", r"C:\img\b.png"]
                ),
                "KRITA_EXE": r"C:\Krita\krita.exe",
            },
            clear=True,
        )
        self.env_patcher.start()

    def tearDown(self):
        self.env_patcher.stop()

    def test_build_request_from_env_reads_paths_and_exe(self):
        request = build_request_from_env()
        self.assertEqual(request.image_paths, [r"C:\img\a.png", r"C:\img\b.png"])
        self.assertEqual(request.krita_exe, r"C:\Krita\krita.exe")

    def test_build_request_requires_image_paths(self):
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError):
                build_request_from_env()

    def test_build_request_rejects_non_string_items(self):
        with patch.dict(os.environ, {"IMAGE_FUNNEL_IMAGE_PATHS": "[1, 2]"}):
            with self.assertRaises(ValueError):
                build_request_from_env()

    def test_find_krita_exe_prefers_explicit_env(self):
        self.assertEqual(find_krita_exe(), r"C:\Krita\krita.exe")

    def test_find_krita_exe_falls_back_to_install_pattern(self):
        install_path = r"C:\Program Files\Krita (x64)\bin\krita.exe"
        with patch.dict(os.environ, {"KRITA_EXE": ""}):
            with patch("krita_open.shutil.which", return_value=None):
                with patch("krita_open.glob.glob", return_value=[install_path]):
                    self.assertEqual(find_krita_exe(), install_path)

    def test_find_krita_exe_raises_when_missing(self):
        with patch.dict(os.environ, {"KRITA_EXE": ""}):
            with patch("krita_open.shutil.which", return_value=None):
                with patch("krita_open.glob.glob", return_value=[]):
                    with self.assertRaises(RuntimeError):
                        find_krita_exe()

    def test_open_in_krita_passes_all_paths_in_single_spawn(self):
        calls: List[Tuple[str, List[str]]] = []

        def fake_spawn(exe: str, paths: List[str]) -> None:
            calls.append((exe, paths))

        count = open_in_krita(
            KritaRequest([r"C:\img\a.png", r"C:\img\b.png"], r"C:\Krita\krita.exe"),
            fake_spawn,
        )

        self.assertEqual(count, 2)
        self.assertEqual(
            calls, [(r"C:\Krita\krita.exe", [r"C:\img\a.png", r"C:\img\b.png"])]
        )

    def test_open_in_krita_rejects_empty_paths(self):
        with self.assertRaises(ValueError):
            open_in_krita(KritaRequest([], r"C:\Krita\krita.exe"), MagicMock())

    def test_spawn_krita_redirects_stdio(self):
        with patch("krita_open.subprocess.Popen") as popen:
            spawn_krita(r"C:\Krita\krita.exe", [r"C:\img\a.png"])

        args, kwargs = popen.call_args
        self.assertEqual(args[0], [r"C:\Krita\krita.exe", r"C:\img\a.png"])
        self.assertEqual(kwargs["stdin"], subprocess.DEVNULL)
        self.assertEqual(kwargs["stdout"], subprocess.DEVNULL)
        self.assertEqual(kwargs["stderr"], subprocess.DEVNULL)
        self.assertTrue(kwargs["close_fds"])

    def test_spawn_krita_keeps_foreground_capability(self):
        """DETACHED_PROCESS 会让 Krita 失去设置前台窗口的能力，必须不使用。"""
        with patch("krita_open.subprocess.Popen") as popen:
            spawn_krita(r"C:\Krita\krita.exe", [r"C:\img\a.png"])

        _, kwargs = popen.call_args
        creationflags = kwargs["creationflags"]
        self.assertTrue(creationflags & subprocess.CREATE_NEW_PROCESS_GROUP)
        self.assertFalse(creationflags & subprocess.DETACHED_PROCESS)

    def test_find_krita_window_skips_other_processes(self):
        """标题含 krita 但属于其他进程的窗口（如 Everything 搜索结果）必须跳过。"""
        user32 = MagicMock()
        user32.GetTopWindow.return_value = 100
        user32.GetWindow.side_effect = [200, 0]
        user32.IsWindowVisible.return_value = True

        def fake_title(hwnd: Any, buf: Any, length: int) -> int:
            buf.value = "krita export_fo - Everything"
            return int(length)

        user32.GetWindowTextW.side_effect = fake_title

        process_names = {100: "everything.exe", 200: "krita.exe"}

        def fake_process_name(hwnd: int) -> str:
            return process_names.get(hwnd, "")

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            with patch("krita_open.window_process_name", side_effect=fake_process_name):
                self.assertEqual(find_krita_window(), 200)

    def test_find_krita_window_prefers_document_window(self):
        """多个候选时优先标题形如 "<文件名> - Krita" 的主窗口，跳过启动画面。"""
        user32 = MagicMock()
        user32.GetTopWindow.return_value = 100
        user32.GetWindow.side_effect = [200, 0]
        user32.IsWindowVisible.return_value = True
        titles = {100: "Krita", 200: "x.png (1 MiB) - Krita"}

        def fake_title(hwnd: Any, buf: Any, length: int) -> int:
            buf.value = titles[int(hwnd.value)]
            return int(length)

        user32.GetWindowTextW.side_effect = fake_title

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            with patch("krita_open.window_process_name", return_value="krita.exe"):
                self.assertEqual(find_krita_window(), 200)

    def test_find_krita_window_returns_zero_when_absent(self):
        user32 = MagicMock()
        user32.GetTopWindow.return_value = 0

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            self.assertEqual(find_krita_window(), 0)

    def test_window_process_name_returns_empty_without_process_id(self):
        """拿不到进程 ID（如窗口刚销毁）时返回空串，调用方按不匹配处理。"""
        with patch("krita_open.window_process_id", return_value=0):
            self.assertEqual(window_process_name(100), "")

    def test_window_process_name_returns_empty_on_open_failure(self):
        """OpenProcess 失败（如进程已退出）时返回空串。"""
        kernel32 = MagicMock()
        kernel32.OpenProcess.return_value = 0

        with patch("krita_open.window_process_id", return_value=1234):
            with patch("krita_open.ctypes.windll") as windll:
                windll.kernel32 = kernel32
                self.assertEqual(window_process_name(100), "")

    def test_window_process_name_returns_basename(self):
        """返回可执行文件名而非完整路径，并统一为小写以便比较。"""
        kernel32 = MagicMock()
        kernel32.OpenProcess.return_value = 4242

        def fake_query(process: Any, flags: int, buf: Any, size: Any) -> int:
            buf.value = r"C:\Program Files\Krita (x64)\bin\krita.exe"
            return 1

        kernel32.QueryFullProcessImageNameW.side_effect = fake_query

        with patch("krita_open.window_process_id", return_value=1234):
            with patch("krita_open.ctypes.windll") as windll:
                windll.kernel32 = kernel32
                self.assertEqual(window_process_name(100), "krita.exe")
        kernel32.CloseHandle.assert_called_once_with(4242)

    def test_simulate_alt_keypress_sends_down_and_up(self):
        """合成 ALT 的按下与抬起，解除前台锁定。"""
        user32 = MagicMock()

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            simulate_alt_keypress()

        self.assertEqual(user32.keybd_event.call_count, 2)
        user32.keybd_event.assert_any_call(0x12, 0, 0, 0)
        user32.keybd_event.assert_any_call(0x12, 0, 0x0002, 0)

    def test_focus_window_retries_after_alt_keypress(self):
        """直接调用被拒后，合成 ALT 解除锁定再试一次。"""
        user32 = MagicMock()
        user32.SetForegroundWindow.side_effect = [0, 1]
        kernel32 = MagicMock()

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            with patch("krita_open.simulate_alt_keypress") as alt:
                self.assertTrue(focus_window(200))

        alt.assert_called_once()
        user32.AttachThreadInput.assert_not_called()

        """已最大化的窗口不是最小化状态，调用 SW_RESTORE 会把它还原，必须跳过。"""
        user32 = MagicMock()
        user32.GetForegroundWindow.return_value = 0
        user32.IsIconic.return_value = False
        kernel32 = MagicMock()
        kernel32.GetCurrentThreadId.return_value = 111

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            focus_window(200)

        user32.ShowWindow.assert_not_called()
        user32.SetForegroundWindow.assert_called_once()

    def test_focus_window_restores_minimized_window(self):
        """最小化的窗口需要先还原，否则无法显示到前台。"""
        user32 = MagicMock()
        user32.GetForegroundWindow.return_value = 0
        user32.IsIconic.return_value = True
        kernel32 = MagicMock()
        kernel32.GetCurrentThreadId.return_value = 111

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            focus_window(200)

        user32.ShowWindow.assert_called_once()

    def test_focus_window_attaches_to_foreground_thread(self):
        """直接调用与 ALT 重试都失败时，附加到前台线程后重试。"""
        user32 = MagicMock()
        user32.SetForegroundWindow.side_effect = [0, 0, 1]
        user32.GetForegroundWindow.return_value = 999
        user32.GetWindowThreadProcessId.return_value = 555
        user32.AttachThreadInput.return_value = 1
        kernel32 = MagicMock()
        kernel32.GetCurrentThreadId.return_value = 111

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            focus_window(200)

        user32.AttachThreadInput.assert_any_call(111, 555, True)
        user32.AttachThreadInput.assert_any_call(111, 555, False)

    def test_focus_window_brings_to_top_as_fallback(self):
        """附加线程后仍未激活时，用 BringWindowToTop 兜底。"""
        user32 = MagicMock()
        user32.SetForegroundWindow.return_value = 0
        user32.GetForegroundWindow.return_value = 999
        user32.GetWindowThreadProcessId.return_value = 555
        user32.AttachThreadInput.return_value = 1
        kernel32 = MagicMock()
        kernel32.GetCurrentThreadId.return_value = 111

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            focus_window(200)

        user32.BringWindowToTop.assert_called_once()

    def test_focus_window_returns_true_without_attach_when_direct_succeeds(self):
        """直接调用成功时立即返回，不做多余的线程附加。"""
        user32 = MagicMock()
        user32.SetForegroundWindow.return_value = 1
        kernel32 = MagicMock()

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            self.assertTrue(focus_window(200))

        user32.AttachThreadInput.assert_not_called()
        user32.BringWindowToTop.assert_not_called()

    def test_focus_window_returns_false_when_activation_fails(self):
        """所有手段都失败时返回 False，调用方据此判断是否真的激活。"""
        user32 = MagicMock()
        user32.SetForegroundWindow.return_value = 0
        user32.GetForegroundWindow.return_value = 0
        kernel32 = MagicMock()

        with patch("krita_open.ctypes.windll") as windll:
            windll.user32 = user32
            windll.kernel32 = kernel32
            self.assertFalse(focus_window(200))

    def test_activate_krita_window_focuses_existing_window(self):
        """复用已有实例时窗口已存在，应立即聚焦而无需等待。"""
        with patch("krita_open.find_krita_window", return_value=200):
            with patch("krita_open.focus_window") as focus:
                activate_krita_window()

        focus.assert_called_once_with(200)

    def test_activate_krita_window_waits_for_window_to_appear(self):
        """冷启动时窗口尚未创建，应轮询等待。"""
        with patch("krita_open.find_krita_window", side_effect=[0, 0, 200]):
            with patch("krita_open.focus_window") as focus:
                with patch("krita_open.time.sleep") as sleep:
                    activate_krita_window()

        self.assertEqual(sleep.call_count, 2)
        focus.assert_called_once_with(200)

    def test_activate_krita_window_raises_on_timeout(self):
        with patch("krita_open.find_krita_window", return_value=0):
            with patch("krita_open.time.sleep"):
                with patch("krita_open.time.monotonic", side_effect=[0.0, 1.0, 100.0]):
                    with self.assertRaises(RuntimeError):
                        activate_krita_window(timeout_seconds=10.0)


if __name__ == "__main__":
    unittest.main()
