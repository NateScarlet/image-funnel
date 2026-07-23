import { describe, test, expect, vi, beforeEach } from "vitest";

// #region 测试数据
// 使用 any 类型绕过 NotificationFragment 的完整类型要求（测试只需必要的字段）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyNotification = any;
const mockNotification = (overrides: Record<string, unknown> = {}): AnyNotification => ({
  id: "notif-1",
  title: "测试通知",
  body: "",
  priority: "NORMAL",
  status: "ACTIVE",
  notBefore: new Date(0).toISOString(),
  notAfter: "9999-12-31T23:59:59.999Z",
  readAt: null,
  dismissedAt: null,
  detailURL: null,
  ...overrides,
});
// #endregion

// #region Mock 函数
const mockShowToast = vi.hoisted(() => vi.fn(() => 1));
const mockRemoveToast = vi.hoisted(() => vi.fn());
const mockRefreshOn = vi.hoisted(() => vi.fn());
const mockIsPast = vi.hoisted(() => vi.fn(() => false));
const mockIsFuture = vi.hoisted(() => vi.fn(() => false));
const mockCurrentTime = vi.hoisted(() => {
  let val: unknown = {};
  return {
    get value() {
      return val;
    },
    set value(v: unknown) {
      val = v;
    },
  };
});

const mockChannelsData = vi.hoisted(() => {
  let val: unknown = { notificationChannels: { nodes: [] } };
  return {
    get value() {
      return val;
    },
    set value(v: unknown) {
      val = v;
    },
  };
});
const mockUseQuery = vi.hoisted(() =>
  vi.fn(() => ({ data: mockChannelsData, loading: { value: false }, refresh: vi.fn() })),
);
const mockUseSubscription = vi.hoisted(() => vi.fn());
const mockQuery = vi.hoisted(() => vi.fn());
const mockMutate = vi.hoisted(() => vi.fn());
const mockModalDrawer = vi.hoisted(() => ({
  isOpen: { value: false },
  open: vi.fn(),
  close: vi.fn(),
}));
// #endregion

// #region Mock 模块
// 绕过 once() 确保每个测试获得独立的 composable 实例
vi.mock("es-toolkit", () => ({
  once: (fn: (...args: unknown[]) => unknown) => fn,
}));
vi.mock("@/composables/useNotification", () => ({
  default: vi.fn(() => ({ show: mockShowToast, remove: mockRemoveToast })),
}));
vi.mock("@/composables/useCurrentTime", () => ({
  default: vi.fn(() => ({
    currentTime: mockCurrentTime,
    refreshOn: mockRefreshOn,
    isPast: mockIsPast,
    isFuture: mockIsFuture,
    refresh: vi.fn(),
  })),
}));
vi.mock("@/composables/useModalDrawer", () => ({
  default: vi.fn((opts: { onDidClose?: () => void }) => {
    (mockModalDrawer as Record<string, unknown>).onDidClose = opts.onDidClose;
    return mockModalDrawer;
  }),
}));
vi.mock("@/graphql/utils/useQuery", () => ({ default: mockUseQuery }));
vi.mock("@/graphql/utils/useSubscription", () => ({ default: mockUseSubscription }));
vi.mock("@/graphql/utils/query", () => ({ default: mockQuery }));
vi.mock("@/graphql/utils/mutate", () => ({ default: mockMutate }));
vi.mock("@/graphql/generated", () => ({
  NotificationChannelsDocument: "NotificationChannelsDocument",
  NotificationChangedDocument: "NotificationChangedDocument",
  NotificationsDocument: "NotificationsDocument",
  UpdateNotificationDocument: "UpdateNotificationDocument",
  NotificationEventType: { SENT: "SENT", UNSENT: "UNSENT", UPDATED: "UPDATED" },
  NotificationPriority: { HIGH: "HIGH", NORMAL: "NORMAL", LOW: "LOW" },
  NotificationStatus: { ACTIVE: "ACTIVE" },
}));
// #endregion

import useNotificationCenter from "./useNotificationCenter";

