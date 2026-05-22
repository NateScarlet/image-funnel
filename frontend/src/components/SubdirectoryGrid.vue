<template>
  <section
    class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
  >
    <div
      class="flex items-center justify-between border-b border-primary-700/50 pb-3"
    >
      <h2
        class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
      >
        <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
          <path :d="mdiFolder" fill="currentColor" />
        </svg>
        子目录
      </h2>
      <div class="flex items-center gap-4">
        <!-- 筛选未评级图片目录开关 -->
        <ToggleSwitch v-model="showLargeUnrated">
          <span class="text-sm text-primary-400">
            显示未评级图片 &gt;
            <input
              v-model.number="maxUnratedCount"
              type="number"
              class="w-12 bg-primary-800 text-primary-100 border border-primary-600 rounded px-2 py-0.5 text-xs focus:outline-none focus:border-secondary-500 mx-1"
              min="0"
              @click.stop
            />
            的目录（{{ largeUnratedCount }}）
          </span>
        </ToggleSwitch>
      </div>
    </div>
    <div
      v-if="sortedDirectories.length > 0"
      class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4"
    >
      <button
        v-for="subDir in sortedDirectories"
        :key="subDir.id"
        class="p-4 bg-primary-800/40 hover:bg-primary-800/80 border border-primary-800 hover:border-secondary-500/50 rounded-xl transition-all text-left group overflow-hidden block w-full hover:scale-[1.02] hover:shadow-lg hover:shadow-black/20"
        @click="emit('navigate', subDir.id)"
      >
        <DirectoryDisplay
          :directory="{ id: subDir.id }"
          :filter-rating="filterRating"
          :loading="loading"
        >
          <template #title>
            <span
              class="text-sm font-semibold text-primary-200 group-hover:text-white truncate"
            >
              {{ getDirName(subDir.relPath) }}
            </span>
          </template>
        </DirectoryDisplay>
      </button>
    </div>
    <div v-else class="py-6 text-center text-primary-500 text-sm italic">
      无符合筛选条件的子目录
    </div>
  </section>
</template>

<script setup lang="ts">
import { mdiFolder } from "@mdi/js";
import DirectoryDisplay from "./DirectoryDisplay.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import type { DirectoryFragment } from "@/graphql/generated";
import useFilteredDirectories from "@/composables/useFilteredDirectories";

// #region 属性与事件定义
const { directories, filterRating } = defineProps<{
  directories: DirectoryFragment[];
  filterRating: readonly number[];
}>();

const emit = defineEmits<(e: "navigate", id: string) => void>();
// #endregion

// 使用新提取的 composable，共享子目录过滤与排序状态
const {
  maxUnratedCount,
  showLargeUnrated,
  largeUnratedCount,
  sortedDirectories,
  loading,
} = useFilteredDirectories(() => directories);

// #region 目录名解析
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
</script>
