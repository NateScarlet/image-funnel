# 只允许使用项目测试脚本运行测试

import io
import json
import os
import unittest
import urllib.error
from contextlib import redirect_stderr, redirect_stdout
from typing import Any, Callable, Dict, List, Optional, Tuple
from unittest.mock import patch

from .focus import (
    DEFAULT_COMFYUI_URL,
    FOCUS_ROUTE_PATH,
    FocusOutcome,
    FocusRequest,
    FocusState,
    FocusStatus,
    activate_and_confirm,
    build_request,
    describe_outcome,
    focus,
    main,
    parse_sid,
    query_focus,
)
from .window import Window


def _state(status: FocusStatus, title: Optional[str] = None) -> FocusState:
    return FocusState(status=status, last_reported_title=title)


class _FakePlatform:
    """注入的窗口平台替身：记录枚举与激活调用，不接触真实桌面。"""

    def __init__(self, windows: List[Window]) -> None:
        self._windows = windows
        self.window_calls = 0
        self.activated: List[int] = []
        self.requested_substring = ""

    def windows(self, title_substring: str) -> List[Window]:
        self.window_calls += 1
        self.requested_substring = title_substring
        return list(self._windows)

    def activate(self, handle: int) -> bool:
        self.activated.append(handle)
        return True


class _ScriptedQuery:
    """按脚本顺序返回焦点状态，并记录每次查询的参数。"""

    def __init__(self, states: List[FocusState]) -> None:
        self._states = list(states)
        self.calls: List[Tuple[str, str]] = []

    def __call__(self, comfyui_url: str, sid: str) -> FocusState:
        self.calls.append((comfyui_url, sid))
        if not self._states:
            raise AssertionError("unexpected focus query")
        return self._states.pop(0)


def _make_pause() -> Tuple[List[float], Callable[[float], None]]:
    calls: List[float] = []

    def pause(seconds: float) -> None:
        calls.append(seconds)

    return calls, pause


def _activate(
    windows: List[Window],
    states: List[FocusState],
    attempts: int = 3,
) -> Tuple[FocusOutcome, _FakePlatform, _ScriptedQuery, List[float]]:
    platform = _FakePlatform(windows)
    query = _ScriptedQuery(states)
    pauses, pause = _make_pause()
    outcome = activate_and_confirm(
        DEFAULT_COMFYUI_URL,
        "target-sid",
        platform,
        query,
        pause,
        attempts=attempts,
        retry_delay=0.01,
    )
    return outcome, platform, query, pauses


