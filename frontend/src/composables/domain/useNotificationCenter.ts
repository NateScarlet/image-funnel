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
  type NotificationChannelsQuery,
  type NotificationFragment,
} from "@/graphql/generated";

type NotificationChannel = NotificationChannelsQuery["notificationChannels"][number];
type Notification = NotificationFragment;

const init = once(() => {
  const selectedChannel = ref<string | null>(null);
  const channelNotifications = ref<Notification[]>([]);

  const { show: showToast } = useNotification();

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

  // 监听实时通知变更订阅，随时刷新会话列表，并呈现对应的 Toast
  useSubscription(NotificationChangedDocument, {
    onNext(res) {
      void refreshChannels();
      if (selectedChannel.value) {
        void refreshSelectedChannel();
      }

      // 实现 Priority 投递 Toast 提示逻辑
      const eventPayload = res.data?.notificationChanged;
      if (eventPayload?.event === NotificationEventType.SENT && eventPayload.notification) {
        const n = eventPayload.notification;
        // HIGH = 手动关闭 (duration = 0)
        // NORMAL = 自动消失 (duration = 5000ms)
        // LOW = 静默不弹
        if (n.priority === NotificationPriority.HIGH) {
          showToast(n.title, "info", 0);
        } else if (n.priority === NotificationPriority.NORMAL) {
          showToast(n.title, "info", 5000);
        }
      }
    },
  });

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
      variables: { id, input: { readAt: new Date().toISOString() } },
    });
  }

  async function markAllAsRead(_channel: string) {
    for (const n of channelNotifications.value) {
      if (!n.readAt) {
        await markAsRead(n.id);
      }
    }
    await refreshSelectedChannel();
  }

  async function markAsDismissed(id: string) {
    await mutate(UpdateNotificationDocument, {
      variables: { id, input: { dismissedAt: new Date().toISOString() } },
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
