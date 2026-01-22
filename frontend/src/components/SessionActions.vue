<template>
  <div
    class="hidden [@media(min-height:768px)]:flex gap-4 w-full max-w-md mb-4"
  >
    <button
      :disabled="marking"
      class="btn-action flex-1 py-4 px-6 bg-red-600 hover:bg-red-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', 'REJECT')"
    >
      <svg v-if="marking" class="w-6 h-6 animate-spin" viewBox="0 0 24 24">
        <path :d="mdiLoading" fill="currentColor" />
      </svg>
      <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
        <path :d="mdiDeleteOutline" fill="currentColor" />
      </svg>
      <span>排除</span>
    </button>

    <button
      :disabled="marking"
      class="btn-action flex-1 py-4 px-6 bg-yellow-600 hover:bg-yellow-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', 'PENDING')"
    >
      <svg v-if="marking" class="w-6 h-6 animate-spin" viewBox="0 0 24 24">
        <path :d="mdiLoading" fill="currentColor" />
      </svg>
      <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
        <path :d="mdiClockOutline" fill="currentColor" />
      </svg>
      <span>稍后再看</span>
    </button>

    <button
      :disabled="marking"
      class="btn-action flex-1 py-4 px-6 bg-green-600 hover:bg-green-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', 'KEEP')"
    >
      <svg v-if="marking" class="w-6 h-6 animate-spin" viewBox="0 0 24 24">
        <path :d="mdiLoading" fill="currentColor" />
      </svg>
      <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
        <path :d="mdiHeartOutline" fill="currentColor" />
      </svg>
      <span>保留</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import {
  mdiDeleteOutline,
  mdiClockOutline,
  mdiHeartOutline,
  mdiLoading,
} from "@mdi/js";

interface Props {
  marking: boolean;
}

type Emits = (e: "mark", action: "REJECT" | "PENDING" | "KEEP") => void;

defineProps<Props>();
defineEmits<Emits>();
</script>