class TestActivateAndConfirm(unittest.TestCase):

    def test_already_focused_skips_window_enumeration_and_activation(self):
        """前置查询确认在前台：不枚举窗口、不调用任何激活原语"""
        outcome, platform, query, _ = _activate(
            [Window(1, "ComfyUI")], [_state(FocusStatus.FOCUSED)]
        )

        self.assertIs(outcome, FocusOutcome.ALREADY_FOCUSED)
        self.assertEqual(platform.window_calls, 0)
        self.assertEqual(platform.activated, [])
        self.assertEqual(len(query.calls), 1)

    def test_remote_page_already_focused_reports_success_without_windows(self):
        """远程访问：本机没有对应窗口，但页面已在前台，不应报未找到窗口"""
        outcome, platform, _, _ = _activate(
            [], [_state(FocusStatus.FOCUSED, "ComfyUI")]
        )

        self.assertIs(outcome, FocusOutcome.ALREADY_FOCUSED)
        self.assertEqual(platform.window_calls, 0)

    def test_single_window_activated_and_confirmed(self):
        """单窗口：激活后查询确认在前台"""
        outcome, platform, query, _ = _activate(
            [Window(7, "ComfyUI")],
            [_state(FocusStatus.NOT_FOCUSED), _state(FocusStatus.FOCUSED)],
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [7])
        self.assertEqual(len(query.calls), 2)

    def test_title_matched_window_is_tried_first_and_stops_on_confirmation(self):
        """标题精确相等的窗口先试；命中即停，不遍历其余窗口"""
        first = Window(1, "ComfyUI - A")
        target = Window(2, "ComfyUI - B")
        outcome, platform, _, _ = _activate(
            [first, target],
            [
                _state(FocusStatus.NOT_FOCUSED, target.title),
                _state(FocusStatus.FOCUSED),
            ],
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [target.handle])

    def test_continues_to_other_windows_when_title_match_is_not_target(self):
        """标题匹配只是优化：确认失败仍要继续尝试其余窗口"""
        title_match = Window(1, "ComfyUI - A")
        target = Window(2, "ComfyUI - B")
        outcome, platform, _, _ = _activate(
            [title_match, target],
            [
                _state(FocusStatus.NOT_FOCUSED, title_match.title),
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.FOCUSED),
            ],
            attempts=1,
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [title_match.handle, target.handle])

    def test_null_title_falls_back_to_enumeration_order(self):
        """last_reported_title 为 null：退化为纯枚举顺序"""
        first = Window(1, "ComfyUI - A")
        second = Window(2, "ComfyUI - B")
        outcome, platform, _, _ = _activate(
            [first, second],
            [_state(FocusStatus.NOT_FOCUSED, None), _state(FocusStatus.FOCUSED)],
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [first.handle])

    def test_unknown_pre_query_degrades_to_trying_every_window(self):
        """前置查询失败（无法确认）：不中断，按枚举顺序逐个试"""
        first = Window(1, "ComfyUI - A")
        second = Window(2, "ComfyUI - B")
        outcome, platform, _, _ = _activate(
            [first, second],
            [_state(FocusStatus.UNKNOWN), _state(FocusStatus.FOCUSED)],
            attempts=1,
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [first.handle])

    def test_all_candidates_unconfirmed_reports_unconfirmed(self):
        """所有候选都确认不了：返回 UNCONFIRMED，不静默声称成功"""
        outcome, platform, _, pauses = _activate(
            [Window(1, "ComfyUI - A"), Window(2, "ComfyUI - B")],
            [
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.NOT_FOCUSED),
            ],
            attempts=2,
        )

        self.assertIs(outcome, FocusOutcome.UNCONFIRMED)
        self.assertEqual(platform.activated, [1, 2])
        self.assertEqual(len(pauses), 2)

    def test_unknown_confirmation_skips_retries_and_moves_to_next_window(self):
        """确认查询无法确认时重试没有意义，直接换下一个候选"""
        outcome, platform, _, pauses = _activate(
            [Window(1, "ComfyUI - A"), Window(2, "ComfyUI - B")],
            [
                _state(FocusStatus.NOT_FOCUSED),
                _state(FocusStatus.UNKNOWN),
                _state(FocusStatus.FOCUSED),
            ],
            attempts=3,
        )

        self.assertIs(outcome, FocusOutcome.CONFIRMED)
        self.assertEqual(platform.activated, [1, 2])
        self.assertEqual(pauses, [])

    def test_no_windows_when_not_focused_reports_no_window(self):
        """前置查询未确认在前台且没有匹配窗口：交给入口按失败处理"""
        outcome, platform, _, _ = _activate([], [_state(FocusStatus.NOT_FOCUSED)])

        self.assertIs(outcome, FocusOutcome.NO_WINDOW)
        self.assertEqual(platform.activated, [])

    def test_missing_sid_still_activates_but_cannot_confirm(self):
        """调用方未拿到 sid（旧版扩展）：仍激活窗口，但无法确认"""
        platform = _FakePlatform([Window(1, "ComfyUI")])
        query = _ScriptedQuery([])
        _, pause = _make_pause()

        outcome = activate_and_confirm(
            DEFAULT_COMFYUI_URL, None, platform, query, pause
        )

        self.assertIs(outcome, FocusOutcome.UNCONFIRMED)
        self.assertEqual(platform.activated, [1])
        self.assertEqual(query.calls, [])


class _FakeResponse:
    def __init__(self, body: bytes) -> None:
        self._body = body

    def read(self) -> bytes:
        return self._body

    def __enter__(self) -> "_FakeResponse":
        return self

    def __exit__(self, *args: object) -> bool:
        return False


