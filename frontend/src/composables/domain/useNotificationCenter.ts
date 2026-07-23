import { ref, computed, watch } from "vue";
import { once } from "es-toolkit";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import useModalDrawer from "@/composables/useModalDrawer";
import useModalDialog from "@/composables/useModalDialog";
import useNotification from "@/composables/useNotification";
import type { NotificationAction } from "@/composables/useNotification";
import useCurrentTime from "@/composables/useCurrentTime";
import {
  NotificationChannelsDocument,
  NotificationChangedDocument,
  NotificationsDocument,
  UpdateNotificationDocument,
  NotificationPriority,
  type NotificationFragment,
} from "@/graphql/generated";

type Notification = NotificationFragment;

// 根据文本长度计算合理的展示时长（标题 + 正文）
function toastDuration(title: string, body: string): number {
  const len = title.length + body.length;
  if (len < 20) return 3000;
  if (len < 50) return 5000;
  return 7000;
}

// 通过 WS transport 批量已读，Promise.all 并发发送避免逐个 await 等待
async function markAsRead(id: string) {
  await mutate(UpdateNotificationDocument, {
    variables: { input: { id, readAt: new Date().toISOString() } },
    context: { transport: "ws" },
  });
}

const init = once(() => {
  const selectedChannel = ref<string | null>(null);

  const { show: showToast, remove: removeToast } = useNotification();
  const shownToastIds = new Set<string>();
  // 追踪 notificationId -> toastId 映射，用于时间到达时关闭 toast
  const toastIdMap = new Map<string, number>();

  // 统一由 useModalDrawer 控制抽屉面板显隐，消除双向同步 watch 反设计
  const drawer = useModalDrawer({
    onDidClose() {
      selectedChannel.value = null;
    },
  });

  // 通知正文弹窗：展示 Hook 执行的 stderr 详情
  const bodyDialogTitle = ref("");
  const bodyDialogBody = ref("");
  const bodyDialog = useModalDialog();

  function openBodyDialog(title: string, body: string) {
    bodyDialogTitle.value = title;
    bodyDialogBody.value = body;
    void bodyDialog.open();
  }

  const { currentTime, refreshOn, isPast, isFuture } = useCurrentTime();

  // 需要时间感知的 Toast 通知（有 notBefore 或 notAfter 约束），Map 支持 O(1) 查找
  const scheduledNotifications = ref<Map<string, Notification>>(new Map());

  const { data: channelsData, refresh: refreshChannels } = useQuery(NotificationChannelsDocument, {
    fetchPolicy: "cache-and-network",
  });

  const { data: notificationsData, refresh: refreshNotifications } = useQuery(
    NotificationsDocument,
    {
      variables: computed(() => {
        if (!selectedChannel.value) return undefined;
        return { filterBy: { channel: [selectedChannel.value] } };
      }),
      fetchPolicy: "network-only",
    },
  );

  const channels = computed(() => {
    return channelsData.value?.notificationChannels?.nodes ?? [];
  });

  const channelNotifications = computed(
    () => notificationsData.value?.notifications?.edges?.map((e) => e.node) ?? [],
  );

  // 使用 render 模式：根据响应式 currentTime 筛选可见通知
  const visibleNotifications = computed(() => {
    return channelNotifications.value.filter((n) => {
      if (isFuture(n.notBefore)) return false;
      if (isPast(n.notAfter)) return false;
      return true;
    });
  });

  // refreshOn 监听所有通知的 notBefore/notAfter，在时间到达时自动刷新 currentTime
  refreshOn(() => {
    const cn = channelNotifications.value;
    const sn = scheduledNotifications.value.values();
    return {
      *[Symbol.iterator]() {
        for (const n of cn) {
          yield n.notBefore;
          yield n.notAfter;
        }
        for (const n of sn) {
          yield n.notBefore;
          yield n.notAfter;
        }
      },
    };
  });

  // 监听 currentTime 变化，重新评估 Toast 显示/隐藏
  watch(
    () => currentTime.value,
    () => {
      for (const n of scheduledNotifications.value.values()) {
        if (isFuture(n.notBefore) || isPast(n.notAfter)) {
          hideToast(n.id);
        } else {
          showToastImmediate(n);
        }
      }
    },
  );

  const unreadCount = computed(() => {
    return channels.value.reduce((sum, ch) => sum + ch.unreadCount, 0);
  });

  const selectedChannelUnreadCount = computed(() => {
    return channels.value.find((ch) => ch.channel === selectedChannel.value)?.unreadCount ?? 0;
  });

  // 立即显示 Toast
  function showToastImmediate(n: Notification) {
    if (shownToastIds.has(n.id)) return;
    shownToastIds.add(n.id);
    const duration = toastDuration(n.title, n.body);
    const actions = buildActions(n);
    const toastId =
      n.priority === NotificationPriority.HIGH
        ? showToast(n.title, "info", 0, actions, n.body)
        : showToast(n.title, "info", duration, actions.length > 0 ? actions : undefined, n.body);
    toastIdMap.set(n.id, toastId);
  }

  // 为通知构建操作按钮数组
  function buildActions(n: Notification): NotificationAction[] {
    const actions: NotificationAction[] = [];

    // body 较长（可能被截断）时显示"查看正文"按钮，打开对话框展示完整内容
    if (n.body.length > 150) {
      actions.push({
        text: "查看正文",
        onClick: (close) => {
          openBodyDialog(n.title, n.body);
          close();
        },
      });
    }

    // 如果有 detailsURL，显示"查看详情"按钮打开详情链接
    if (n.detailsURL) {
      const url = n.detailsURL;
      actions.push({
        text: "查看详情",
        onClick: (close) => {
          window.open(url, "_blank");
          close();
        },
      });
    }

    // HIGH 优先级通知不可自动关闭，提供关闭按钮
    if (n.priority === NotificationPriority.HIGH) {
      actions.push({
        text: "关闭",
        onClick: (close) => {
          void markAsDismissed(n.id);
          close();
        },
      });
    }

    return actions;
  }

  // 隐藏 Toast（notAfter 到达时调用）
  function hideToast(id: string) {
    const toastId = toastIdMap.get(id);
    if (toastId !== undefined) {
      removeToast(toastId);
      toastIdMap.delete(id);
    }
    shownToastIds.delete(id);
  }

  // 投递 Toast；所有通知都有时间约束（notBefore/notAfter 为 NonNull），支持时间感知
  function spawnToast(n: Notification) {
    // 已读或已关闭的通知不显示 toast，避免 markAllAsRead 等操作触发 NotificationChanged
    // 订阅回调时重新弹出已处理的 toast
    if (n.readAt) return;
    if (n.dismissedAt) return;

    // 加入调度列表，以便 watch 响应时间变化（notBefore 到达时显示，notAfter 到达时消失）
    const existing = scheduledNotifications.value.get(n.id);
    if (existing) {
      Object.assign(existing, n);
    } else {
      scheduledNotifications.value.set(n.id, n);
    }

    // 计算期望状态：应该在时间窗口内显示
    if (isFuture(n.notBefore)) return;
    if (isPast(n.notAfter)) return;

    // 幂等同步 UI：已经是期望状态则什么都不做
    if (shownToastIds.has(n.id)) return;

    showToastImmediate(n);
  }

  // 监听实时通知变更订阅，随时刷新会话列表，并呈现对应的 Toast
  useSubscription(NotificationChangedDocument, {
    onNext(res) {
      void refreshChannels();
      void refreshNotifications();

      const eventPayload = res.data?.notificationChanged;
      if (!eventPayload) return;

      const { notification } = eventPayload;
      if (!notification) return;

      // 统一处理：时间感知的渲染层自动处理所有状态变化
      // UNSENT 时服务端设置 notAfter 为当前时间，时间过滤自然隐藏
      spawnToast(notification);
    },
  });

  // 应用加载时弹每个频道最新一条未读通知（不依赖 useQuery）
  void loadAndToastInitialUnreads();
  // 恢复调度列表：页面刷新后内存状态清空，重新从服务端加载 notBefore 在未来的通知
  void restoreScheduledNotifications();

  async function loadAndToastInitialUnreads() {
    const { data: chData } = await query(NotificationChannelsDocument, {
      fetchPolicy: "network-only",
    });
    const chs = chData?.notificationChannels?.nodes ?? [];
    for (const ch of chs) {
      // unreadCount > 0 已确认有未读，latestNotification 是最新一条
      // status/readAt 由服务端派生，不在客户端重新实现
      if (ch.latestNotification && ch.unreadCount > 0) {
        const latestNotif = ch.latestNotification;
        if (latestNotif.priority !== NotificationPriority.LOW) {
          spawnToast(latestNotif);
        }
      }
    }
  }

  // 恢复调度列表：查询所有 notBefore 在未来的通知，加入调度列表以便时间到达时自动显示
  // 页面刷新后 scheduledNotifications 内存状态清空，此函数负责重建
  async function restoreScheduledNotifications() {
    const { data } = await query(NotificationsDocument, {
      // pendingAt: now 让服务端直接筛选 notBefore > now 的通知，无需客户端二次过滤
      variables: { filterBy: { pendingAt: new Date().toISOString() } },
      fetchPolicy: "network-only",
    });
    const notifs = data?.notifications?.edges?.map((e) => e.node) ?? [];
    for (const n of notifs) {
      // 服务端已过滤，所有结果都是 notBefore 在未来的，直接加入调度列表
      if (!scheduledNotifications.value.has(n.id)) {
        scheduledNotifications.value.set(n.id, n);
      }
    }
  }

  // #region 自动标记已读
  const markingReadIds = new Set<string>();

  // 监听选中频道及频道通知，有未读通知时自动批量标记已读
  watch(
    [selectedChannel, channelNotifications],
    async ([channel, notifs]) => {
      if (!channel || !notifs || notifs.length === 0) return;
      const unreads = notifs.filter((n) => !n.readAt && !markingReadIds.has(n.id));
      if (unreads.length === 0) return;

      for (const n of unreads) {
        markingReadIds.add(n.id);
      }
      try {
        await Promise.all(unreads.map((n) => markAsRead(n.id)));
        await refreshChannels();
      } finally {
        for (const n of unreads) {
          markingReadIds.delete(n.id);
        }
      }
    },
    { immediate: true },
  );

  async function selectChannel(channel: string) {
    selectedChannel.value = channel;
    await markAllAsRead(channel);
  }

  async function markAllAsRead(channel: string) {
    const currentUnreads = channelNotifications.value.filter((n) => !n.readAt);
    if (currentUnreads.length > 0) {
      await Promise.all(currentUnreads.map((n) => markAsRead(n.id)));
    }
    const { data } = await query(NotificationsDocument, {
      variables: { filterBy: { channel: [channel], read: false } },
      fetchPolicy: "network-only",
    });
    const unreads = data?.notifications?.edges?.map((e) => e.node) ?? [];
    const unreadsToMark = unreads.filter(
      (n) => !n.readAt && !currentUnreads.some((cu) => cu.id === n.id),
    );
    if (unreadsToMark.length > 0) {
      await Promise.all(unreadsToMark.map((n) => markAsRead(n.id)));
    }
    await refreshChannels();
  }
  // #endregion

  async function markAsDismissed(id: string) {
    await mutate(UpdateNotificationDocument, {
      variables: { input: { id, dismissedAt: new Date().toISOString() } },
    });
    await refreshNotifications();
  }

  return {
    channels,
    unreadCount,
    selectedChannel,
    channelNotifications: visibleNotifications,
    selectedChannelUnreadCount,
    drawer,
    bodyDialog,
    bodyDialogTitle,
    bodyDialogBody,
    selectChannel,
    markAsRead,
    markAllAsRead,
    markAsDismissed,
    refreshChannels,
    currentTime,
    // 导出供测试使用
    spawnToast,
    hideToast,
    scheduledNotifications,
  };
});

export default function useNotificationCenter() {
  return init();
}
