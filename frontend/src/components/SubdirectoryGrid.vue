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
      <div class="flex flex-wrap items-center gap-3 sm:gap-4">
        <!-- 本地搜索子目录输入框 -->
        <div class="relative min-w-36 max-w-60 flex-1 sm:flex-none">
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索子目录..."
            class="w-full pl-8 pr-8 h-8 bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-xs text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all"
          />
          <svg
            class="w-4 h-4 text-primary-400 absolute left-2 top-1/2 -translate-y-1/2 pointer-events-none"
            viewBox="0 0 24 24"
          >
            <path :d="mdiMagnify" fill="currentColor" />
          </svg>
          <button
            v-if="searchQuery"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-primary-400 hover:text-primary-200 transition-colors p-0.5 rounded-full hover:bg-primary-700/50 cursor-pointer"
            title="清空"
            @click="searchQuery = ''"
          >
            <svg class="w-3 h-3" viewBox="0 0 24 24">
              <path :d="mdiClose" fill="currentColor" />
            </svg>
          </button>
        </div>

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
    <div class="max-h-[40vh] overflow-y-auto pr-1">
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
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { mdiFolder, mdiMagnify, mdiClose } from "@mdi/js";
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

// 搜索关键字响应式变量
const searchQuery = ref("");

// 使用新提取的 composable，共享子目录过滤与排序状态
const {
  maxUnratedCount,
  showLargeUnrated,
  largeUnratedCount,
  sortedDirectories,
  loading,
} = useFilteredDirectories(() => directories, searchQuery);

// #region 目录名解析
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
</script>
