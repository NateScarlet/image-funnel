# -*- coding: utf-8 -*-
"""可取消的同步 JSON HTTP 调用：常驻补全（serve）路径的网络出口。

同步 `requests` 的阻塞调用无法从另一线程干净中断，因此这里的客户端用 httpx，
并把「关闭客户端」注册到取消令牌上：取消时由取消线程关闭客户端，令在途请求
立刻抛错退出。取消只保证「不再需要该结果」，不承诺打断任意阻塞调用（ADR 0011）。

不向上暴露 httpx 类型：调用方只看到 HttpError / HttpCanceledError。
"""

from __future__ import annotations

from typing import Any, Mapping, Optional

import httpx

from .cancellation import CancelToken


class HttpError(Exception):
    """同步 HTTP 调用失败：网络、超时、HTTP 状态码、响应非 JSON。"""


class HttpCanceledError(Exception):
    """在途 HTTP 调用因取消被掐断：请求方已不需要该结果，这不是失败。

    刻意不做 HttpError 的子类：取消不是失败，把它当失败处理会让调用方照常记错误
    日志、给用户挂「上游失败」提示，甚至降级后继续把剩余计算跑完（ADR 0011 的
    取消语义正好相反）。取消只保证不回传结果，所以它必须与失败分开传播。
    """


def post_json(
    url: str,
    payload: Mapping[str, Any],
    *,
    timeout: float,
    cancel_token: CancelToken,
    headers: Optional[Mapping[str, str]] = None,
) -> Any:
    """发起一次可取消的同步 JSON POST，返回解析后的响应体。

    取消检查点之一在发起请求之前：已取消的请求不再发起网络调用。客户端一旦
    发起请求即注册到取消令牌，取消线程关闭它即可掐断在途连接。
    """
    if cancel_token.is_canceled():
        raise HttpCanceledError("请求已取消，未发起网络请求")
    with httpx.Client() as client:
        # 网络层自行注册中止手段，令牌不感知具体 HTTP 库类型
        cancel_token.on_cancel(client.close)
        try:
            response = client.post(
                url, json=dict(payload), headers=headers, timeout=timeout
            )
            response.raise_for_status()
            return response.json()
        except httpx.HTTPError as e:
            # 客户端被取消线程关闭时，在途请求以传输错误（如 ReadError）结束
            if cancel_token.is_canceled():
                raise HttpCanceledError("请求已取消，在途请求已被掐断") from e
            raise HttpError(f"HTTP 请求失败: {e}") from e
        except RuntimeError as e:
            # 注册清理动作与发起请求之间收到取消：客户端已被取消线程关闭，
            # httpx 拒绝在已关闭的客户端上发请求（抛 RuntimeError 而非 HTTPError）
            if cancel_token.is_canceled():
                raise HttpCanceledError("请求已取消，未发起网络请求") from e
            raise
        except ValueError as e:
            raise HttpError(f"HTTP 响应不是合法 JSON: {e}") from e