def _http_error(code: int) -> urllib.error.HTTPError:
    return urllib.error.HTTPError(
        url="http://127.0.0.1:8188",
        code=code,
        msg="error",
        hdrs=None,  # type: ignore[arg-type]
        fp=None,
    )


class TestQueryFocus(unittest.TestCase):

    def _query_with_body(self, body: bytes) -> Tuple[FocusState, Dict[str, Any]]:
        captured: Dict[str, Any] = {}

        def _urlopen(
            request: Any,
            *args: object,
            **kwargs: object,
        ) -> _FakeResponse:
            captured["url"] = request.full_url
            captured["method"] = request.get_method()
            return _FakeResponse(body)

        with patch("urllib.request.urlopen", _urlopen):
            state = query_focus("http://127.0.0.1:8188/", "abc")
        return state, captured

    def test_parses_focused_state_and_reported_title(self):
        """解析 has_focus 与 last_reported_title"""
        state, captured = self._query_with_body(
            json.dumps(
                {"has_focus": True, "last_reported_title": "ComfyUI - 图"}
            ).encode("utf-8")
        )

        self.assertIs(state.status, FocusStatus.FOCUSED)
        self.assertEqual(state.last_reported_title, "ComfyUI - 图")
        self.assertEqual(
            captured["url"], f"http://127.0.0.1:8188{FOCUS_ROUTE_PATH}/abc"
        )
        self.assertEqual(captured["method"], "GET")

    def test_parses_not_focused_state(self):
        """has_focus 为假：确认不在前台"""
        state, _ = self._query_with_body(
            json.dumps({"has_focus": False, "last_reported_title": None}).encode(
                "utf-8"
            )
        )

        self.assertIs(state.status, FocusStatus.NOT_FOCUSED)
        self.assertIsNone(state.last_reported_title)

    def test_encodes_sid_in_path(self):
        """sid 作为路径段必须编码，避免特殊字符破坏 URL"""
        captured: Dict[str, Any] = {}

        def _urlopen(request: Any, *args: object, **kwargs: object) -> _FakeResponse:
            captured["url"] = request.full_url
            return _FakeResponse(b"{}")

        with patch("urllib.request.urlopen", _urlopen):
            query_focus("http://127.0.0.1:8188", "a b/c")

        self.assertEqual(
            captured["url"], f"http://127.0.0.1:8188{FOCUS_ROUTE_PATH}/a%20b%2Fc"
        )

    def test_404_is_unknown_but_does_not_raise(self):
        """404（路由不存在或 sid 不存在）按无法确认处理"""
        with patch("urllib.request.urlopen", side_effect=_http_error(404)):
            state = query_focus(DEFAULT_COMFYUI_URL, "abc")

        self.assertIs(state.status, FocusStatus.UNKNOWN)
        self.assertIsNone(state.last_reported_title)

    def test_server_error_is_unknown(self):
        """5xx 与网络错误同样按无法确认处理"""
        with patch("urllib.request.urlopen", side_effect=_http_error(500)):
            state = query_focus(DEFAULT_COMFYUI_URL, "abc")

        self.assertIs(state.status, FocusStatus.UNKNOWN)

    def test_connection_error_is_unknown(self):
        """连不上目标时按无法确认处理，不终止整个流程"""
        with patch(
            "urllib.request.urlopen",
            side_effect=urllib.error.URLError("connection refused"),
        ):
            state = query_focus(DEFAULT_COMFYUI_URL, "abc")

        self.assertIs(state.status, FocusStatus.UNKNOWN)

    def test_invalid_json_is_unknown(self):
        """响应不是 JSON 时按无法确认处理"""
        with patch("urllib.request.urlopen", return_value=_FakeResponse(b"not json")):
            state = query_focus(DEFAULT_COMFYUI_URL, "abc")

        self.assertIs(state.status, FocusStatus.UNKNOWN)

    def test_missing_has_focus_is_unknown(self):
        """缺少 has_focus 字段时无法判断，按无法确认处理"""
        with patch(
            "urllib.request.urlopen",
            return_value=_FakeResponse(b'{"last_reported_title": "ComfyUI"}'),
        ):
            state = query_focus(DEFAULT_COMFYUI_URL, "abc")

        self.assertIs(state.status, FocusStatus.UNKNOWN)


