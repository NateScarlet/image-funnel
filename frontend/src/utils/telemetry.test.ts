import { beforeEach, describe, expect, test, vi } from "vitest";

const sentry = vi.hoisted(() => ({
  init: vi.fn(),
  captureMessage: vi.fn(),
  addBreadcrumb: vi.fn(),
  vueIntegration: vi.fn(() => ({ name: "Vue" })),
}));

vi.mock("@sentry/vue", () => sentry);

import { captureMarkPerformance, captureNavigationEvent, initTelemetry } from "./telemetry";

describe("导航链路单事件上报", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test("中间步骤写入面包屑，不发事件", () => {
    captureNavigationEvent({ step: "decision-started", directoryId: "dir:a" });

    expect(sentry.addBreadcrumb).toHaveBeenCalledTimes(1);
    expect(sentry.addBreadcrumb.mock.calls[0]?.[0]).toMatchObject({
      category: "post-commit-navigation",
      data: expect.objectContaining({ step: "decision-started", directoryId: "dir:a" }),
    });
    expect(sentry.captureMessage).not.toHaveBeenCalled();
  });

  test("终止步骤只发一个事件，携带完整步骤信息", () => {
    captureNavigationEvent({ step: "error-classified", errorClass: "local" });

    expect(sentry.captureMessage).toHaveBeenCalledTimes(1);
    expect(sentry.captureMessage.mock.calls[0]?.[1]).toMatchObject({
      fingerprint: ["post-commit-navigation", "error-classified", "local"],
      level: "error",
      extra: expect.objectContaining({ step: "error-classified", errorClass: "local" }),
    });
  });

  test("成功导航只写面包屑，不产生事件", () => {
    captureNavigationEvent({ step: "navigated-to-session", nextDirectoryId: "dir:a" });

    expect(sentry.addBreadcrumb).toHaveBeenCalledTimes(1);
    expect(sentry.captureMessage).not.toHaveBeenCalled();
  });

  test("未切换视图与视图闪烁各自产生一个事件", () => {
    captureNavigationEvent({ step: "navigation-not-switched", errorClass: "local" });
    captureNavigationEvent({ step: "navigation-flicker", errorClass: "local" });

    expect(sentry.captureMessage).toHaveBeenCalledTimes(2);
    expect(sentry.captureMessage.mock.calls.map((call) => call[1]?.fingerprint[1])).toEqual([
      "navigation-not-switched",
      "navigation-flicker",
    ]);
  });

  test("只有超过阈值的性能事件才上报，阈值为 0 时关闭", () => {
    const performance = {
      sessionId: "session-1",
      round: 0,
      imageId: "img:1:a.png",
      loadMs: 500,
      markIntervalMs: 200,
      autoRejectTimeoutMs: 0,
      dpr: 1,
      viewportWidth: 800,
      viewportHeight: 600,
    };

    captureMarkPerformance(performance, 300);
    expect(sentry.captureMessage).toHaveBeenCalledTimes(1);

    vi.clearAllMocks();
    captureMarkPerformance({ ...performance, loadMs: 300 }, 300);
    expect(sentry.captureMessage).not.toHaveBeenCalled();

    vi.clearAllMocks();
    captureMarkPerformance(performance, 0);
    expect(sentry.captureMessage).not.toHaveBeenCalled();

    captureMarkPerformance(performance, null);
    expect(sentry.captureMessage).not.toHaveBeenCalled();

    captureMarkPerformance(performance, undefined);
    expect(sentry.captureMessage).not.toHaveBeenCalled();
  });

  test("初始化时注入官方 Vue 集成、release 与发送前脱敏钩子", () => {
    const app = { marker: "app" } as never;
    initTelemetry("https://key@example.com/1", app);

    const options = sentry.init.mock.calls[0]?.[0];
    expect(sentry.vueIntegration).toHaveBeenCalledWith(
      expect.objectContaining({ app, attachProps: false }),
    );
    expect(options).toMatchObject({
      integrations: [expect.objectContaining({ name: "Vue" })],
      release: expect.any(String),
    });
    expect(options?.release).not.toBe("");
    expect(options?.beforeSend).toBeTypeOf("function");
  });
});

describe("发送前过滤取消噪音", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function send(event: Record<string, unknown>): unknown {
    initTelemetry("https://key@example.com/1", {} as never);
    const beforeSend = sentry.init.mock.calls[0]?.[0]?.beforeSend as (e: unknown) => unknown;
    return beforeSend(event);
  }

  const cancellation = {
    exception: {
      values: [
        {
          type: "AbortError",
          value: "The operation was aborted.",
          mechanism: {
            type: "auto.browser.global_handlers.onunhandledrejection",
            handled: false,
          },
        },
      ],
    },
  };

  test("丢弃全局未捕获的 Apollo 取消 AbortError", () => {
    expect(send(cancellation)).toBeNull();
  });

  test("保留显式上报的同类 AbortError", () => {
    const event = {
      exception: {
        values: [
          {
            type: "AbortError",
            value: "The operation was aborted.",
            mechanism: { type: "generic" },
          },
        ],
      },
    };

    expect(send(event)).not.toBeNull();
  });

  test("保留其他错误并照常脱敏", () => {
    const event = {
      exception: { values: [{ type: "Error", value: "failed at C:/Users/me/secret" }] },
      extra: {},
      tags: {},
      contexts: {},
    };

    const result = send(event) as {
      extra: Record<string, unknown>;
      exception: { values: { value: string }[] };
    };

    expect(result).not.toBeNull();
    expect(result.extra.scrubSalt).toEqual(expect.any(String));
    expect(result.exception.values[0]?.value).not.toContain("C:/Users/me/secret");
  });
});
