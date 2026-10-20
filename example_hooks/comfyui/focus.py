#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""「激活并确认 ComfyUI 接收页面」入口：``comfyui.focus <sid>``。

只做两件事：把 ComfyUI 窗口带到前台，并通过 comfyui-nodes 的
``GET /io.github.natescarlet/focus/{sid}`` 确认目标页面此刻是否在前台。
不读取图片、不发送工作流；``open_workflow`` 送达后复用同一份激活确认逻辑。

确认状态随 websocket 连接销毁，因此每次调用都要重新查询；查询失败（404 / 网络 /
5xx）按「无法确认」降级，不阻止激活动作。sid 由调用方显式传入，本入口不读取或
持久化「上一次的 sid」。
"""

import argparse
import json
import os
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from enum import Enum
from typing import Any, Callable, Dict, List, Optional, cast

from .window import (
    Window,
    WindowPlatform,
    default_window_platform,
)

# comfyui-nodes 暴露的焦点状态查询路由（读取路径为 <route>/{sid}）
FOCUS_ROUTE_PATH = "/io.github.natescarlet/focus"

# 目标窗口标题的硬编码子串；不做模糊匹配、前缀匹配或大小写归一化
WINDOW_TITLE_SUBSTRING = "ComfyUI"

# 默认 ComfyUI 服务地址，与其它 comfyui 入口保持一致
DEFAULT_COMFYUI_URL = "http://127.0.0.1:8188"

# 单个候选窗口内的确认重试：浏览器 focus 事件到 HTTP 上报到达后端有几十毫秒延迟
CONFIRM_ATTEMPTS = 5
CONFIRM_RETRY_DELAY_SECONDS = 0.1

# 查询是「此刻」的状态，不能让一次卡住的连接拖死整个钩子
_REQUEST_TIMEOUT_SECONDS = 2.0


class FocusStatus(Enum):
    """一次焦点查询的三种结果；不能用布尔值压缩，因为降级行为不同"""

    FOCUSED = "focused"
    NOT_FOCUSED = "not_focused"
    UNKNOWN = "unknown"


@dataclass(frozen=True)
class FocusState:
    """焦点查询结果：状态与该 sid 最后一次上报时的标题快照"""

    status: FocusStatus
    last_reported_title: Optional[str]


class FocusOutcome(Enum):
    """「激活并确认」的最终结果"""

    ALREADY_FOCUSED = "already_focused"
    CONFIRMED = "confirmed"
    UNCONFIRMED = "unconfirmed"
    NO_WINDOW = "no_window"


# 查询协议：(ComfyUI 服务地址, sid) -> 焦点状态
FocusQuerier = Callable[[str, str], FocusState]

# 重试等待协议：测试注入空实现即可避免真实等待
Pause = Callable[[float], None]


@dataclass(frozen=True)
class FocusRequest:
    """一次 focus 请求的上下文（由入口构造并注入）"""

    sid: str
    comfyui_url: str


def _parse_focus_payload(payload: Any) -> FocusState:
    """校验查询响应并转为 FocusState；结构不符合契约时按无法确认处理"""
    if not isinstance(payload, dict):
        return FocusState(FocusStatus.UNKNOWN, None)

    mapping = cast(Dict[str, Any], payload)
    has_focus = mapping.get("has_focus")
    if not isinstance(has_focus, bool):
        return FocusState(FocusStatus.UNKNOWN, None)

    title = mapping.get("last_reported_title")
    if title is not None and not isinstance(title, str):
        return FocusState(FocusStatus.UNKNOWN, None)

    status = FocusStatus.FOCUSED if has_focus else FocusStatus.NOT_FOCUSED
    return FocusState(status, title)


def query_focus(comfyui_url: str, sid: str) -> FocusState:
    """查询目标 sid 对应的页面此刻是否在前台。

    404（路由不存在或 sid 不存在）、网络错误、5xx、响应不合法都返回 UNKNOWN，
    由调用方按「无法确认」降级；这里不抛出异常，因为激活本身仍然有价值。
    """
    url = (
        f"{comfyui_url.rstrip('/')}{FOCUS_ROUTE_PATH}/"
        f"{urllib.parse.quote(sid, safe='')}"
    )
    request = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(
            request, timeout=_REQUEST_TIMEOUT_SECONDS
        ) as response:
            payload: Any = json.loads(response.read())
    except (OSError, ValueError):
        return FocusState(FocusStatus.UNKNOWN, None)
    return _parse_focus_payload(payload)


def _order_candidates(
    windows: List[Window], last_reported_title: Optional[str]
) -> List[Window]:
    """把标题精确匹配的窗口排到最前，其余保持枚举顺序。

    标题只是优化：缺失或没有精确匹配时退化为纯枚举顺序。
    """
    if last_reported_title is None:
        return windows
    matched = [window for window in windows if window.title == last_reported_title]
    rest = [window for window in windows if window.title != last_reported_title]
    return matched + rest


def activate_and_confirm(
    comfyui_url: str,
    sid: Optional[str],
    platform: WindowPlatform,
    query: FocusQuerier,
    pause: Pause,
    attempts: int = CONFIRM_ATTEMPTS,
    retry_delay: float = CONFIRM_RETRY_DELAY_SECONDS,
) -> FocusOutcome:
    """激活 ComfyUI 窗口并按 sid 确认切到的是接收页面。

    前置查询确认已在前台时直接成功，不枚举窗口、不调用任何激活原语（也覆盖远程访问：
    本机没有对应窗口，但页面确实在前台）。sid 缺失时（旧版扩展未返回）仍激活窗口，
    只是无法确认，最终结果与确认失败一致。
    """
    initial = query(comfyui_url, sid) if sid else FocusState(FocusStatus.UNKNOWN, None)
    if initial.status is FocusStatus.FOCUSED:
        return FocusOutcome.ALREADY_FOCUSED

    candidates = platform.windows(WINDOW_TITLE_SUBSTRING)
    if not candidates:
        return FocusOutcome.NO_WINDOW

    for window in _order_candidates(candidates, initial.last_reported_title):
        platform.activate(window.handle)
        if not sid:
            continue
        for attempt in range(attempts):
            state = query(comfyui_url, sid)
            if state.status is FocusStatus.FOCUSED:
                return FocusOutcome.CONFIRMED
            if state.status is FocusStatus.UNKNOWN:
                # 无法确认（404/网络/5xx）时重试没有意义，换下一个候选
                break
            if attempt + 1 < attempts:
                pause(retry_delay)

    return FocusOutcome.UNCONFIRMED


def describe_outcome(outcome: FocusOutcome) -> str:
    """把激活确认结果翻译成给用户看的一句话（不含句号）"""
    if outcome is FocusOutcome.ALREADY_FOCUSED:
        return "接收页面已在前台，无需切换窗口"
    if outcome is FocusOutcome.CONFIRMED:
        return "已激活 ComfyUI 窗口并确认接收页面在前台"
    if outcome is FocusOutcome.UNCONFIRMED:
        return "已激活 ComfyUI 窗口，但无法确认是否为接收页面"
    if outcome is FocusOutcome.NO_WINDOW:
        return "未找到标题包含 ComfyUI 的可见窗口"
    raise ValueError(f"unknown focus outcome: {outcome}")


def focus(
    request: FocusRequest,
    platform: WindowPlatform,
    query: FocusQuerier,
    pause: Pause,
) -> str:
    """激活并确认目标页面，返回给用户看的结果描述。

    focus 的全部目的就是切窗口：前置查询未确认在前台、又没有找到候选窗口时快速失败，
    由入口以非零退出码上报。找到窗口但确认不了时仍算成功，并如实说明无法确认。
    """
    outcome = activate_and_confirm(
        request.comfyui_url, request.sid, platform, query, pause
    )
    if outcome is FocusOutcome.NO_WINDOW:
        raise ValueError(describe_outcome(outcome))
    return describe_outcome(outcome)


def parse_sid(argv: Optional[List[str]] = None) -> str:
    """解析必填的 sid 命令行参数；缺失或为空时按命令行工具惯例退出"""
    parser = argparse.ArgumentParser(
        description="激活 ComfyUI 窗口并确认接收页面在前台"
    )
    parser.add_argument(
        "sid", help="目标页面 sid，由 comfyui.open_workflow 送达工作流时返回"
    )
    sid = cast(str, parser.parse_args(argv).sid).strip()
    if not sid:
        parser.error("sid must not be empty")
    return sid


def build_request(sid: str) -> FocusRequest:
    """单次模式：入口从环境变量构造请求上下文（缺失即用默认值）"""
    return FocusRequest(
        sid=sid,
        comfyui_url=os.environ.get("COMFYUI_URL", DEFAULT_COMFYUI_URL),
    )


def main() -> None:
    request = build_request(parse_sid())
    platform = default_window_platform()
    print(focus(request, platform, query_focus, time.sleep))


if __name__ == "__main__":
    main()
