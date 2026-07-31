<script setup lang="ts">
import { ref } from "vue";
import { mdiChevronLeft, mdiBellOutline, mdiOpenInNew } from "@mdi/js";
import useNotificationCenter from "@/composables/domain/useNotificationCenter";
import NotificationBodyDialog from "@/components/NotificationBodyDialog.vue";
import { formatDate } from "@/utils/date";

const {
  channels,
  unreadCount,
  selectedChannel,
  channelNotifications,
  selectedChannelUnreadCount,
  drawer,
  bodyDialog,
  bodyDialogTitle,
  bodyDialogBody,
  selectChannel,
} = useNotificationCenter();

// 跟踪已展开的通知 ID，支持点击正文或按钮切换展开/折叠
const expandedIds = ref(new Set<string>());

function toggleExpand(id: string) {
  const next = new Set(expandedIds.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  expandedIds.value = next;
}

function handleSelectChannel(channel: string) {
  void selectChannel(channel);
}

function handleBack() {
  selectedChannel.value = null;
}
</script>

<template>
  <drawer.component
    container-class="w-full max-w-md md:max-w-lg bg-primary-800 border-l border-primary-700 overflow-y-auto overflow-x-hidden shadow-2xl flex flex-col h-full text-left"
  >
    <!-- Header -->
    <div class="sticky top-0 z-10 bg-primary-800 border-b border-primary-700 p-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3">
          <!-- Back button in channel detail -->
          <button
            v-if="selectedChannel"
            class="p-1 rounded-lg hover:bg-primary-700 transition-colors text-primary-400 hover:text-white cursor-pointer"
            @click="handleBack"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiChevronLeft" fill="currentColor" />
            </svg>
          </button>
          <h2 class="text-lg font-bold text-primary-100">
            {{ selectedChannel ?? "通知中心" }}
          </h2>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-primary-500">
            {{ selectedChannel ? `${selectedChannelUnreadCount} 条未读` : `${unreadCount} 条未读` }}
          </span>
        </div>
      </div>
    </div>

    <!-- Channel List (default view) -->
    <div v-if="!selectedChannel" class="flex-1">
      <div
        v-if="channels.length === 0"
        class="flex flex-col items-center justify-center py-16 text-primary-500 text-sm gap-2"
      >
        <svg class="w-12 h-12 opacity-30" viewBox="0 0 24 24">
          <path :d="mdiBellOutline" fill="currentColor" />
        </svg>
        <span>暂无通知</span>
      </div>

      <button
        v-for="ch in channels"
        :key="ch.channel"
        class="w-full flex items-start gap-3 px-4 py-3 hover:bg-primary-700/50 transition-colors text-left border-b border-primary-700/30 cursor-pointer"
        @click="handleSelectChannel(ch.channel)"
      >
        <!-- Unread count badge on the left -->
        <span
          v-if="ch.unreadCount > 0"
          class="shrink-0 flex items-center justify-center h-5 min-w-5 rounded-full bg-secondary-600 px-2 text-xs font-bold text-white mt-0.5"
        >
          {{ ch.unreadCount }}
        </span>

        <!-- Content -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center justify-between gap-2">
            <span class="font-medium text-primary-100 text-sm truncate">
              {{ ch.channel }}
            </span>
          </div>
          <p class="text-xs text-primary-500 mt-1">
            {{ ch.latestNotification?.title ?? "暂无通知" }}
          </p>
        </div>
      </button>
    </div>

    <!-- Channel Detail -->
    <div v-else class="flex-1">
      <div
        v-if="channelNotifications.length === 0"
        class="flex flex-col items-center justify-center py-16 text-primary-500 text-sm"
      >
        暂无通知
      </div>

      <div
        v-for="notification in channelNotifications"
        :key="notification.id"
        class="flex items-start gap-3 px-4 py-3 hover:bg-primary-700/30 transition-colors border-b border-primary-700/30"
      >
        <!-- Priority indicator -->
        <div
          :class="[
            'mt-2 h-2 w-2 shrink-0 rounded-full',
            notification.priority === 'HIGH'
              ? 'bg-red-500'
              : notification.priority === 'NORMAL'
                ? 'bg-secondary-500'
                : 'bg-primary-500',
          ]"
        />

        <!-- Content -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center justify-between gap-2">
            <span class="text-sm font-medium text-primary-100 truncate">
              {{ notification.title }}
            </span>
            <span class="text-xs text-primary-500 shrink-0">{{
              formatDate(notification.createdAt)
            }}</span>
          </div>
          <!-- 通知正文：默认折叠两行，点击不影响选中 -->
          <p
            v-if="notification.body"
            class="text-xs text-primary-400 mt-1 select-text whitespace-pre-wrap"
            :class="{ 'line-clamp-2': !expandedIds.has(notification.id) }"
          >
            {{ notification.body }}
          </p>
          <button
            v-if="notification.body"
            class="text-xs text-secondary-400 hover:text-secondary-300 transition-colors cursor-pointer mt-1"
            @click="toggleExpand(notification.id)"
          >
            {{ expandedIds.has(notification.id) ? "收起" : "展开" }}
          </button>
          <div v-if="notification.detailsURL" class="mt-1">
            <a
              :href="notification.detailsURL"
              target="_blank"
              class="inline-flex items-center gap-1 text-xs text-secondary-400 hover:text-secondary-300 hover:underline transition-colors"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path :d="mdiOpenInNew" fill="currentColor" />
              </svg>
              查看详情
            </a>
          </div>
        </div>
      </div>
    </div>
  </drawer.component>

  <!-- 通知正文弹窗：展示 Hook 执行的 stderr 详情 -->
  <bodyDialog.component container-class="sm:max-w-lg">
    <NotificationBodyDialog
      :title="bodyDialogTitle"
      :body="bodyDialogBody"
      @close="bodyDialog.close()"
    />
  </bodyDialog.component>
</template>
