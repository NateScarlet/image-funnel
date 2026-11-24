import json
import logging
import os
import subprocess
import time
import unittest
from typing import Any, List, Optional, Tuple
from unittest.mock import MagicMock, patch

logging.disable(logging.CRITICAL)

from krita_open import (
    KritaRequest,
    activate_krita_window,
    build_request_from_env,
    find_krita_exe,
    find_krita_window,
    focus_window,
    main,
    NamedPipeTransport,
    PipeNotAvailable,
    PruneTabsRpcClient,
    RpcError,
    open_in_krita,
    run_hook,
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


class FakePipeTransport:
    """管道接缝的测试替身：记录请求字节，按预设返回响应或抛出异常。"""

    def __init__(
        self, response: bytes = b"", raises: Optional[BaseException] = None
    ) -> None:
        self.response = response
        self.raises = raises
        self.requests: List[Tuple[str, bytes, float]] = []

    def request(self, path: str, payload: bytes, timeout_seconds: float) -> bytes:
        self.requests.append((path, payload, timeout_seconds))
        if self.raises is not None:
            raise self.raises
        return self.response


class TestPruneTabsRpcClient(unittest.TestCase):
    """RPC 客户端接缝：协议字节、响应分类与无监听端行为。"""

    def test_prune_tabs_sends_exact_protocol_request(self):
        transport = FakePipeTransport(
            b'{"jsonrpc":"2.0","result":{"closed":2},"id":1}\n'
        )

        PruneTabsRpcClient(transport).prune_tabs()

        self.assertEqual(len(transport.requests), 1)
        path, payload, timeout = transport.requests[0]
        self.assertEqual(path, r"\\.\pipe\io.github.natescarlet.krita-plugin")
        self.assertEqual(
            payload, b'{"jsonrpc":"2.0","method":"pruneTabs","params":{},"id":1}\n'
        )
        self.assertGreater(timeout, 0)

    def test_prune_tabs_silently_ignores_missing_listener(self):
        """连接或写入失败代表没有监听端，必须静默忽略且不影响退出码。"""
        transport = FakePipeTransport(raises=PipeNotAvailable("no listener"))

        PruneTabsRpcClient(transport).prune_tabs()

        self.assertEqual(len(transport.requests), 1)

    def test_prune_tabs_raises_on_rpc_error_response(self):
        """监听端报错必须抛给调用者，不能只写日志后当成功。"""
        transport = FakePipeTransport(
            b'{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":1}\n'
        )

        with self.assertRaises(RpcError):
            PruneTabsRpcClient(transport).prune_tabs()

    def test_prune_tabs_raises_on_malformed_response(self):
        """无法解析的响应不能当作成功。"""
        with self.assertRaises(RpcError):
            PruneTabsRpcClient(FakePipeTransport(b"not json\n")).prune_tabs()

    def test_pipe_transport_times_out_when_listener_never_responds(self):
        """监听端不回响应时读取必须超时，不能永久阻塞。"""
        pipe = MagicMock()

        def never_responds() -> bytes:
            time.sleep(0.5)
            return b'{"jsonrpc":"2.0","result":{},"id":1}\n'

        pipe.readline.side_effect = never_responds
        with patch("krita_open.open", create=True, return_value=pipe):
            with self.assertRaises(TimeoutError):
                NamedPipeTransport().request(
                    r"\\.\pipe\test", b"req\n", timeout_seconds=0.01
                )
        pipe.close.assert_called_once()


class TestPruneTabsOrchestration(unittest.TestCase):
    """编排接缝：RPC 调用发生在打开并激活 Krita 之后。"""

    def test_run_hook_prunes_after_opening_and_activating(self):
        events: List[str] = []
        request = KritaRequest([r"C:\img\a.png"], r"C:\Krita\krita.exe")

        def spawn(exe: str, paths: List[str]) -> None:
            events.append("open")

        def activate() -> None:
            events.append("activate")

        rpc = MagicMock()
        rpc.prune_tabs.side_effect = lambda: events.append("prune")

        count = run_hook(request, spawn, activate, rpc)

        self.assertEqual(count, 1)
        self.assertEqual(events, ["open", "activate", "prune"])

    def test_run_hook_propagates_rpc_error_to_caller(self):
        """RPC 错误必须让调用者可见（未捕获异常 => 非零退出码）。"""
        rpc = MagicMock()
        rpc.prune_tabs.side_effect = RpcError("rpc failed")

        with self.assertRaises(RpcError):
            run_hook(
                KritaRequest([r"C:\img\a.png"], r"C:\Krita\krita.exe"),
                MagicMock(),
                MagicMock(),
                rpc,
            )

    def test_main_wires_real_rpc_client_into_hook(self):
        """入口必须把真实 RPC 客户端注入编排，否则清理动作不会实际发生。"""
        request = KritaRequest([r"C:\img\a.png"], r"C:\Krita\krita.exe")
        with patch("krita_open.build_request_from_env", return_value=request):
            with patch("krita_open.run_hook") as hook:
                main()

        rpc_client = hook.call_args.args[3]
        self.assertIsInstance(rpc_client, PruneTabsRpcClient)

    def test_main_exits_normally_when_plugin_has_no_listener(self):
        """无监听端时入口不报错、不输出错误提示，图片照常打开。"""
        request = KritaRequest([r"C:\img\a.png"], r"C:\Krita\krita.exe")
        transport = FakePipeTransport(raises=PipeNotAvailable("no listener"))

        with patch("krita_open.build_request_from_env", return_value=request):
            with patch("krita_open.spawn_krita") as spawn:
                with patch("krita_open.activate_krita_window") as activate:
                    with patch("krita_open.NamedPipeTransport", return_value=transport):
                        with patch("builtins.print") as printer:
                            main()

        spawn.assert_called_once_with(request.krita_exe, request.image_paths)
        activate.assert_called_once_with()
        printer.assert_called_once_with("已在 Krita 中打开 1 张图片")


if __name__ == "__main__":
    unittest.main()
