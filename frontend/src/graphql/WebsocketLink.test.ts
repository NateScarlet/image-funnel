import { describe, test, expect, vi, beforeEach } from "vitest";

const mockClient = {
  subscribe: vi.fn(() => () => {}),
};

vi.mock("graphql-ws", async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>();
  return {
    ...actual,
    createClient: vi.fn(() => mockClient),
  };
});

import WebSocketLink from "./WebsocketLink";
import { createClient } from "graphql-ws";
import type { ClientOptions } from "graphql-ws";
import { websocketConnected } from "@/events";

// 创建被测实例并从 createClient 入参中取出合并后的 connected 回调；
// 实现必须向 createClient 注册该回调，否则快速失败
function setup(options: Partial<ClientOptions> = {}) {
  const link = new WebSocketLink({ url: "ws://test/graphql", ...options });
  const clientOptions = vi.mocked(createClient).mock.lastCall?.[0];
  const connected = clientOptions?.on?.connected;
  if (!connected) {
    throw new Error("createClient 未收到 on.connected 回调");
  }
  return {
    link,
    // payload 固定 undefined，仅暴露测试关心的参数
    fireConnected: (wasRetry = false) => {
      connected("socket", undefined, wasRetry);
    },
  };
}

describe("WebSocketLink 连接建立广播", () => {
  beforeEach(() => {
    vi.mocked(createClient).mockClear();
    mockClient.subscribe.mockClear();
  });

  test("连接建立时广播全局 websocketConnected 事件", () => {
    const { fireConnected } = setup();
    const listener = vi.fn();
    const unsubscribe = websocketConnected.subscribe(listener);

    fireConnected();

    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener.mock.calls[0][0]).toBeInstanceOf(CustomEvent);

    unsubscribe();
  });

  test("每次连接建立（重连）都重新广播事件", () => {
    const { fireConnected } = setup();
    const listener = vi.fn();
    const unsubscribe = websocketConnected.subscribe(listener);

    fireConnected(false);
    fireConnected(true);

    expect(listener).toHaveBeenCalledTimes(2);

    unsubscribe();
  });

  test("透传调用方自带的 on.connected 回调", () => {
    const passthrough = vi.fn();
    const { fireConnected } = setup({ on: { connected: passthrough } });

    fireConnected();

    expect(passthrough).toHaveBeenCalledTimes(1);
  });
});
