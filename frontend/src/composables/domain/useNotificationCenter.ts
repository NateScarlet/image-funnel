import { ref, computed } from "vue";
import { once } from "es-toolkit";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import {
  NotificationChannelsDocument,
  NotificationChangedDocument,
  NotificationsDocument,
  UpdateNotificationDocument,
  type NotificationChannelsQuery,
  type NotificationFragment,
} from "@/graphql/generated";

type NotificationChannel = NotificationChannelsQuery["notificationChannels"][number];
type Notification = NotificationFragment;

const init = once(() => {
  const panelOpen = ref(false);
  const selectedChannel = ref<string | null>(null);
  const channelNotifications = ref<Notification[]>([]);

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

  // 监听实时通知变更订阅，随时刷新会话列表
  useSubscription(NotificationChangedDocument, {
    onNext() {
      void refreshChannels();
      // 如果当前正在查看某个频道详情，顺便刷新详情列表
      if (selectedChannel.value) {
        void refreshSelectedChannel();
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

  function open() {
    panelOpen.value = true;
  }

  function close() {
    panelOpen.value = false;
    selectedChannel.value = null;
    channelNotifications.value = [];
  }

  async function selectChannel(channel: string) {
    selectedChannel.value = channel;
    await refreshSelectedChannel();
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
    // 批量读完后本地主动刷新一下
    await refreshSelectedChannel();
  }

  async function markAsDismissed(id: string) {
    await mutate(UpdateNotificationDocument, {
      variables: { id, input: { dismissedAt: new Date().toISOString() } },
    });
    // 本地主动刷新该条通知
    await refreshSelectedChannel();
  }

  return {
    channels,
    unreadCount,
    selectedChannel,
    channelNotifications,
    selectedChannelUnreadCount,
    panelOpen,
    open,
    close,
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
