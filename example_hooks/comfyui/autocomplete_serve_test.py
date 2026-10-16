import io
import json
import logging
import os
import sys
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Dict, List, Optional, Tuple, cast
from unittest.mock import patch, MagicMock

logging.disable(logging.CRITICAL)
from .autocomplete import (
    AutocompleteRequest,
    AutocompleteServices,
    AutocompleteSuggestion,
    autocomplete,
    build_providers,
    serve,
)
from .__main__ import get_parser
from .cancellation import CancelToken


def _request(
    *,
    root_dir: str = "",
    directory_rel_path: str = "",
    cwords: Optional[List[str]] = None,
) -> AutocompleteRequest:
    """构造一个指向 /remix 新增标签上下文的补全请求。"""
    return AutocompleteRequest(
        target_command="remix",
        query="",
        prev_word="",
        cwords=cwords if cwords is not None else ["/remix", "--add"],
        image_paths=[],
        root_dir=root_dir,
        directory_rel_path=directory_rel_path,
        cancel_token=CancelToken(),
    )


def _run_serve(input_lines: List[str]) -> str:
    """用假 stdin/stdout 运行 serve()，返回 stdout 收集到的全部输出。"""
    old_stdin, old_stdout = sys.stdin, sys.stdout
    try:
        sys.stdin = io.StringIO("\n".join(input_lines) + "\n")
        out = io.StringIO()
        sys.stdout = out
        serve()
    finally:
        sys.stdin, sys.stdout = old_stdin, old_stdout
    return out.getvalue()


def _autocomplete_request(req_id: Any, **params: Any) -> str:
    return json.dumps(
        {"jsonrpc": "2.0", "id": req_id, "method": "autocomplete", "params": params}
    )


def _cancel_request(req_id: Any) -> str:
    return json.dumps(
        {"jsonrpc": "2.0", "method": "$/cancelRequest", "params": {"id": req_id}}
    )


