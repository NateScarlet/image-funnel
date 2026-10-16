# -*- coding: utf-8 -*-

import http.server
import threading
import time
import unittest
from contextlib import contextmanager
from typing import Any, Callable, Generator, List, Tuple, cast

from .cancellable_http import HttpCanceledError, HttpError, post_json
from .cancellation import CancelToken


class _RecordingHandler(http.server.BaseHTTPRequestHandler):
    """记录收到的请求并立即以固定状态码响应。"""

    protocol_version = "HTTP/1.1"
    status = 200
    body = b"{}"
    requests_seen: List[str] = []
    lock = threading.Lock()

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler 约定命名
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        with _RecordingHandler.lock:
            _RecordingHandler.requests_seen.append(self.path)
        self.send_response(_RecordingHandler.status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(_RecordingHandler.body)))
        self.end_headers()
        self.wfile.write(_RecordingHandler.body)

    def log_message(self, format: str, *args: Any) -> None:
        return


class _SlowHandler(http.server.BaseHTTPRequestHandler):
    """收到请求后挂住不响应，直到测试放行——用来观察在途请求能否被取消掐断。"""

    protocol_version = "HTTP/1.1"
    received = threading.Event()
    release = threading.Event()

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler 约定命名
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        # 单次连接：客户端被取消关闭后不要再去读下一个请求（否则 socketserver 打印异常噪音）
        self.close_connection = True
        _SlowHandler.received.set()
        _SlowHandler.release.wait(timeout=30)
        try:
            body = b"{}"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except OSError:
            # 客户端已被取消关闭：写回失败是预期结果
            pass

    def log_message(self, format: str, *args: Any) -> None:
        return


@contextmanager
def _stub_server(
    handler: type[http.server.BaseHTTPRequestHandler],
) -> Generator[str, None, None]:
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        host, port = cast(Tuple[str, int], server.server_address[:2])
        yield f"http://{host}:{port}/api/search"
    finally:
        _SlowHandler.release.set()
        server.shutdown()
        server.server_close()


class _CancelOnRegistration(CancelToken):
    """在注册清理动作的那一刻取消：复现「注册完成、请求尚未发出」这一竞态窗口。"""

    def on_cancel(self, callback: Callable[[], None]) -> None:
        super().on_cancel(callback)
        self.cancel()


class TestPostJson(unittest.TestCase):
    def setUp(self) -> None:
        _RecordingHandler.requests_seen.clear()
        _RecordingHandler.status = 200
        _RecordingHandler.body = b'{"results": []}'
        _SlowHandler.received.clear()
        _SlowHandler.release.clear()

    def test_returns_parsed_body(self) -> None:
        with _stub_server(_RecordingHandler) as url:
            body = post_json(
                url, {"query": "x"}, timeout=5.0, cancel_token=CancelToken()
            )
        self.assertEqual(body, {"results": []})
        self.assertEqual(_RecordingHandler.requests_seen, ["/api/search"])

    def test_http_error_status_wrapped(self) -> None:
        _RecordingHandler.status = 500
        with _stub_server(_RecordingHandler) as url:
            with self.assertRaises(HttpError):
                post_json(url, {}, timeout=5.0, cancel_token=CancelToken())

    def test_cancel_before_request_skips_network_call(self) -> None:
        token = CancelToken()
        token.cancel()
        with _stub_server(_RecordingHandler) as url:
            with self.assertRaises(HttpCanceledError):
                post_json(url, {}, timeout=5.0, cancel_token=token)
        self.assertEqual(_RecordingHandler.requests_seen, [])

    def test_cancel_aborts_inflight_request(self) -> None:
        """对本地慢速服务发起请求后触发取消：调用应在阈值内抛错退出。"""
        with _stub_server(_SlowHandler) as url:
            token = CancelToken()
            errors: List[Exception] = []

            def call() -> None:
                try:
                    post_json(url, {}, timeout=30.0, cancel_token=token)
                except Exception as e:  # noqa: BLE001 - 记录实际抛出的异常类型
                    errors.append(e)

            worker = threading.Thread(target=call, daemon=True)
            worker.start()
            self.assertTrue(_SlowHandler.received.wait(5), "服务端应已收到在途请求")
            start = time.monotonic()
            token.cancel()
            worker.join(timeout=5)
            elapsed = time.monotonic() - start

        self.assertFalse(worker.is_alive(), "取消后调用应立即退出而不是等请求自然完成")
        self.assertLess(elapsed, 2.0, "取消应在阈值内掐断在途请求")
        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], HttpCanceledError)

    def test_cancel_between_registration_and_send_raises_canceled_error(self) -> None:
        """注册清理动作与发起请求之间被取消：报取消而不是 httpx 的 RuntimeError。"""
        token = _CancelOnRegistration()
        with _stub_server(_RecordingHandler) as url:
            with self.assertRaises(HttpCanceledError):
                post_json(url, {}, timeout=5.0, cancel_token=token)
        self.assertEqual(_RecordingHandler.requests_seen, [])

    def test_canceled_error_is_not_an_http_error(self) -> None:
        """取消不是失败：取消错误刻意与 HttpError 无继承关系，避免调用方按失败处理。"""
        self.assertFalse(issubclass(HttpCanceledError, HttpError))


if __name__ == "__main__":
    unittest.main()
