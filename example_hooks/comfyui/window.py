#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Windows 窗口原语：枚举标题匹配的可见顶层窗口，并把单个窗口带到前台。

本模块只处理窗口句柄与标题，不涉及 HTTP 或 ComfyUI 语义，供 focus 编排层注入使用。
Win32 入口由 ``default_window_platform`` 在最外层构建；构造函数也接受显式注入的
user32/kernel32 替身，因此原语本身可以脱离真实桌面单测。
"""

import ctypes
from ctypes import wintypes
from dataclasses import dataclass
from typing import Any, List, Protocol, cast

# GetWindow 的 GW_HWNDNEXT：按 Z 序取下一个顶层窗口
_GW_HWNDNEXT = 2

# ShowWindow 的 SW_RESTORE：只用于最小化窗口，避免把最大化的窗口还原成原始尺寸
_SW_RESTORE = 9

# keybd_event 的虚拟键码与标志（合成 ALT 按键解除前台窗口切换锁定）
_VK_MENU = 0x12
_KEYEVENTF_KEYUP = 0x0002


@dataclass(frozen=True)
class Window:
    """一个可见顶层窗口：句柄与枚举时刻的标题"""

    handle: int
    title: str


class WindowPlatform(Protocol):
    """编排层依赖的窗口能力：只做枚举与激活"""

    def windows(self, title_substring: str) -> List[Window]: ...

    def activate(self, handle: int) -> bool: ...


def _hwnd_value(handle: Any) -> int:
    """把 ctypes 返回的句柄统一转成 int；0/None 统一为空句柄。"""
    value = getattr(handle, "value", handle)
    return int(value or 0)


class Win32WindowPlatform:
    """基于 Win32 API 的窗口平台实现。

    user32/kernel32 由调用方注入，既可以是 ``ctypes.windll`` 上的真实 DLL，
    也可以是测试替身；本类不在构造时自行读取全局状态以外的依赖。
    """

    def __init__(self, user32: Any, kernel32: Any) -> None:
        self._user32 = user32
        self._kernel32 = kernel32
        # 句柄是 64 位指针，必须声明 restype，否则 ctypes 会按 c_int 截断
        user32.GetTopWindow.restype = wintypes.HWND
        user32.GetWindow.restype = wintypes.HWND
        user32.GetForegroundWindow.restype = wintypes.HWND

    def windows(self, title_substring: str) -> List[Window]:
        """按 Z 序返回所有可见且标题包含 ``title_substring`` 的顶层窗口"""
        found: List[Window] = []
        hwnd = _hwnd_value(self._user32.GetTopWindow(None))
        while hwnd:
            if self._user32.IsWindowVisible(wintypes.HWND(hwnd)):
                title = self._window_title(hwnd)
                if title_substring in title:
                    found.append(Window(handle=hwnd, title=title))
            hwnd = _hwnd_value(
                self._user32.GetWindow(wintypes.HWND(hwnd), _GW_HWNDNEXT)
            )
        return found

    def activate(self, handle: int) -> bool:
        """把窗口带到前台，返回最终是否成为前台窗口。

        钩子进程通常由后台服务拉起，Windows 只允许持有前台窗口的线程切换前台。
        依次尝试：直接调用、合成 ALT 按键解除锁定后重试、附加到当前前台线程后重试，
        最后用 BringWindowToTop 兜底。
        """
        user32 = self._user32
        window = wintypes.HWND(handle)

        if user32.IsIconic(window):
            user32.ShowWindow(window, _SW_RESTORE)

        if user32.SetForegroundWindow(window):
            return True

        self._simulate_alt_keypress()
        if user32.SetForegroundWindow(window):
            return True

        foreground = user32.GetForegroundWindow()
        foreground_handle = _hwnd_value(foreground)
        if foreground_handle:
            foreground_thread = user32.GetWindowThreadProcessId(foreground, None)
            current_thread = self._kernel32.GetCurrentThreadId()
            if foreground_thread and foreground_thread != current_thread:
                attached = bool(
                    user32.AttachThreadInput(current_thread, foreground_thread, True)
                )
                try:
                    user32.SetForegroundWindow(window)
                finally:
                    if attached:
                        user32.AttachThreadInput(
                            current_thread, foreground_thread, False
                        )

        user32.BringWindowToTop(window)
        return _hwnd_value(user32.GetForegroundWindow()) == handle

    def _window_title(self, hwnd: int) -> str:
        """读取窗口标题；读取失败时按空标题处理（自然不匹配）"""
        buffer = ctypes.create_unicode_buffer(512)
        self._user32.GetWindowTextW(wintypes.HWND(hwnd), buffer, len(buffer))
        return cast(str, buffer.value)

    def _simulate_alt_keypress(self) -> None:
        """合成一次 ALT 按键：系统在用户按下 ALT 后放开前台窗口切换限制"""
        user32 = self._user32
        user32.keybd_event(_VK_MENU, 0, 0, 0)
        user32.keybd_event(_VK_MENU, 0, _KEYEVENTF_KEYUP, 0)


def default_window_platform() -> WindowPlatform:
    """在最外层入口构建真实的 Win32 窗口平台"""
    return Win32WindowPlatform(ctypes.windll.user32, ctypes.windll.kernel32)