class TestServe(unittest.TestCase):

    def test_serve_responds_with_suggestions(self) -> None:
        suggestions = [
            AutocompleteSuggestion(
                text="positive",
                displayText="positive",
                description="正向区域",
                type="region",
                style="",
            )
        ]
        with patch(
            "comfyui.autocomplete.autocomplete", return_value=iter(suggestions)
        ) as mock_ac:
            out = _run_serve(
                [
                    _autocomplete_request(
                        1,
                        cwords=["/add", "--region"],
                        cwordIdx=1,
                        prevWord="--region",
                        linePrefix="/add --region",
                        query="",
                    )
                ]
            )

        request: AutocompleteRequest = mock_ac.call_args.args[0]
        self.assertEqual(request.target_command, "add")
        self.assertEqual(request.query, "")
        self.assertEqual(request.prev_word, "--region")
        self.assertEqual(request.cwords, ["/add", "--region"])
        self.assertEqual(request.image_paths, [])

        lines = [json.loads(l) for l in out.strip().splitlines() if l.strip()]
        self.assertEqual(len(lines), 1)
        resp = lines[0]
        self.assertEqual(resp["jsonrpc"], "2.0")
        self.assertEqual(resp["id"], 1)
        self.assertEqual(
            resp["result"],
            [
                {
                    "text": "positive",
                    "displayText": "positive",
                    "description": "正向区域",
                    "type": "region",
                    "style": "",
                }
            ],
        )

    def test_serve_echoes_request_id(self) -> None:
        with patch("comfyui.autocomplete.autocomplete", return_value=iter([])):
            out = _run_serve([_autocomplete_request(42, cwords=["/add"])])
        resp = json.loads(out.strip())
        self.assertEqual(resp["id"], 42)
        self.assertEqual(resp["result"], [])

    def test_serve_cancel_skips_response(self) -> None:
        def slow_autocomplete(
            request: Any, services: Any
        ) -> List[AutocompleteSuggestion]:
            time.sleep(0.2)
            return []

        with patch("comfyui.autocomplete.autocomplete", side_effect=slow_autocomplete):
            out = _run_serve(
                [
                    _autocomplete_request(1, cwords=["/add"]),
                    _cancel_request(1),
                ]
            )
        # 已取消的请求不应返回结果
        self.assertEqual(out.strip(), "")

    def test_serve_cancel_unknown_id_is_noop(self) -> None:
        with patch(
            "comfyui.autocomplete.autocomplete", return_value=iter([])
        ) as mock_ac:
            out = _run_serve(
                [
                    _autocomplete_request(1, cwords=["/add"]),
                    _cancel_request(999),
                ]
            )
        mock_ac.assert_called_once()
        resp = json.loads(out.strip())
        self.assertEqual(resp["id"], 1)

    def test_serve_ignores_invalid_lines(self) -> None:
        with patch("comfyui.autocomplete.autocomplete", return_value=iter([])):
            out = _run_serve(
                [
                    "this is not json",
                    json.dumps({"jsonrpc": "2.0", "method": "unknown", "params": {}}),
                    _autocomplete_request(7, cwords=["/add"]),
                ]
            )
        resp = json.loads(out.strip())
        self.assertEqual(resp["id"], 7)
        self.assertEqual(resp["result"], [])

    def test_serve_reports_error_response(self) -> None:
        """请求执行失败以 JSON-RPC error 上报，而非静默返回空建议。"""
        with patch(
            "comfyui.autocomplete.autocomplete", side_effect=RuntimeError("boom")
        ):
            out = _run_serve([_autocomplete_request(1, cwords=["/add"])])
        resp = json.loads(out.strip())
        self.assertEqual(resp["jsonrpc"], "2.0")
        self.assertEqual(resp["id"], 1)
        self.assertEqual(resp["error"]["code"], -32000)
        self.assertIn("failed", resp["error"]["message"])
        self.assertNotIn("result", resp)

    def test_serve_error_response_carries_failure_reason(self) -> None:
        """错误消息必须带真实原因：钩子 stderr 只在 Debug 转发，界面上没有第二处线索。"""
        with patch(
            "comfyui.autocomplete.autocomplete",
            side_effect=ValueError("嵌入服务 URL 含多余的成对引号"),
        ):
            out = _run_serve([_autocomplete_request(1, cwords=["/add"])])
        message = json.loads(out.strip())["error"]["message"]
        self.assertIn("ValueError", message)
        self.assertIn("多余的成对引号", message)

    def test_autocomplete_explicit_context_used_for_node_completion(self) -> None:
        """逐请求显式传入的上下文驱动补全：空 query + prev_word=--node 应完成节点补全。"""
        fake_prompt_meta: Dict[str, Any] = {
            "node_1": {"class_type": "CLIPTextEncode", "inputs": {"text": "1girl"}},
            "node_2": {"class_type": "KSampler", "inputs": {"cfg": 8.0}},
        }
        request = AutocompleteRequest(
            target_command="remix",
            query="",
            prev_word="--node",
            cwords=["/remix", "--add", "--node"],
            image_paths=["mock.png"],
            root_dir="",
            directory_rel_path="",
            cancel_token=CancelToken(),
        )
        with build_providers(request, get_parser(), "", False) as providers:
            services = AutocompleteServices(parser=get_parser(), providers=providers)
            with patch(
                "comfyui.autocomplete._load_workflow_data",
                return_value=({}, {"nodes": []}, fake_prompt_meta, None),
            ):
                suggestions = list(autocomplete(request, services))

        # 应返回 CLIPTextEncode 节点，过滤 KSampler（不受任何环境变量影响）
        texts = [s.text for s in suggestions]
        self.assertIn("node_1", texts)
        self.assertNotIn("node_2", texts)

    def test_build_providers_context_manager_enters_and_exits_sqlite_context(
        self,
    ) -> None:
        """验证 build_providers 作为 contextmanager，会在退出时通过 ExitStack 退出 SQLiteContext。"""
        fake_ctx = MagicMock()
        with tempfile.TemporaryDirectory() as data_dir, patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": data_dir}
        ), patch("comfyui.autocomplete.SQLiteContext", return_value=fake_ctx):
            with build_providers(
                _request(root_dir="root", directory_rel_path="dir"),
                get_parser(),
                "http://localhost",
                False,
            ) as providers:
                self.assertEqual(len(providers), 6)
                fake_ctx.__enter__.assert_called_once()
            fake_ctx.__exit__.assert_called_once()

    def test_serve_closes_db_contexts_after_request(self) -> None:
        fake_ctx = MagicMock()
        with tempfile.TemporaryDirectory() as data_dir, patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": data_dir}
        ), patch.dict(os.environ, {"DANBOORU_SEARCH_URL": "http://localhost"}), patch(
            "comfyui.autocomplete.SQLiteContext", return_value=fake_ctx
        ), patch(
            "comfyui.autocomplete.autocomplete", return_value=iter([])
        ):
            _run_serve(
                [
                    _autocomplete_request(
                        1, cwords=["/add"], rootDir="root", directoryRelPath="dir"
                    )
                ]
            )

        fake_ctx.__enter__.assert_called_once()
        fake_ctx.__exit__.assert_called_once()