describe("useNotificationCenter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockIsPast.mockReturnValue(false);
    mockIsFuture.mockReturnValue(false);
    mockChannelsData.value = { notificationChannels: { nodes: [] } };
    mockQuery.mockResolvedValue({ data: { notificationChannels: { nodes: [] } } });
  });

  // #region spawnToast 时间窗口判断
  describe("spawnToast", () => {
    test("notBefore 在未来时，不显示 toast 但注册到 scheduledNotifications", () => {
      mockIsFuture.mockImplementation(
        ((v: unknown) => v === "2060-01-01") as unknown as () => boolean,
      );
      const { spawnToast, scheduledNotifications } = useNotificationCenter();

      const n = mockNotification({ notBefore: "2060-01-01" });
      spawnToast(n);

      expect(mockShowToast).not.toHaveBeenCalled();
      expect(scheduledNotifications.value.size).toBe(1);
      expect(scheduledNotifications.value.get("notif-1")?.id).toBe("notif-1");
    });

    test("notAfter 已过期时，不显示 toast", () => {
      mockIsPast.mockImplementation(
        ((v: unknown) => v === "2020-01-01") as unknown as () => boolean,
      );
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ notAfter: "2020-01-01" });
      spawnToast(n);

      expect(mockShowToast).not.toHaveBeenCalled();
    });

    test("无时间约束时，立即显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      spawnToast(mockNotification());

      expect(mockShowToast).toHaveBeenCalledTimes(1);
    });

    test("notBefore 已过且 notAfter 未到时，立即显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ notBefore: "2020-01-01", notAfter: "2060-01-01" });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledTimes(1);
    });

    test("同一通知不重复显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification();
      spawnToast(n);
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledTimes(1);
    });

    test("readAt 已设置的通知不显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ readAt: "2025-01-01T00:00:00.000Z" });
      spawnToast(n);

      expect(mockShowToast).not.toHaveBeenCalled();
    });

    test("dismissedAt 已设置的通知不显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ dismissedAt: "2025-01-01T00:00:00.000Z" });
      spawnToast(n);

      expect(mockShowToast).not.toHaveBeenCalled();
    });

    test("readAt 和 dismissedAt 都为 null 时正常显示 toast", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ readAt: null, dismissedAt: null });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledTimes(1);
    });

    test("HIGH 优先级通知 toast 不可自动关闭", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ priority: "HIGH" });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledWith(
        "测试通知",
        "info",
        0,
        expect.arrayContaining([expect.objectContaining({ text: "关闭" })]),
        "",
      );
    });

    test("NORMAL 优先级通知 toast 有自动关闭时长", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ priority: "NORMAL" });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledWith(
        "测试通知",
        "info",
        expect.any(Number),
        undefined,
        "",
      );
      expect(mockShowToast).toHaveBeenCalledTimes(1);
    });

    test("带 body 的通知触发 spawnToast 时将 body 传入 toast 载荷", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ title: "带正文通知", body: "这是通知正文" });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledWith(
        "带正文通知",
        "info",
        expect.any(Number),
        undefined,
        "这是通知正文",
      );
    });

    test("不带 body 的通知触发 spawnToast 时传入空 body", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({ title: "无正文通知", body: "" });
      spawnToast(n);

      expect(mockShowToast).toHaveBeenCalledWith(
        "无正文通知",
        "info",
        expect.any(Number),
        undefined,
        "",
      );
    });

    test("非 hooks 频道带 detailsURL 的通知生成查看详情按钮且点击在 window.open 中打开", () => {
      const windowOpenSpy = vi.spyOn(window, "open").mockImplementation(() => null);
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({
        priority: "HIGH",
        channel: "system",
        detailsURL: "https://example.com/details",
      });
      spawnToast(n);

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const actions = (mockShowToast.mock.calls[0] as any[])[3];
      expect(actions).toEqual(
        expect.arrayContaining([expect.objectContaining({ text: "查看详情" })]),
      );

      const detailAction = actions.find((a: { text: string }) => a.text === "查看详情");
      const closeFn = vi.fn();
      detailAction.onClick(closeFn);

      expect(windowOpenSpy).toHaveBeenCalledWith("https://example.com/details", "_blank");
      expect(closeFn).toHaveBeenCalled();

      windowOpenSpy.mockRestore();
    });

    test("hooks 频道带 detailsURL 的通知生成查看正文和查看详情按钮", () => {
      const { spawnToast } = useNotificationCenter();

      const n = mockNotification({
        priority: "HIGH",
        channel: "hooks",
        detailsURL: "https://example.com/hook-details",
      });
      spawnToast(n);

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const actions = (mockShowToast.mock.calls[0] as any[])[3];
      const texts = actions.map((a: { text: string }) => a.text);
      expect(texts).toEqual(["查看正文", "查看详情", "关闭"]);
    });
  });
  // #endregion

  // #region hideToast
  describe("hideToast", () => {
    test("调用 removeToast 并清理 toastIdMap", () => {
      const { spawnToast, hideToast } = useNotificationCenter();

      const n = mockNotification();
      spawnToast(n);

      const toastId = mockShowToast.mock.results[0].value;
      hideToast(n.id);

      expect(mockRemoveToast).toHaveBeenCalledWith(toastId);
    });

    test("hideToast 对不存在的通知不报错", () => {
      const { hideToast } = useNotificationCenter();

      expect(() => hideToast("nonexistent")).not.toThrow();
      expect(mockRemoveToast).not.toHaveBeenCalled();
    });
  });
  // #endregion

  // #region scheduledNotifications 管理
  describe("scheduledNotifications", () => {
    test("有 time 约束的通知加入调度列表", () => {
      mockIsFuture.mockImplementation(
        ((v: unknown) => v === "2060-01-01") as unknown as () => boolean,
      );
      const { spawnToast, scheduledNotifications } = useNotificationCenter();

      spawnToast(mockNotification({ notBefore: "2060-01-01" }));
      spawnToast(mockNotification({ id: "notif-2", notAfter: "2060-01-01" }));

      expect(scheduledNotifications.value.size).toBe(2);
    });

    test("更新已存在的通知", () => {
      mockIsFuture.mockImplementation(
        ((v: unknown) => v === "2060-01-01") as unknown as () => boolean,
      );
      const { spawnToast, scheduledNotifications } = useNotificationCenter();

      const n = mockNotification({ notBefore: "2060-01-01" });
      spawnToast(n);

      // 更新同一通知
      const updated = mockNotification({ notBefore: "2060-01-01", notAfter: "2070-01-01" });
      spawnToast(updated);

      expect(scheduledNotifications.value.size).toBe(1);
      expect(scheduledNotifications.value.get("notif-1")?.notAfter).toBe("2070-01-01");
    });
  });
  // #endregion

  // #region 通知撤回
  describe("通知撤回", () => {
    test("UNSENT 时服务端设置 notAfter 为当前时间，spawnToast 自然不显示", () => {
      mockIsPast.mockImplementation(
        ((v: unknown) => v === "2020-01-01") as unknown as () => boolean,
      );
      const { scheduledNotifications } = useNotificationCenter();

      const subscriptionCallback = mockUseSubscription.mock.calls[0]?.[1]?.onNext;
      // UNSENT 后服务端设置 notAfter 为当前时间（已过期）
      const n = mockNotification({ notAfter: "2020-01-01" });

      subscriptionCallback({
        data: { notificationChanged: { event: "UNSENT", notification: n } },
      });

      // notAfter 已过期，不显示 toast
      expect(mockShowToast).not.toHaveBeenCalled();
      // 有 time 约束（notAfter），加入调度列表供 watch 管理生命周期
      expect(scheduledNotifications.value.size).toBe(1);
    });
  });
  // #endregion

  // #region 频道选择与自动已读
  describe("selectChannel 与自动标记已读", () => {
    test("选择频道时，向服务端查询并自动标记该频道未读通知为已读", async () => {
      const { selectChannel, selectedChannel } = useNotificationCenter();

      const mockUnreadNotif = mockNotification({
        id: "notif-unread",
        channel: "hooks",
        readAt: null,
      });
      mockQuery.mockResolvedValueOnce({
        data: {
          notifications: {
            edges: [{ node: mockUnreadNotif }],
          },
        },
      });

      await selectChannel("hooks");

      expect(selectedChannel.value).toBe("hooks");
      expect(mockMutate).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          variables: { input: expect.objectContaining({ id: "notif-unread" }) },
        }),
      );
    });
  });
  // #endregion
});
