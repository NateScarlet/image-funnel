import { describe, test, expect, vi } from "vitest";
import { createClient } from "graphql-ws";
import type { Client } from "graphql-ws";
import { wsClientOptions } from "./client";

// 每个测试独立的测试床：连接尝试计数与客户端列表互不串扰
// （dispose 后仍可能有挂起的重试周期，共享计数器会跨测试污染）
function makeHarness() {
  let attempts = 0;
  // 拆除标记：dispose 与无限重试周期存在竞态，先停止派发失败事件再销毁，
  // 避免悬挂的拒绝泄漏为未处理错误
  let open = true;
  const clients: Client[] = [];

  class HandshakeFailingSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    onopen: (() => void) | null = null;
    onmessage: ((ev: unknown) => void) | null = null;
    onerror: ((ev: Event) => void) | null = null;
    onclose: ((ev: { code: number; reason: string; wasClean: boolean }) => void) | null = null;
    readyState = HandshakeFailingSocket.CONNECTING;

    constructor() {
      attempts++;
      queueMicrotask(() => {
        // 模拟网关 502：握手未完成——浏览器先派发裸 error 事件（无 close code），
        // 随后才会派发 close(1006)，但库在 error 处即注销监听，close 不会被消费
        if (!open) {
          return;
        }
        this.onerror?.(new Event("error"));
        this.onclose?.({ code: 1006, reason: "", wasClean: false });
      });
    }

    send(): void {}

    close(): void {}
  }

  return {
    getAttempts: () => attempts,
    create(options: Parameters<typeof createClient>[0]) {
      const client = createClient({
        ...options,
        webSocketImpl: HandshakeFailingSocket as unknown as typeof WebSocket,
        // 用 setTimeout 让渡事件循环：无限重试若全走微任务会饿死测试进程；
        // 置于展开之后以覆盖被测配置自带的退避节奏——节奏不是本测试的关注点
        retryWait: () => new Promise((resolve) => setTimeout(resolve, 0)),
      });
      clients.push(client);
      return client;
    },
    disposeAll() {
      open = false;
      clients.forEach((client) => client.dispose());
      clients.length = 0;
    },
  };
}

describe("常驻 WS 连接的握手失败重试", () => {
  // 项目生产配置（wsClientOptions）：握手失败风暴下持续发起新的连接尝试，
  // 停机窗口内不放弃，恢复后自动重连并触发版本失配检查
  test("项目配置下握手失败风暴持续发起重试", async () => {
    const harness = makeHarness();
    try {
      harness.create({ ...wsClientOptions, url: "ws://fake/graphql" });

      await vi.waitFor(() => expect(harness.getAttempts()).toBeGreaterThanOrEqual(5));
    } finally {
      harness.disposeAll();
    }
  });

  // 对照组：锁定库默认策略确实会首次握手失败即永久放弃——
  // 这是 wsClientOptions 中显式 shouldRetry 存在的理由，若上游改默认行为则此断言失效提醒我们复查
  test("对照：库默认策略首次握手失败即放弃", async () => {
    const harness = makeHarness();
    try {
      harness.create({ url: "ws://fake/graphql", lazy: false });

      // 给足多轮微任务与定时器轮转的机会
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(harness.getAttempts()).toBe(1);
    } finally {
      harness.disposeAll();
    }
  });
});
