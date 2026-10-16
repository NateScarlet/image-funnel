# -*- coding: utf-8 -*-
"""一次补全请求的取消令牌：请求方不再需要该结果时的信号。

语义见 ADR 0011：取消只保证「请求方不再需要该结果」，接收方尽力尽早停止无谓
工作（如中止在途 HTTP 请求），但**不承诺**打断任意阻塞调用。因此令牌对使用者
只暴露「是否已取消」与「注册一个取消时执行的清理动作」，由各网络层自行注册
中止手段，令牌不感知具体的 HTTP 库类型。
"""

from __future__ import annotations

import threading
from typing import Callable, List

# 取消时执行的清理动作（如关闭在途 HTTP 客户端）
CancelCallback = Callable[[], None]


class CancelToken:
    """每请求一个的取消令牌；重复取消只生效一次。"""

    def __init__(self) -> None:
        self._canceled = threading.Event()
        self._callbacks: List[CancelCallback] = []
        self._lock = threading.Lock()

    def is_canceled(self) -> bool:
        return self._canceled.is_set()

    def on_cancel(self, callback: CancelCallback) -> None:
        """注册取消时执行的清理动作。

        令牌已取消时立即执行：动作与取消信号的先后顺序不影响最终结果。
        """
        with self._lock:
            if not self._canceled.is_set():
                self._callbacks.append(callback)
                return
        callback()

    def cancel(self) -> None:
        """置为已取消并执行已注册的清理动作（由取消方调用，可重复调用）。"""
        with self._lock:
            if self._canceled.is_set():
                return
            self._canceled.set()
            callbacks = self._callbacks
            self._callbacks = []
        # 在锁外执行，避免清理动作再次注册或取消时死锁
        for callback in callbacks:
            callback()