class _BlockingSearchHandler(BaseHTTPRequestHandler):
    """收到搜索请求后挂住不响应，用来观察取消能否掐断在途请求。

    若取消没有传到网络层，调用要等 HTTP 超时（5s）才返回；测试据此把
    「取消后是否迅速收尾」当作取消确实生效的证据。
    """

    protocol_version = "HTTP/1.1"
    received = threading.Event()
    release = threading.Event()

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler 约定命名
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        # 单次连接：客户端被取消关闭后不必再读下一个请求（否则打印异常噪音）
        self.close_connection = True
        _BlockingSearchHandler.received.set()
        _BlockingSearchHandler.release.wait(timeout=30)
        try:
            body = b'{"results": []}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except OSError:
            # 客户端已因取消关闭连接：写回失败是预期结果
            pass

    def log_message(self, format: str, *args: Any) -> None:
        return


class TestServeCancelIntegration(unittest.TestCase):
    """端到端：$/cancelRequest 经 serve 传到网络层，掐断在途请求且不回响应。"""

    def setUp(self) -> None:
        _BlockingSearchHandler.received.clear()
        _BlockingSearchHandler.release.clear()

    def test_cancel_propagates_to_network_and_task_ends_promptly(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), _BlockingSearchHandler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        host, port = cast(Tuple[str, int], server.server_address[:2])

        # 用真实管道而非预填 stdin：必须等桩服务确已收到在途请求，取消才发得出去，
        # 从而真正覆盖「废除在途 HTTP」而不是「请求还没发就被取消」
        read_fd, write_fd = os.pipe()
        reader = os.fdopen(read_fd, "r", encoding="utf-8")
        writer = os.fdopen(write_fd, "w", encoding="utf-8")
        out = io.StringIO()
        old_stdin, old_stdout = sys.stdin, sys.stdout
        elapsed = float("inf")

        with tempfile.TemporaryDirectory() as data_dir, tempfile.TemporaryDirectory() as root_dir:
            # IMAGE_FUNNEL_DATA_DIR 只提供模型格式配置兜底；其下无 danbooru 产物，
            # 因此本地链路不激活，请求走 DANBOORU_SEARCH_URL 指向的桩服务
            env = {
                "IMAGE_FUNNEL_DATA_DIR": data_dir,
                "DANBOORU_SEARCH_URL": f"http://{host}:{port}",
            }
            with patch.dict(os.environ, env):
                os.environ.pop("DANBOORU_DATA_DIR", None)
                serve_thread = threading.Thread(target=serve, daemon=True)
                try:
                    sys.stdin, sys.stdout = reader, out
                    serve_thread.start()
                    writer.write(
                        _autocomplete_request(
                            1,
                            cwords=["/remix", "--add"],
                            cwordIdx=2,
                            prevWord="",
                            linePrefix="/remix --add ",
                            query="girl",
                            imagePaths=[],
                            rootDir=root_dir,
                            directoryRelPath="",
                        )
                        + "\n"
                    )
                    writer.flush()
                    self.assertTrue(
                        _BlockingSearchHandler.received.wait(10),
                        "在线链路应已发出在途请求",
                    )

                    start = time.monotonic()
                    writer.write(_cancel_request(1) + "\n")
                    writer.flush()
                    # 关闭输入端：serve 的读循环结束并在收尾后返回
                    writer.close()
                    serve_thread.join(timeout=10)
                    elapsed = time.monotonic() - start
                finally:
                    sys.stdin, sys.stdout = old_stdin, old_stdout
                    reader.close()
                    _BlockingSearchHandler.release.set()
                    server.shutdown()
                    server.server_close()

        self.assertFalse(serve_thread.is_alive(), "取消后 serve 应已退出")
        self.assertLess(
            elapsed,
            1.5,
            "取消应掐断在途请求并立刻收尾，而不是等 HTTP 超时（5s）"
            "或收尾超时（2s）自然结束",
        )
        self.assertEqual(out.getvalue().strip(), "", "已取消的请求不应返回响应")


if __name__ == "__main__":
    unittest.main()
