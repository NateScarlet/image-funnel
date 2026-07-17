import { ref, computed } from "vue";
import { once } from "es-toolkit";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import useModalDrawer from "@/composables/useModalDrawer";
import useNotification from "@/composables/useNotification";
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

type NotificationChannel = NotificationChannelsQuery["notificationChannels"][number];
type Notification = NotificationFragment;

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

  const { data: channelsData, refresh: refreshChannels } = useQuery(NotificationChannelsDocument, {
    fetchPolicy: "cache-and-network",
  });

  const channels = computed<NotificationChannel[]>(() => {
    return channelsData.value?.notificationChannels ?? [];
  });

  const unreadCount = computed(() => {
    return channels.value.reduce((sum, ch) => sum + ch.unreadCount, 0);
  });

  const selectedChannelUnreadCount = computed(() => {
    return channels.value.find((ch) => ch.channel === selectedChannel.value)?.unreadCount ?? 0;
  });

  // 检查通知是否在可显示时间窗口内（notBefore/notAfter 由前端处理）
  function isTimely(n: { notBefore?: string | null; notAfter?: string | null }): boolean {
    const now = Date.now();
    if (n.notBefore) {
      const t = new Date(n.notBefore).getTime();
      if (!isNaN(t) && now < t) return false;
    }
    if (n.notAfter) {
      const t = new Date(n.notAfter).getTime();
      if (!isNaN(t) && now > t) return false;
    }
    return true;
  }

  // 根据文本长度计算合理的展示时长
  function toastDuration(title: string): number {
    const len = title.length;
    if (len < 20) return 3000;
    if (len < 50) return 5000;
    return 7000;
  }

  // 投递 Toast；优先级仅影响是否可关闭（HIGH 需手动确认）
  function spawnToast(n: {
    id: string;
    title: string;
    priority: NotificationPriority;
    notBefore?: string | null;
    notAfter?: string | null;
  }) {
    if (!isTimely(n) || shownToastIds.has(n.id)) return;
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
      if (eventPayload?.event === NotificationEventType.SENT && eventPayload.notification) {
        spawnToast(eventPayload.notification);
      }
    },
  });

  // 应用加载时弹每个频道最新一条未读通知（不依赖 useQuery）
  void loadAndToastInitialUnreads();

  async function loadAndToastInitialUnreads() {
    const { data: chData } = await query(NotificationChannelsDocument, {
      fetchPolicy: "network-only",
    });
    const chs = chData?.notificationChannels ?? [];
    for (const ch of chs) {
      const n = ch.latestNotification;
      if (
        n &&
        !n.readAt &&
        n.status === NotificationStatus.ACTIVE &&
        n.priority !== NotificationPriority.LOW
      ) {
        spawnToast(n);
      }
    }
  }

  async function refreshSelectedChannel() {
    if (!selectedChannel.value) return;
    const { data } = await query(NotificationsDocument, {
      variables: { channel: selectedChannel.value },
      fetchPolicy: "network-only",
    });
    channelNotifications.value = data?.notifications?.nodes ?? [];
  }

  async function selectChannel(channel: string) {
    selectedChannel.value = channel;
    await refreshSelectedChannel();
    // 自动批量已读
    await markAllAsRead(channel);
  }

  async function markAsRead(id: string) {
    await mutate(UpdateNotificationDocument, {
      variables: { input: { id, readAt: new Date().toISOString() } },
    });
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
    channelNotifications,
    selectedChannelUnreadCount,
    drawer,
    selectChannel,
    markAsRead,
    markAllAsRead,
    markAsDismissed,
    refreshChannels,
  };
});

export default function useNotificationCenter() {
  return init();
}
