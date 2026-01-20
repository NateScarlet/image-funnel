<template>
  <div class="max-w-md w-full mx-auto">
    <div class="text-center mb-8">
      <div class="text-6xl mb-4">🎉</div>
      <h2 class="text-3xl font-bold mb-2 text-white">筛选完成！</h2>
      <p class="text-slate-400">已处理目录中的所有图片</p>
    </div>

    <div
      class="bg-slate-800/50 rounded-2xl p-6 border border-slate-700/50 shadow-xl backdrop-blur-sm"
    >
      <CommitForm
        :session-id="sessionId"
        :stats="stats"
        title=""
        @committed="$emit('committed')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import CommitForm from "./CommitForm.vue";

interface SessionStats {
  kept: number;
  reviewed: number;
  rejected: number;
}

interface Props {
  sessionId: string;
  stats?: SessionStats;
}

type Emits = (e: "committed") => void;

defineProps<Props>();
defineEmits<Emits>();
</script>
