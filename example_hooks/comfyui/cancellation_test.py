# -*- coding: utf-8 -*-

import threading
import unittest

from .cancellation import CancelToken, never_canceled_token


class TestCancelToken(unittest.TestCase):
    def test_not_canceled_initially(self) -> None:
        token = CancelToken()
        self.assertFalse(token.is_canceled())

    def test_cancel_marks_and_runs_registered_callbacks(self) -> None:
        token = CancelToken()
        ran: list[str] = []
        token.on_cancel(lambda: ran.append("a"))
        token.on_cancel(lambda: ran.append("b"))
        token.cancel()
        self.assertTrue(token.is_canceled())
        self.assertEqual(ran, ["a", "b"])

    def test_callback_registered_after_cancel_runs_immediately(self) -> None:
        token = CancelToken()
        token.cancel()
        ran: list[str] = []
        token.on_cancel(lambda: ran.append("late"))
        self.assertEqual(ran, ["late"])

    def test_cancel_is_idempotent(self) -> None:
        token = CancelToken()
        ran: list[str] = []
        token.on_cancel(lambda: ran.append("once"))
        token.cancel()
        token.cancel()
        self.assertEqual(ran, ["once"])

    def test_callback_can_register_during_cancel_without_deadlock(self) -> None:
        """清理动作在锁外执行：动作内再次注册（已取消）应立即执行而不是卡死。"""
        token = CancelToken()
        ran: list[str] = []
        token.on_cancel(lambda: token.on_cancel(lambda: ran.append("nested")))
        token.cancel()
        self.assertEqual(ran, ["nested"])

    def test_cancel_from_other_thread_runs_callbacks(self) -> None:
        """取消由另一线程触发（serve 线程取消工作线程），清理动作必须被执行。"""
        token = CancelToken()
        canceled_in_worker = threading.Event()
        token.on_cancel(canceled_in_worker.set)
        threading.Thread(target=token.cancel).start()
        self.assertTrue(canceled_in_worker.wait(5))


class TestNeverCanceledToken(unittest.TestCase):
    """没有取消通道的入口用的令牌：不会取消，且每次调用都是新实例。"""

    def test_token_is_not_canceled(self) -> None:
        self.assertFalse(never_canceled_token().is_canceled())

    def test_each_call_yields_a_fresh_instance(self) -> None:
        """共享单例会让注册上去的清理动作永远累积（无取消事件把它们取走）。"""
        self.assertIsNot(never_canceled_token(), never_canceled_token())


if __name__ == "__main__":
    unittest.main()
