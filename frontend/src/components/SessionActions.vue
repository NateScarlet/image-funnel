<template>
  <div>
    <button
      :disabled="marking"
      class="btn-action flex-1 p-2 bg-red-600 hover:bg-red-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', ImageAction.REJECT)"
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
      class="btn-action flex-1 p-2 bg-yellow-600 hover:bg-yellow-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', ImageAction.PENDING)"
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
      class="btn-action flex-1 p-2 bg-green-600 hover:bg-green-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap transition-colors"
      @click="$emit('mark', ImageAction.KEEP)"
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
import { ImageAction } from "@/graphql/generated";
import {
  mdiDeleteOutline,
  mdiClockOutline,
  mdiHeartOutline,
  mdiLoading,
} from "@mdi/js";

interface Props {
  marking: boolean;
}

type Emits = (e: "mark", action: ImageAction) => void;

defineProps<Props>();
defineEmits<Emits>();
</script>
