<script setup lang="ts">
import { useDevices } from "@/composables/useDevices";

withDefaults(
  defineProps<{
    variant?: "default" | "menu-item";
  }>(),
  {
    variant: "default",
  },
);

const { devices, pairingRequests, open } = useDevices();
</script>

<template>
  <button
    :class="[
      variant === 'default'
        ? 'relative flex items-center gap-2 rounded-lg border border-primary-700 bg-primary-800/80 px-4 py-2 text-sm font-medium text-primary-300 transition-all hover:border-primary-600 hover:bg-primary-700 hover:text-white active:scale-95 cursor-pointer select-none'
        : 'relative w-full py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-all flex items-center gap-3 text-primary-200 hover:text-white active:scale-95 cursor-pointer select-none',
    ]"
    :title="variant === 'default' ? '管理已配对设备' : undefined"
    @click="open"
  >
    <!-- 手机/设备图标 -->
    <svg
      :class="variant === 'default' ? 'h-4 w-4 shrink-0' : 'h-5 w-5 shrink-0'"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z"
      />
    </svg>

    <!-- 内容展示 -->
    <template v-if="variant === 'default'">
      <span class="hidden sm:inline">{{ devices.length }} 台设备</span>
      <span class="sm:hidden">{{ devices.length }} 台</span>
    </template>
    <template v-else>
      <span class="flex-1 text-left">设备管理 ({{ devices.length }})</span>
    </template>

    <!-- 待配对提示红点 -->
    <span
      v-if="pairingRequests.length > 0"
      :class="[
        variant === 'default'
          ? 'absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-[10px] font-bold text-white shadow-lg animate-pulse'
          : 'flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-xs font-bold text-white shadow-lg animate-pulse',
      ]"
    >
      {{ pairingRequests.length }}
    </span>
  </button>
</template>
