# 只允许使用项目测试脚本运行测试

import unittest
from typing import Any, Callable, List, Tuple

from .window import Window, Win32WindowPlatform


def _hwnd_value(handle: Any) -> int:
    """把 ctypes 句柄或测试替身的整数统一还原为 int。"""
    value = getattr(handle, "value", handle)
    return int(value or 0)


class _Func:
    """ctypes 函数替身：允许设置 restype，调用时转发到注入的实现。"""

    def __init__(self, fn: Callable[..., Any]) -> None:
        self._fn = fn
        self.restype: Any = None

    def __call__(self, *args: Any) -> Any:
        return self._fn(*args)


class FakeUser32:
    """按 Z 序枚举窗口的 user32 替身，只实现窗口原语实际用到的接口。"""

    def __init__(
        self,
        windows: List[Tuple[int, str, bool]],
        iconic: Tuple[int, ...] = (),
        foreground: int = 0,
        set_foreground_results: Tuple[bool, ...] = (True,),
    ) -> None:
        self.order = [handle for handle, _, _ in windows]
        self.titles = {handle: title for handle, title, _ in windows}
        self.visible = {handle: visible for handle, _, visible in windows}
        self.iconic = set(iconic)
        self.foreground = foreground
        self.pending_set_foreground_results = list(set_foreground_results)
        self.set_foreground_calls: List[int] = []
        self.show_window_calls: List[Tuple[int, int]] = []
        self.key_events: List[int] = []
        self.bring_to_top_calls: List[int] = []
        self.attach_calls: List[Tuple[int, int, bool]] = []

        self.GetTopWindow = _Func(self._top_window)
        self.GetWindow = _Func(self._get_window)
        self.IsWindowVisible = _Func(self._is_window_visible)
        self.IsIconic = _Func(self._is_iconic)
        self.GetWindowTextW = _Func(self._get_window_text)
        self.ShowWindow = _Func(self._show_window)
        self.SetForegroundWindow = _Func(self._set_foreground)
        self.GetForegroundWindow = _Func(self._get_foreground)
        self.keybd_event = _Func(self._keybd_event)
        self.GetWindowThreadProcessId = _Func(self._get_window_thread_process_id)
        self.AttachThreadInput = _Func(self._attach_thread_input)
        self.BringWindowToTop = _Func(self._bring_window_to_top)

    def _top_window(self, _parent: Any) -> int:
        return self.order[0] if self.order else 0

    def _is_window_visible(self, hwnd: Any) -> bool:
        return self.visible.get(_hwnd_value(hwnd), False)

    def _is_iconic(self, hwnd: Any) -> bool:
        return _hwnd_value(hwnd) in self.iconic

    def _get_foreground(self) -> int:
        return self.foreground

    def _keybd_event(self, vk: int, _scan: int, _flags: int, _extra: int) -> None:
        self.key_events.append(vk)

    def _get_window_thread_process_id(self, hwnd: Any, _pid: Any) -> int:
        return 100 + _hwnd_value(hwnd)

    def _bring_window_to_top(self, hwnd: Any) -> None:
        self.bring_to_top_calls.append(_hwnd_value(hwnd))

    def _get_window(self, hwnd: Any, _command: int) -> int:
        handle = _hwnd_value(hwnd)
        index = self.order.index(handle)
        return self.order[index + 1] if index + 1 < len(self.order) else 0

    def _get_window_text(self, hwnd: Any, buffer: Any, _size: int) -> int:
        title = self.titles.get(_hwnd_value(hwnd), "")
        buffer.value = title
        return len(title)

    def _show_window(self, hwnd: Any, command: int) -> int:
        handle = _hwnd_value(hwnd)
        self.show_window_calls.append((handle, command))
        self.iconic.discard(handle)
        return 1

    def _set_foreground(self, hwnd: Any) -> int:
        handle = _hwnd_value(hwnd)
        self.set_foreground_calls.append(handle)
        if self.pending_set_foreground_results:
            succeeded = self.pending_set_foreground_results.pop(0)
        else:
            succeeded = False
        if succeeded:
            self.foreground = handle
        return int(succeeded)

    def _attach_thread_input(self, current: int, target: int, attach: bool) -> int:
        self.attach_calls.append((current, target, attach))
        return 1


