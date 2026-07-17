import { ref, computed } from "vue";
import { once } from "es-toolkit";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import useModalDrawer from "@/composables/useModalDrawer";
import useNotification from "@/composables/useNotification";
import useCurrentTime from "@/composables/useCurrentTime";
import {
  NotificationChannelsDocument,
  NotificationChangedDocument,
  NotificationsDocument,
  UpdateNotificationDocument,
  NotificationEventType,
  NotificationPriority,
  NotificationStatus,
  type NotificationChannelsQuery,
  type NotificationFragment,
} from "@/graphql/generated";

type NotificationChannel = NonNullable<
  NotificationChannelsQuery["notificationChannels"]
>["nodes"][number];
type Notification = NotificationFragment;

// 根据文本长度计算合理的展示时长
function toastDuration(title: string): number {
  const len = title.length;
  if (len < 20) return 3000;
  if (len < 50) return 5000;
  return 7000;
}

// 标记单条通知已读
async function markAsRead(id: string) {
  await mutate(UpdateNotificationDocument, {
    variables: { input: { id, readAt: new Date().toISOString() } },
  });
}

const init = once(() => {
  const selectedChannel = ref<string | null>(null);
  const channelNotifications = ref<Notification[]>([]);

  const { show: showToast } = useNotification();
  const shownToastIds = new Set<string>();

  // 统一由 useModalDrawer 控制抽屉面板显隐，消除双向同步 watch 反设计
  const drawer = useModalDrawer({
    onDidClose() {
      selectedChannel.value = null;
      channelNotifications.value = [];
    },
  });

  const { currentTime, refreshOn, isPast, isFuture } = useCurrentTime();

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
    const times: string[] = [];
    for (const n of channelNotifications.value) {
      times.push(n.notBefore);
      times.push(n.notAfter);
    }
    return times;
  });

  const { data: channelsData, refresh: refreshChannels } = useQuery(NotificationChannelsDocument, {
    fetchPolicy: "cache-and-network",
  });

  const channels = computed<NotificationChannel[]>(() => {
    return channelsData.value?.notificationChannels?.nodes ?? [];
  });

  const unreadCount = computed(() => {
    return channels.value.reduce((sum, ch) => sum + ch.unreadCount, 0);
  });

  const selectedChannelUnreadCount = computed(() => {
    return channels.value.find((ch) => ch.channel === selectedChannel.value)?.unreadCount ?? 0;
  });

  // 投递 Toast；优先级仅影响是否可关闭（HIGH 需手动确认）
  function spawnToast(n: {
    id: string;
    title: string;
    priority: NotificationPriority;
    notBefore?: string | null;
    notAfter?: string | null;
  }) {
    if (shownToastIds.has(n.id)) return;
    // 时间检查：不在窗口内则跳过
    if (isFuture(n.notBefore)) return;
    if (isPast(n.notAfter)) return;
    shownToastIds.add(n.id);
    const duration = toastDuration(n.title);
    if (n.priority === NotificationPriority.HIGH) {
      showToast(n.title, "info", 0, {
        text: "关闭",
        onClick: (close) => {
          void markAsDismissed(n.id);
          close();
        },
      });
    } else {
      showToast(n.title, "info", duration);
    }
  }

  // 监听实时通知变更订阅，随时刷新会话列表，并呈现对应的 Toast
  useSubscription(NotificationChangedDocument, {
    onNext(res) {
      void refreshChannels();
      if (selectedChannel.value) {
        void refreshSelectedChannel();
      }

      const eventPayload = res.data?.notificationChanged;
      if (!eventPayload) return;

      const { event, notification } = eventPayload;
      if (!notification) return;

      if (event === NotificationEventType.SENT) {
        // 新通知到达，投递 Toast
        spawnToast(notification);
      } else if (event === NotificationEventType.UNSENT) {
        // 通知被撤回，从本地列表移除
        channelNotifications.value = channelNotifications.value.filter(
          (n) => n.id !== notification.id,
        );
        shownToastIds.delete(notification.id);
      } else if (event === NotificationEventType.UPDATED) {
        // 通知更新（可见时间变化等），刷新本地列表
        if (notification.notBefore || notification.notAfter) {
          // 时间窗口变化，清除已显示的 Toast 记录以便重新评估
          shownToastIds.delete(notification.id);
        }
      }
    },
  });

  // 应用加载时弹每个频道最新一条未读通知（不依赖 useQuery）
  void loadAndToastInitialUnreads();

  async function loadAndToastInitialUnreads() {
    const { data: chData } = await query(NotificationChannelsDocument, {
      fetchPolicy: "network-only",
    });
    const chs = chData?.notificationChannels?.nodes ?? [];
    for (const ch of chs) {
      if (ch.latestNotification && ch.unreadCount > 0) {
        // 直接使用频道中的最新通知
        const latestNotif = ch.latestNotification;
        if (
          latestNotif &&
          !latestNotif.readAt &&
          latestNotif.status === NotificationStatus.ACTIVE &&
          latestNotif.priority !== NotificationPriority.LOW
        ) {
          spawnToast(latestNotif);
        }
      }
    }
  }

  async function refreshSelectedChannel() {
    if (!selectedChannel.value) return;
    const { data } = await query(NotificationsDocument, {
      variables: { filterBy: { channel: [selectedChannel.value] } },
      fetchPolicy: "network-only",
    });
    channelNotifications.value = data?.notifications?.edges?.map((e) => e.node) ?? [];
  }

  async function selectChannel(channel: string) {
    selectedChannel.value = channel;
    await refreshSelectedChannel();
    // 自动批量已读
    await markAllAsRead(channel);
  }

  async function markAllAsRead(_channel: string) {
    const unreads = channelNotifications.value.filter((n) => !n.readAt);
    if (unreads.length > 0) {
      // Promise.all 并发调用，极大提升网络效率，避免连续同步阻塞
      await Promise.all(unreads.map((n) => markAsRead(n.id)));
    }
    await refreshSelectedChannel();
  }

  async function markAsDismissed(id: string) {
    await mutate(UpdateNotificationDocument, {
      variables: { input: { id, dismissedAt: new Date().toISOString() } },
    });
    await refreshSelectedChannel();
  }

  return {
    channels,
    unreadCount,
    selectedChannel,
    channelNotifications: visibleNotifications,
    selectedChannelUnreadCount,
    drawer,
    selectChannel,
    markAsRead,
    markAllAsRead,
    markAsDismissed,
    refreshChannels,
    currentTime,
  };
});

export default function useNotificationCenter() {
  return init();
}
