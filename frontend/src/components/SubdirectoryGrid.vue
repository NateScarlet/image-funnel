<template>
  <section
    class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
  >
    <div class="flex items-center justify-between border-b border-primary-700/50 pb-3">
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
            name="search"
            autocomplete="off"
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
            <NumberInput
              v-model="maxUnratedCount"
              :min="0"
              :step="1"
              class="w-12 bg-primary-800 text-primary-100 border border-primary-600 rounded px-2 py-0.5 text-xs focus:outline-none focus:border-secondary-500 mx-1"
              @click.stop
            />
            的目录（{{ largeUnratedCount }}）
          </span>
        </ToggleSwitch>
      </div>
    </div>
    <div ref="containerRef" class="max-h-[40vh] overflow-y-auto pr-1">
      <div
        v-if="processedSubdirectories.length > 0"
        class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4 p-4"
      >
        <RouterLink
          v-for="{ dir: subDir, isFilteredOutButShown } in processedSubdirectories"
          :key="subDir.id"
          :to="{
            path: '/browse',
            query: { dir: subDir.id },
          }"
          class="p-4 bg-primary-800/40 hover:bg-primary-800/80 rounded-xl transition-all text-left group overflow-hidden block w-full hover:scale-105 hover:shadow-lg hover:shadow-black/20 no-underline text-primary-100 hover:text-white"
          :class="[
            isFilteredOutButShown
              ? 'border-2 border-dashed border-yellow-600 hover:border-yellow-500'
              : 'border border-primary-800 hover:border-secondary-500/50',
          ]"
        >
          <DirectoryDisplay
            :directory="{ id: subDir.id }"
            :filter-rating="filterRating"
            :loading="loading"
          >
            <template #title>
              <span class="text-sm font-semibold text-primary-200 group-hover:text-white truncate">
                {{ getDirName(subDir.relPath) }}
              </span>
            </template>
          </DirectoryDisplay>
        </RouterLink>
      </div>

      <!-- 搜索无结果或加载中的提示 -->
      <div
        v-else
        class="py-8 flex flex-col items-center justify-center text-primary-400 gap-2 select-none"
      >
        <template v-if="loading">
          <!-- 正在加载动画 -->
          <svg class="w-8 h-8 animate-spin text-secondary-500" viewBox="0 0 24 24" fill="none">
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
          <span class="text-sm">正在加载子目录...</span>
        </template>
        <template v-else>
          <svg class="w-8 h-8 text-primary-500" viewBox="0 0 24 24">
            <path :d="mdiFolder" fill="currentColor" />
          </svg>
          <span class="text-sm">
            {{ searchQuery.trim() ? "未找到匹配的子目录" : "无子目录" }}
          </span>
        </template>
      </div>

      <!-- 加载更多分页控制 -->
      <div v-if="hasNextPage" class="mt-4 flex justify-center border-t border-primary-700/30 pt-3">
        <button
          :disabled="loading"
          class="px-4 py-1.5 bg-primary-800 hover:bg-primary-700 rounded-lg text-xs text-primary-300 hover:text-white border border-primary-700 transition-colors cursor-pointer flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
          @click="fetchMore"
        >
          <!-- 加载中动画 -->
          <svg
            v-if="loading"
            class="w-3.5 h-3.5 animate-spin text-secondary-500"
            viewBox="0 0 24 24"
            fill="none"
          >
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
          <span>{{ loading ? "正在加载..." : "加载更多子目录" }}</span>
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from "vue";
import { mdiFolder, mdiMagnify, mdiClose, mdiLoading } from "@mdi/js";
import { sortBy } from "es-toolkit";
import DirectoryDisplay from "./DirectoryDisplay.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import NumberInput from "./NumberInput.vue";
import useDirectories, { maxUnratedCount, showLargeUnrated } from "@/composables/useDirectories";
import useInfiniteScroll from "@/composables/useInfiniteScroll";
import { useDirectoryStats } from "@/composables/domain/useDirectoryBrowse";

// #region 属性与事件定义
const { directoryId, filterRating } = defineProps<{
  directoryId: string;
  filterRating: readonly number[];
}>();

// 搜索关键字缓冲区，保存搜索词与对应的目录 ID，实现声明式目录切换与状态保留
const searchQueryBuffer = ref({ id: directoryId, query: "" });
const searchQuery = computed({
  get() {
    return searchQueryBuffer.value.id === directoryId ? searchQueryBuffer.value.query : "";
  },
  set(val) {
    searchQueryBuffer.value = {
      id: directoryId,
      query: val,
    };
  },
});

const subdirectoryLoadingCount = ref(0);

// 使用 useDirectories，共享子目录过滤与排序状态，实现内部数据拉取与 Relay 分页
const { largeUnratedCount, sortedDirectories, hasNextPage, fetchMore } = useDirectories(
  () => ({
    id: directoryId,
    filterBy: {
      query: searchQuery.value || undefined,
    },
  }),
  {
    loadingCount: subdirectoryLoadingCount,
    maxUnratedCount: maxUnratedCount,
    showLargeUnrated,
  },
);

const loading = computed(() => subdirectoryLoadingCount.value > 0);

const { getCachedStats } = useDirectoryStats();

const processedSubdirectories = computed(() => {
  const dirs = sortedDirectories.value;
  const limit = maxUnratedCount.value;
  const showLarge = showLargeUnrated.value;

  const items = dirs.map((dir) => {
    const stats = getCachedStats(dir.id);
    const unratedCount =
      stats?.ratingCounts.find((rc: { rating: number; count: number }) => rc.rating === 0)?.count ??
      0;

    // 当有未评级限制且它包含子目录时，如果它自身的未评级图片数量 > limit，
    // 说明它本来该被过滤掉，但因为有子目录而被保留显示。
    const isFilteredOutButShown =
      !showLarge &&
      limit !== undefined &&
      stats &&
      stats.subdirectoryCount > 0 &&
      unratedCount > limit;

    return {
      dir,
      isFilteredOutButShown,
    };
  });

  // 把 isFilteredOutButShown 的排在最后
  return sortBy(items, [(item) => (item.isFilteredOutButShown ? 1 : 0)]);
});

const containerRef = useTemplateRef<HTMLElement>("containerRef");

useInfiniteScroll(containerRef, async () => {
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});

// #region 目录名解析
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
</script>
