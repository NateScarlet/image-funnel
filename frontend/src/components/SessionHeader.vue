<template>
  <header
    class="bg-slate-800 border-b border-slate-700 p-2 md:p-4 flex-shrink-0"
  >
    <div class="max-w-7xl mx-auto flex items-center justify-between">
      <div class="flex-1 min-w-0 mr-4">
        <div class="text-xs md:text-sm text-slate-400 truncate">
          {{ session?.directory || "加载中..." }}
        </div>
        <div class="text-sm md:text-lg font-semibold truncate">
          {{ session?.currentIndex || 0 }} / {{ session?.currentSize || 0 }}
          <span class="text-green-400 ml-2"
            >保留: {{ stats?.kept || 0 }} / {{ session?.targetKeep || 0 }}</span
          >
        </div>
      </div>

      <button
        class="md:hidden p-2 rounded-lg hover:bg-slate-700 transition-colors"
        @click="$emit('showMenu')"
      >
        <svg
          class="w-6 h-6"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path :d="mdiMenu" fill="currentColor" />
        </svg>
      </button>

      <div class="hidden md:flex items-center gap-4">
        <button
          :disabled="!session?.canUndo || undoing"
          class="px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('undo')"
        >
          <svg v-if="undoing" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
            <path :d="mdiLoading" fill="currentColor" />
          </svg>
          <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiUndo" fill="currentColor" />
          </svg>
          <span>撤销</span>
        </button>

        <button
          class="px-4 py-2 bg-slate-700 hover:bg-slate-600 rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('showUpdateSessionModal')"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiCogOutline" fill="currentColor" />
          </svg>
          修改预设
        </button>

        <button
          :disabled="!session?.canCommit"
          class="px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('showCommitModal')"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiCheck" fill="currentColor" />
          </svg>
          提交
        </button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { mdiMenu, mdiUndo, mdiCheck, mdiLoading, mdiCogOutline } from "@mdi/js";

interface SessionStats {
  kept: number;
}

interface Session {
  directory?: string;
  currentIndex?: number;
  currentSize?: number;
  targetKeep?: number;
  canUndo?: boolean;
  canCommit?: boolean;
}

interface Props {
  session?: Session | null;
  stats?: SessionStats;
  undoing: boolean;
}

defineProps<Props>();
defineEmits<{
  showMenu: [];
  undo: [];
  showUpdateSessionModal: [];
  showCommitModal: [];
}>();
</script>
