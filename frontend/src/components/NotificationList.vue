<template>
  <div class="fixed top-4 right-4 z-60 flex flex-col gap-2 max-w-md w-full">
    <div v-if="notifications.length > 1" class="flex justify-end items-center gap-2 px-2">
      <span
        v-if="hiddenCount > 0"
        class="text-xs text-white/80 bg-black/20 px-2 py-1 rounded-full backdrop-blur-sm"
      >
        +{{ hiddenCount }}
      </span>
      <button
        v-if="notifications.length > 1"
        class="text-xs text-white/80 hover:text-white flex items-center gap-1 transition-colors bg-black/20 hover:bg-black/40 px-3 py-1 rounded-full backdrop-blur-sm"
        @click="clear"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-4 h-4">
          <path :d="mdiBroom" fill="currentColor" />
        </svg>
        清除全部
      </button>
    </div>
    <TransitionGroup name="notification">
      <div
        v-for="notification in visibleNotifications"
        :key="notification.id"
        :class="[
          'p-4 rounded-lg shadow-lg flex items-start gap-3 cursor-pointer',
          typeClasses[notification.type],
        ]"
        @click="notification.persistent ? undefined : dismiss(notification)"
      >
        <div class="shrink-0">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-6 h-6">
            <path :d="iconPaths[notification.type]" fill="currentColor" />
          </svg>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium">{{ notification.message }}</div>
          <div
            v-if="notification.body"
            :ref="(el) => registerBodyEl(notification.id, el)"
            class="mt-1 text-xs opacity-90 break-words whitespace-pre-wrap max-h-24 overflow-hidden"
          >
            {{ notification.body }}
          </div>
          <button
            v-if="notification.body && overflowMap.get(notification.id)"
            class="text-xs text-secondary-400 hover:text-secondary-300 transition-colors cursor-pointer mt-1"
            @click.stop="openBodyDialog(notification)"
          >
            查看全文
          </button>
          <button
            v-if="notification.controller?.openDetails"
            class="px-2 py-1 bg-white/25 hover:bg-white/35 text-white font-semibold rounded text-xs transition-colors cursor-pointer mt-1"
            @click.stop="openDetails(notification)"
          >
            {{ notification.controller.openDetailsText ?? "查看详情" }}
          </button>
        </div>
        <button
          v-if="!notification.persistent"
          class="shrink-0 opacity-60 hover:opacity-100 transition-opacity"
          @click.stop="dismiss(notification)"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="w-5 h-5">
            <path :d="mdiClose" fill="currentColor" />
          </svg>
        </button>
      </div>
    </TransitionGroup>

    <!-- 通知正文全文弹窗 -->
    <bodyDialog.component container-class="sm:max-w-lg">
      <NotificationBodyDialog
        v-if="bodyDialogNotification"
        :title="bodyDialogNotification.title"
        :body="bodyDialogNotification.body"
        @close="bodyDialog.close()"
      />
    </bodyDialog.component>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive, nextTick, onUpdated } from "vue";
import useNotification from "../composables/useNotification";
import type { Notification } from "../composables/useNotification";
import useModalDialog from "../composables/useModalDialog";
import NotificationBodyDialog from "./NotificationBodyDialog.vue";
import {
  mdiClose,
  mdiAlertCircleOutline,
  mdiCheckCircleOutline,
  mdiAlertOutline,
  mdiInformationOutline,
  mdiBroom,
} from "@mdi/js";

const { notifications, remove, clear } = useNotification();
// 容器 z-60 必须高于模态框外壳的 z-50（useModal 默认值）：全屏查看器等模态打开时后挂载到 body，
// 同层级会按 DOM 顺序覆盖先挂载的 toast，导致复制等操作的反馈被查看器挡住
const MAX_NOTIFICATIONS = 5;

// #region 正文溢出检测与全文弹窗
const overflowMap = reactive(new Map<number, boolean>());
const bodyRefs = new Map<number, HTMLDivElement>();

function registerBodyEl(id: number, el: unknown) {
  if (el === null) {
    // 元素已从 DOM 中移除，清理状态
    bodyRefs.delete(id);
    overflowMap.delete(id);
    return;
  }
  if (!(el instanceof HTMLDivElement)) {
    console.error("NotificationList: 正文元素类型异常", el);
    return;
  }
  bodyRefs.set(id, el);
  nextTick(() => {
    overflowMap.set(id, el.scrollHeight > el.clientHeight);
  });
}

onUpdated(() => {
  for (const [id, el] of bodyRefs) {
    overflowMap.set(id, el.scrollHeight > el.clientHeight);
  }
});

const bodyDialogNotification = ref<{ id: number; title: string; body: string }>();

const bodyDialog = useModalDialog({
  onDidOpen() {
    document.body.style.overflow = "hidden";
  },
  onWillClose() {
    document.body.style.overflow = "";
  },
});

function openBodyDialog(notification: Notification) {
  bodyDialogNotification.value = {
    id: notification.id,
    title: notification.message,
    body: notification.body ?? "",
  };
  bodyDialog.open();
}
// #endregion

const visibleNotifications = computed(() => {
  return notifications.value.slice(-MAX_NOTIFICATIONS);
});

const hiddenCount = computed(() => {
  return Math.max(0, notifications.value.length - MAX_NOTIFICATIONS);
});

const iconPaths: Record<string, string> = {
  error: mdiAlertCircleOutline,
  success: mdiCheckCircleOutline,
  warning: mdiAlertOutline,
  info: mdiInformationOutline,
};

const typeClasses: Record<string, string> = {
  error: "bg-red-900/90 text-red-100 border border-red-700",
  success: "bg-green-900/90 text-green-100 border border-green-700",
  warning: "bg-yellow-900/90 text-yellow-100 border border-yellow-700",
  info: "bg-blue-900/90 text-blue-100 border border-blue-700",
};

// 用户主动关闭：先触发 controller 的业务操作（如持久化服务端关闭状态），再移除 toast
function dismiss(notification: Notification) {
  notification.controller?.dismiss?.();
  remove(notification.id);
}

function openDetails(notification: Notification) {
  notification.controller?.openDetails?.();
  remove(notification.id);
}
</script>

<style scoped>
.notification-enter-active,
.notification-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s ease;
}

.notification-enter-from {
  opacity: 0;
  transform: translateX(30px);
}

.notification-leave-to {
  opacity: 0;
  transform: translateX(30px);
}

.notification-move {
  transition: transform 0.3s ease;
}
</style>