class TestFocusEntry(unittest.TestCase):

    def test_no_window_raises_with_explanation(self):
        """focus 的全部目的就是切窗口，没做成就是失败"""
        platform = _FakePlatform([])
        query = _ScriptedQuery([_state(FocusStatus.NOT_FOCUSED)])
        _, pause = _make_pause()

        with self.assertRaises(ValueError) as ctx:
            focus(
                FocusRequest(sid="target-sid", comfyui_url=DEFAULT_COMFYUI_URL),
                platform,
                query,
                pause,
            )

        self.assertIn("未找到", str(ctx.exception))

    def test_confirmed_focus_returns_confirmation_message(self):
        """确认成功时输出同时说明激活与确认结果"""
        platform = _FakePlatform([Window(1, "ComfyUI")])
        query = _ScriptedQuery(
            [_state(FocusStatus.NOT_FOCUSED), _state(FocusStatus.FOCUSED)]
        )
        _, pause = _make_pause()

        result = focus(
            FocusRequest(sid="target-sid", comfyui_url=DEFAULT_COMFYUI_URL),
            platform,
            query,
            pause,
        )

        self.assertEqual(result, describe_outcome(FocusOutcome.CONFIRMED))
        self.assertIn("确认", result)

    def test_unconfirmed_focus_returns_explanation(self):
        """已激活但确认不了时返回说明，而不是静默成功"""
        platform = _FakePlatform([Window(1, "ComfyUI")])
        query = _ScriptedQuery([_state(FocusStatus.UNKNOWN)] * 4)
        _, pause = _make_pause()

        result = focus(
            FocusRequest(sid="target-sid", comfyui_url=DEFAULT_COMFYUI_URL),
            platform,
            query,
            pause,
        )

        self.assertEqual(result, describe_outcome(FocusOutcome.UNCONFIRMED))
        self.assertIn("无法确认", result)

    def test_parse_sid_requires_positional_argument(self):
        """缺少 sid 时按命令行工具惯例输出版式并以非零码退出"""
        stderr = io.StringIO()
        with redirect_stderr(stderr), self.assertRaises(SystemExit) as ctx:
            parse_sid([])

        self.assertNotEqual(ctx.exception.code, 0)
        self.assertIn("sid", stderr.getvalue())

    def test_parse_sid_returns_argument(self):
        self.assertEqual(parse_sid(["abc"]), "abc")

    def test_build_request_uses_default_url(self):
        env = {k: v for k, v in os.environ.items() if k != "COMFYUI_URL"}
        with patch.dict(os.environ, env, clear=True):
            request = build_request("abc")

        self.assertEqual(request.comfyui_url, DEFAULT_COMFYUI_URL)

    def test_build_request_reads_url_from_env(self):
        with patch.dict(os.environ, {"COMFYUI_URL": "http://example:8188"}):
            request = build_request("abc")

        self.assertEqual(request.comfyui_url, "http://example:8188")

    def test_main_prints_result_line(self):
        """入口只向 stdout 打印一行结果"""
        platform = _FakePlatform([Window(1, "ComfyUI")])
        query = _ScriptedQuery(
            [_state(FocusStatus.NOT_FOCUSED), _state(FocusStatus.FOCUSED)]
        )
        with patch("sys.argv", ["comfyui.focus", "abc"]), patch(
            "comfyui.focus.default_window_platform", return_value=platform
        ), patch("comfyui.focus.query_focus", query), patch("comfyui.focus.time.sleep"):
            stdout = io.StringIO()
            with redirect_stdout(stdout):
                main()

        lines = stdout.getvalue().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertIn("确认", lines[0])


if __name__ == "__main__":
    unittest.main()