class FakeKernel32:
    def __init__(self, current_thread: int = 1) -> None:
        self.GetCurrentThreadId = _Func(lambda: current_thread)


def _make_platform(
    windows: List[Tuple[int, str, bool]],
    **kwargs: Any,
) -> Tuple[Win32WindowPlatform, FakeUser32]:
    user32 = FakeUser32(windows, **kwargs)
    return Win32WindowPlatform(user32, FakeKernel32()), user32


class TestMatchingWindows(unittest.TestCase):

    def test_returns_visible_titled_windows_in_z_order(self):
        """只返回可见且标题包含目标子串的窗口，顺序沿用 Z 序枚举顺序"""
        platform, _ = _make_platform(
            [
                (1, "ComfyUI", True),
                (2, "记事本", True),
                (3, "workflow - ComfyUI", True),
                (4, "ComfyUI（隐藏）", False),
            ]
        )

        self.assertEqual(
            platform.windows("ComfyUI"),
            [Window(1, "ComfyUI"), Window(3, "workflow - ComfyUI")],
        )

    def test_title_match_is_case_sensitive_substring(self):
        """标题匹配沿用硬编码子串，不做大小写或模糊归一化"""
        platform, _ = _make_platform(
            [
                (1, "comfyui", True),
                (2, "ComfyUI", True),
            ]
        )

        self.assertEqual(platform.windows("ComfyUI"), [Window(2, "ComfyUI")])

    def test_returns_empty_list_when_no_window_matches(self):
        """没有匹配窗口时返回空列表，由编排层决定降级行为"""
        platform, _ = _make_platform([(1, "记事本", True)])

        self.assertEqual(platform.windows("ComfyUI"), [])


class TestActivateWindow(unittest.TestCase):

    def test_activates_window_without_restoring_normal_window(self):
        """已恢复的窗口直接激活：不调用 ShowWindow 改变其尺寸状态"""
        platform, user32 = _make_platform(
            [(7, "ComfyUI", True)], set_foreground_results=(True,)
        )

        self.assertTrue(platform.activate(7))
        self.assertEqual(user32.show_window_calls, [])
        self.assertEqual(user32.set_foreground_calls, [7])

    def test_restores_minimized_window_before_activating(self):
        """最小化的窗口先还原，再尝试带到前台"""
        platform, user32 = _make_platform(
            [(7, "ComfyUI", True)], iconic=(7,), set_foreground_results=(True,)
        )

        self.assertTrue(platform.activate(7))
        self.assertEqual(user32.show_window_calls, [(7, 9)])
        self.assertEqual(user32.set_foreground_calls, [7])

    def test_retries_after_simulated_alt_when_first_attempt_refused(self):
        """首次被前台锁定拒绝时，合成 ALT 按键解除限制后重试"""
        platform, user32 = _make_platform(
            [(7, "ComfyUI", True)], set_foreground_results=(False, True)
        )

        self.assertTrue(platform.activate(7))
        self.assertEqual(user32.key_events, [0x12, 0x12])
        self.assertEqual(user32.set_foreground_calls, [7, 7])

    def test_reports_failure_when_window_never_reaches_foreground(self):
        """所有激活原语都失败时如实返回 False，不假装成功"""
        platform, user32 = _make_platform(
            [(7, "ComfyUI", True)],
            foreground=99,
            set_foreground_results=(False, False),
        )

        self.assertFalse(platform.activate(7))
        self.assertEqual(user32.bring_to_top_calls, [7])


if __name__ == "__main__":
    unittest.main()
