#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""发送到 Krita 钩子：把选中的图片交给本机 Krita 打开。

优先通过 krita-plugin 的 openImages RPC 在 Krita 内同步打开图片，等图片成为
活动文档后激活窗口再清理标签；插件未监听时回退为命令行打开并跳过清理。Hook
Runner 通过管道捕获钩子输出并等待写端关闭，因此回退启动时必须把 Krita 的
标准流全部重定向。核心逻辑不读取环境变量，图片列表与可执行文件路径由最外层
入口构造并注入。
"""

import ctypes
import glob
import json
import logging
import os
import queue
import shutil
import subprocess
import threading
import time
from ctypes import wintypes
from dataclasses import dataclass
from typing import Any, BinaryIO, Callable, List, Optional, Protocol, Tuple, cast

_LOGGER = logging.getLogger(__name__)

# KRITA_EXE 未设置且 PATH 中找不到 krita 时的兜底探测位置
_KRITA_INSTALL_PATTERNS = (r"C:\Program Files\Krita*\bin\krita.exe",)


@dataclass(frozen=True)
class KritaRequest:
    """一次「发送到 Krita」请求的上下文（由入口构造并注入）"""

    image_paths: List[str]
    krita_exe: str


# 启动器协议：接收 Krita 可执行文件与待打开的图片路径
Spawner = Callable[[str, List[str]], None]


def open_in_krita(request: KritaRequest, spawn: Spawner) -> int:
    """把请求中的图片一次性交给 Krita 打开，返回处理的图片数量。

    单次启动传入全部路径：Krita 会把它们全部打开到同一个实例里。
    """
    if not request.image_paths:
        raise ValueError("No image paths to open in Krita.")
    spawn(request.krita_exe, request.image_paths)
    return len(request.image_paths)


# Krita 插件 RPC 的注入接缝：打开图片与清理标签两个动作
class KritaRpcClient(Protocol):
    def open_images(self, paths: List[str]) -> None: ...

    def prune_tabs(self) -> None: ...


def run_hook(
    request: KritaRequest,
    spawn: Spawner,
    activate: Callable[[], None],
    rpc_client: KritaRpcClient,
) -> int:
    """让插件打开图片后激活窗口并清理标签，返回打开的图片数。

    有监听端时先同步打开图片，图片成为活动文档后才激活窗口、发送 pruneTabs；
    无监听端（冷启动或插件未启用）时回退为命令行打开并跳过清理，不等待 pipe
    就绪。RPC 错误、非法响应与超时由客户端抛出并原样传播给调用者。
    """
    try:
        rpc_client.open_images(request.image_paths)
    except PipeNotAvailable:
        # 冷启动没有旧标签可清理，插件未监听时也没有清理能力，按原方式打开即可
        count = open_in_krita(request, spawn)
        activate()
        return count
    activate()
    rpc_client.prune_tabs()
    return len(request.image_paths)


# #region Krita 插件 RPC 客户端

# 监听端固定在 krita-plugin 注册的 pipe 上，换行分隔 JSON-RPC
_KRITA_PLUGIN_PIPE = r"\\.\pipe\io.github.natescarlet.krita-plugin"
_RESPONSE_TIMEOUT_SECONDS = 5.0
_PRUNE_TABS_REQUEST = (
    json.dumps(
        {"jsonrpc": "2.0", "method": "pruneTabs", "params": {}, "id": 1},
        separators=(",", ":"),
    ).encode("utf-8")
    + b"\n"
)


def _open_images_request(paths: List[str]) -> bytes:
    """构造 openImages 请求：参数放在 params.paths，由插件在 Krita 内同步打开。"""
    return (
        json.dumps(
            {
                "jsonrpc": "2.0",
                "method": "openImages",
                "params": {"paths": paths},
                "id": 1,
            },
            separators=(",", ":"),
        ).encode("utf-8")
        + b"\n"
    )


class PipeNotAvailable(Exception):
    """连接或写入失败：本机没有监听端，由编排层决定是否回退。"""


class RpcError(Exception):
    """监听端返回错误响应，或响应不符合协议。"""


class PipeTransport(Protocol):
    """named pipe 系统边界：写入请求并带超时读取一行响应。"""

    def request(self, path: str, payload: bytes, timeout_seconds: float) -> bytes: ...


class PipeKritaRpcClient:
    """把 openImages / pruneTabs 请求交给可注入的管道 transport。

    传输层失败（PipeNotAvailable、超时）与非法响应一律向上抛出，由编排层
    决定无监听端时回退、其余失败时让调用者可见。
    """

    def __init__(self, transport: PipeTransport) -> None:
        self._transport = transport

    def open_images(self, paths: List[str]) -> None:
        raw = self._transport.request(
            _KRITA_PLUGIN_PIPE,
            _open_images_request(paths),
            _RESPONSE_TIMEOUT_SECONDS,
        )
        _parse_rpc_response(raw)

    def prune_tabs(self) -> None:
        raw = self._transport.request(
            _KRITA_PLUGIN_PIPE, _PRUNE_TABS_REQUEST, _RESPONSE_TIMEOUT_SECONDS
        )
        _parse_rpc_response(raw)


def _parse_rpc_response(raw: bytes) -> None:
    """把响应分类为「正常完成」或「失败」，失败必须让调用者可见。"""
    try:
        response: object = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RpcError("krita-plugin 返回了非法 JSON 响应") from exc
    if not isinstance(response, dict):
        raise RpcError("krita-plugin 响应不是 JSON 对象")
    if "error" in response:
        raise RpcError(f"krita-plugin 返回错误：{response['error']}")
    if "result" not in response:
        raise RpcError("krita-plugin 响应缺少 result")


def _read_line_with_timeout(pipe: BinaryIO, timeout_seconds: float) -> bytes:
    """在守护线程里读取一行；读取有超时上限，超时以异常告知调用者。"""
    outcome: queue.Queue[Tuple[Optional[bytes], Optional[BaseException]]] = queue.Queue(
        maxsize=1
    )

    def read() -> None:
        try:
            outcome.put((pipe.readline(), None))
        except BaseException as exc:
            outcome.put((None, exc))

    threading.Thread(target=read, daemon=True).start()
    try:
        line, error = outcome.get(timeout=timeout_seconds)
    except queue.Empty:
        raise TimeoutError("等待 krita-plugin 响应超时") from None
    if error is not None:
        raise error
    return cast(bytes, line)


class NamedPipeTransport:
    """真实管道 transport：仅在最外层入口构建，供 RPC 客户端注入。"""

    def request(self, path: str, payload: bytes, timeout_seconds: float) -> bytes:
        try:
            pipe = open(path, "w+b", buffering=0)
        except OSError as exc:
            raise PipeNotAvailable(str(exc)) from exc
        try:
            try:
                pipe.write(payload)
            except OSError as exc:
                raise PipeNotAvailable(str(exc)) from exc
            return _read_line_with_timeout(pipe, timeout_seconds)
        finally:
            pipe.close()


# #endregion


# #region 窗口激活

# GetWindow 的 GW_HWNDNEXT：按 Z 序取下一个顶层窗口
_GW_HWNDNEXT = 2
# ShowWindow 的 SW_RESTORE：把最小化的窗口还原到原始尺寸和位置。
# 只对最小化的窗口使用——对已最大化的窗口调用会把它还原成最大化之前的大小。
_SW_RESTORE = 9
# 等待 Krita 窗口出现的上限：冷启动要加载资源，可能耗时数十秒
_WINDOW_WAIT_SECONDS = 60.0


# OpenProcess 的 PROCESS_QUERY_LIMITED_INFORMATION 权限
_PROCESS_QUERY_LIMITED_INFORMATION = 0x1000


def window_process_id(hwnd: int) -> int:
    """返回窗口所属进程的 ID，查询失败时返回 0。"""
    pid = wintypes.DWORD()
    ctypes.windll.user32.GetWindowThreadProcessId(
        wintypes.HWND(hwnd), ctypes.byref(pid)
    )
    return int(pid.value)


def window_process_name(hwnd: int) -> str:
    """返回窗口所属进程的可执行文件名（小写），查询失败时返回空串。

    标题可能被其他程序复用（例如 Everything 搜索 "krita" 时标题里也含 krita），
    因此用进程名而不是标题来识别窗口归属。
    """
    pid = window_process_id(hwnd)
    if not pid:
        return ""
    kernel32 = ctypes.windll.kernel32
    process = kernel32.OpenProcess(_PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
    if not process:
        return ""
    try:
        buffer = ctypes.create_unicode_buffer(1024)
        size = wintypes.DWORD(len(buffer))
        if not kernel32.QueryFullProcessImageNameW(
            process, 0, buffer, ctypes.byref(size)
        ):
            return ""
        return os.path.basename(buffer.value).lower()
    finally:
        kernel32.CloseHandle(process)


def find_krita_window() -> int:
    """查找可见的 Krita 主窗口，返回句柄，找不到时返回 0。

    按进程名识别 Krita。有多个候选窗口时（如启动画面），优先返回标题带文档名
    的那个（形如 "<文件名> (<大小>) - Krita"）。
    """
    user32 = ctypes.windll.user32
    user32.GetTopWindow.restype = wintypes.HWND
    user32.GetWindow.restype = wintypes.HWND
    candidates: List[Tuple[int, str]] = []
    hwnd = user32.GetTopWindow(None)
    while hwnd:
        if (
            user32.IsWindowVisible(wintypes.HWND(hwnd))
            and window_process_name(int(hwnd)) == "krita.exe"
        ):
            title = ctypes.create_unicode_buffer(512)
            user32.GetWindowTextW(wintypes.HWND(hwnd), title, len(title))
            candidates.append((int(hwnd), title.value))
        hwnd = user32.GetWindow(wintypes.HWND(hwnd), _GW_HWNDNEXT)

    if not candidates:
        return 0
    for hwnd_value, title_value in candidates:
        if title_value.endswith("- Krita"):
            return hwnd_value
    return candidates[0][0]


# keybd_event 的虚拟键码与标志
_VK_MENU = 0x12
_KEYEVENTF_KEYUP = 0x0002


def simulate_alt_keypress() -> None:
    """合成一次 ALT 按键，解除 Windows 对前台窗口切换的锁定。

    SetForegroundWindow 只对满足特定条件的进程生效（见微软文档），由后台服务
    拉起的钩子进程不满足其中任何一条，直接调用会被拒绝，表现为任务栏闪烁。
    系统在用户按下 ALT 键后会放开这一限制，合成 ALT 按键可达到同样效果，
    这是 forcefocus 一类工具的通行做法。
    """
    user32 = ctypes.windll.user32
    user32.keybd_event(_VK_MENU, 0, 0, 0)
    user32.keybd_event(_VK_MENU, 0, _KEYEVENTF_KEYUP, 0)


def focus_window(hwnd: int) -> bool:
    """把窗口带到前台，返回是否成功激活。

    Windows 只允许持有前台窗口的线程切换前台窗口。钩子进程由后台服务拉起，
    通常不具备该权限。依次尝试：直接调用、合成 ALT 按键解除前台锁定后重试、
    附加到当前前台线程后重试，最后用 BringWindowToTop 兜底。
    只有最小化的窗口才需要还原，最大化的窗口保持原状。
    """
    user32 = ctypes.windll.user32
    kernel32 = ctypes.windll.kernel32
    window = wintypes.HWND(hwnd)
    user32.GetForegroundWindow.restype = wintypes.HWND

    if user32.IsIconic(window):
        user32.ShowWindow(window, _SW_RESTORE)

    if user32.SetForegroundWindow(window):
        return True

    # 直接调用被拒绝：合成 ALT 按键解除前台锁定后重试
    simulate_alt_keypress()
    if user32.SetForegroundWindow(window):
        return True

    foreground = user32.GetForegroundWindow()
    if foreground:
        foreground_thread = user32.GetWindowThreadProcessId(
            wintypes.HWND(foreground), None
        )
        current_thread = kernel32.GetCurrentThreadId()
        if foreground_thread and foreground_thread != current_thread:
            attached = bool(
                user32.AttachThreadInput(current_thread, foreground_thread, True)
            )
            try:
                user32.SetForegroundWindow(window)
            finally:
                if attached:
                    user32.AttachThreadInput(current_thread, foreground_thread, False)

    user32.BringWindowToTop(window)
    return int(user32.GetForegroundWindow() or 0) == hwnd


def activate_krita_window(timeout_seconds: float = _WINDOW_WAIT_SECONDS) -> None:
    """等待 Krita 窗口出现并把它带到前台，超时仍未出现则报错。

    复用已有实例时窗口已经存在，立即返回；冷启动则要等窗口建好。
    """
    deadline = time.monotonic() + timeout_seconds
    while True:
        hwnd = find_krita_window()
        if hwnd:
            focus_window(hwnd)
            return
        if time.monotonic() >= deadline:
            raise RuntimeError(f"等待 {timeout_seconds:.0f} 秒后仍未找到 Krita 窗口")
        time.sleep(0.2)


# #endregion


def spawn_krita(krita_exe: str, image_paths: List[str]) -> None:
    """启动 Krita 并立即返回，不等待其退出。

    三个标准流全部重定向到 DEVNULL 并关闭其余可继承句柄，使 Krita 不持有
    Hook Runner 的捕获管道。不使用 DETACHED_PROCESS：它会让 Krita 失去设置
    前台窗口的能力。
    """
    creationflags = 0
    if os.name == "nt":
        creationflags = subprocess.CREATE_NEW_PROCESS_GROUP
    subprocess.Popen(
        [krita_exe, *image_paths],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        close_fds=True,
        creationflags=creationflags,
    )


def find_krita_exe() -> str:
    """定位 Krita 可执行文件：KRITA_EXE > PATH > 常见安装位置。"""
    explicit = os.environ.get("KRITA_EXE", "").strip()
    if explicit:
        return explicit

    found = shutil.which("krita")
    if found:
        return found

    for pattern in _KRITA_INSTALL_PATTERNS:
        matches = sorted(glob.glob(pattern))
        if matches:
            return matches[-1]

    raise RuntimeError(
        "找不到 Krita 可执行文件：请把 krita 加入 PATH，或设置 KRITA_EXE 环境变量"
    )


def _parse_string_list(raw: str, env_name: str) -> List[str]:
    """解析 JSON 字符串数组（可信边界校验：要求恰好是字符串列表，否则报错）"""
    items: List[Any] = json.loads(raw)
    for item in items:
        if not isinstance(item, str):
            raise ValueError(f"{env_name} must be a JSON array of strings.")
    return cast(List[str], items)


def build_request_from_env() -> KritaRequest:
    """单次模式：入口从环境变量构造请求上下文（缺失即报错，快速失败）。"""
    raw_paths = os.environ.get("IMAGE_FUNNEL_IMAGE_PATHS")
    if not raw_paths:
        raise ValueError("Environment variable IMAGE_FUNNEL_IMAGE_PATHS is missing.")
    return KritaRequest(
        image_paths=_parse_string_list(raw_paths, "IMAGE_FUNNEL_IMAGE_PATHS"),
        krita_exe=find_krita_exe(),
    )


def main() -> None:
    request = build_request_from_env()
    _LOGGER.debug(
        "Opening %d image(s) in Krita: %s",
        len(request.image_paths),
        request.image_paths,
    )
    rpc_client = PipeKritaRpcClient(NamedPipeTransport())
    count = run_hook(request, spawn_krita, activate_krita_window, rpc_client)
    print(f"已在 Krita 中打开 {count} 张图片")


if __name__ == "__main__":
    main()
