import io
import json
import logging
import os
import sys
import time
import unittest
from typing import Any, Dict, List
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

    def test_autocomplete_explicit_context_used_for_node_completion(self) -> None:
        """逐请求显式传入的上下文驱动补全：空 query + prev_word=--node 应完成节点补全。"""
        fake_prompt_meta: Dict[str, Any] = {
            "node_1": {"class_type": "CLIPTextEncode", "inputs": {"text": "1girl"}},
            "node_2": {"class_type": "KSampler", "inputs": {"cfg": 8.0}},
        }
        request = AutocompleteRequest(
            target_command="add",
            query="",
            prev_word="--node",
            cwords=["/add", "--node"],
            image_paths=["mock.png"],
            root_dir="",
            directory_rel_path="",
        )
        with build_providers("", "", "", False) as providers:
            services = AutocompleteServices(parser=get_parser(), providers=providers)
            with patch(
                "comfyui.autocomplete._load_workflow_data",
                return_value=({}, {"nodes": []}, fake_prompt_meta),
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
        with patch("comfyui.autocomplete.SQLiteContext", return_value=fake_ctx):
            with build_providers("root", "dir", "http://localhost", False) as providers:
                self.assertEqual(len(providers), 7)
                fake_ctx.__enter__.assert_called_once()
            fake_ctx.__exit__.assert_called_once()

    def test_serve_closes_db_contexts_after_request(self) -> None:
        fake_ctx = MagicMock()
        with patch.dict(os.environ, {"DANBOORU_SEARCH_URL": "http://localhost"}), patch(
            "comfyui.autocomplete.SQLiteContext", return_value=fake_ctx
        ), patch("comfyui.autocomplete.autocomplete", return_value=iter([])):
            _run_serve(
                [
                    _autocomplete_request(
                        1, cwords=["/add"], rootDir="root", directoryRelPath="dir"
                    )
                ]
            )

        fake_ctx.__enter__.assert_called_once()
        fake_ctx.__exit__.assert_called_once()


if __name__ == "__main__":
    unittest.main()
