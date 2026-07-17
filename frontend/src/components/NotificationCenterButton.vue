<script setup lang="ts">
import useNotificationCenter from "@/composables/domain/useNotificationCenter";

defineProps<{
  variant?: "default" | "menu-item";
}>();

const { unreadCount, drawer } = useNotificationCenter();
</script>

<template>
  <button
    :class="[
      variant === 'default'
        ? 'relative flex items-center gap-2 rounded-lg border border-primary-700 bg-primary-800/80 px-4 py-2 text-sm font-medium text-primary-300 transition-all hover:border-primary-600 hover:bg-primary-700 hover:text-white active:scale-95 cursor-pointer select-none'
        : 'relative w-full py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-all flex items-center gap-3 text-primary-200 hover:text-white active:scale-95 cursor-pointer select-none',
    ]"
    title="通知中心"
    @click="drawer.open()"
  >
    <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor">
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
      />
    </svg>

    <template v-if="variant === 'default'">
      <span class="hidden sm:inline">通知</span>
    </template>
    <template v-else>
      <span class="flex-1 text-left">通知中心</span>
    </template>

    <span
      v-if="unreadCount > 0"
      class="flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1 text-xs font-bold text-white shadow-lg"
    >
      {{ unreadCount > 99 ? "99+" : unreadCount }}
    </span>
  </button>
</template>
