<template>
  <div class="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-md w-full">
    <TransitionGroup name="notification">
      <div
        v-for="notification in notifications"
        :key="notification.id"
        :class="[
          'p-4 rounded-lg shadow-lg flex items-start gap-3 cursor-pointer',
          typeClasses[notification.type],
        ]"
        @click="remove(notification.id)"
      >
        <div class="flex-shrink-0">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="w-6 h-6"
          >
            <path :d="iconPaths[notification.type]" fill="currentColor" />
          </svg>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium">{{ notification.message }}</div>
        </div>
        <button
          class="flex-shrink-0 opacity-60 hover:opacity-100 transition-opacity"
          @click.stop="remove(notification.id)"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="w-5 h-5"
          >
            <path :d="mdiClose" fill="currentColor" />
          </svg>
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import useNotification from "../composables/useNotification";
import {
  mdiClose,
  mdiAlertCircleOutline,
  mdiCheckCircleOutline,
  mdiAlertOutline,
  mdiInformationOutline,
} from "@mdi/js";

const { notifications, remove } = useNotification();

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
</script>

<style scoped>
.notification-enter-active,
.notification-leave-active {
  transition: all 0.3s ease;
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
