<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <label class="block text-sm font-medium text-primary-300">
        选择目录
      </label>
      <template v-if="completedCount">
        <label class="flex items-center gap-2 cursor-pointer">
          <span class="text-sm text-primary-400"
            >显示已达标目录（{{ completedCount }}）</span
          >
          <div class="relative">
            <input
              v-model="showCompletedDirectories"
              type="checkbox"
              class="sr-only peer"
            />
            <div
              class="w-11 h-6 bg-primary-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-secondary-600"
            ></div>
          </div>
        </label>
      </template>
    </div>
    <div class="bg-primary-700 rounded-lg p-4">
      <div v-if="!currentDirectory?.root" class="mb-4">
        <button
          class="text-secondary-400 hover:text-secondary-300 text-sm flex items-center gap-1"
          @click="goToParent"
        >
          <svg
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
          返回上级
        </button>
      </div>

      <DirectoryItem
        v-if="currentDirectory"
        :key="currentDirectory.id"
        v-model="selectedId"
        class="mb-2"
        :directory="currentDirectory"
        :filter-rating="filterRating"
        :target-keep="targetKeep"
        :loading="backgroundLoadingCount > 0"
      />

      <div v-if="searchableItems.length > 5" class="mb-4">
        <input
          v-model="searchQuery"
          type="search"
          class="w-full bg-primary-800 text-primary-100 border border-primary-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-secondary-500 placeholder-primary-500 transition-colors"
          placeholder="搜索目录..."
        />
      </div>

      <div
        v-if="items.length > 0"
        class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
        @scroll="handleScroll"
      >
        <template v-for="item in visibleFilteredItems" :key="item.key">
          <DirectoryItem
            v-model="selectedId"
            :directory="item.dir"
            :filter-rating="filterRating"
            :target-keep="targetKeep"
            :loading="backgroundLoadingCount > 0"
          />
        </template>
      </div>

      <div v-else-if="loading" class="space-y-4">
        <div class="bg-primary-700 rounded-lg p-4">
          <div class="animate-pulse">
            <div class="h-4 bg-primary-600 rounded mb-2 w-3/4"></div>
            <div class="h-3 bg-primary-600 rounded w-1/2"></div>
          </div>
        </div>
        <div class="bg-primary-700 rounded-lg p-4">
          <div class="animate-pulse">
            <div class="h-4 bg-primary-600 rounded mb-2 w-3/4"></div>
            <div class="h-3 bg-primary-600 rounded w-1/2"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { sortBy } from "es-toolkit";
import DirectoryItem from "./DirectoryItem.vue";
import useStorage from "../composables/useStorage";
import useAsyncTask from "../composables/useAsyncTask";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import useDirectoryStats from "../composables/useDirectoryStats";
import ExactSearchMatcher from "../utils/ExactSearchMatcher";
import type { DirectoryFragment } from "../graphql/generated";

const { currentDirectory, directories, loading, filterRating, targetKeep } =
  defineProps<{
    currentDirectory: DirectoryFragment | null | undefined;
    directories: DirectoryFragment[];
    loading: boolean;
    filterRating: readonly number[];
    targetKeep: number;
    rootPath: string;
  }>();

const emit = defineEmits<(e: "go-to-parent") => void>();

const selectedId = defineModel<string>();

const { model: showCompletedDirectories } = useStorage<boolean>(
  localStorage,
  "showCompletedDirectories@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
  () => false,
);

const { recordDirectoryOrder } = useDirectoryProgress();

// 从缓存中获取统计信息
const { getCachedStats, refetchStats } = useDirectoryStats();

const backgroundLoadingCount = ref(0);

// 在后台批量加载未获取统计信息的目录，避免同时发起大量查询
useAsyncTask({
  loadingCount: backgroundLoadingCount,
  args() {
    const toLoad = directories.map((d) => d.id);
    return toLoad.length > 0 ? [toLoad] : undefined;
  },
  async task(toLoad, ctx) {
    await refetchStats(toLoad, ctx.signal());
  },
});

const items = computed(() => {
  return sortBy(
    directories.map((dir) => {
      const stats = getCachedStats(dir.id);
      const keepCount =
        stats?.ratingCounts.reduce(
          (sum: number, rc: { rating: number; count: number }) =>
            sum + (filterRating.includes(rc.rating) ? rc.count : 0),
          0,
        ) ?? 0;

      const isCompleted =
        stats?.subdirectoryCount === 0 && keepCount <= targetKeep;
      return {
        key: dir.id,
        dir,
        stats,
        isCompleted,
      };
    }),
    [
      (item) => {
        return !item.stats;
      },
      (item) => item.stats?.imageCount === 0,
      (item) => item.stats?.latestImage?.modTime || "",
    ],
  );
});

const searchableItems = computed(() => {
  return items.value.filter(
    (item) => showCompletedDirectories.value || !item.isCompleted,
  );
});

const searchState = ref({ query: "", directoryId: "" });

const searchQuery = computed({
  get: () =>
    searchState.value.directoryId === (currentDirectory?.id ?? "")
      ? searchState.value.query
      : "",
  set: (val: string) => {
    searchState.value = { query: val, directoryId: currentDirectory?.id ?? "" };
  },
});

const filteredItems = computed(() => {
  const matcher = new ExactSearchMatcher(searchQuery.value);
  return items.value.filter((item) => {
    const name = item.dir.path.split(/[\\/]/).pop() ?? "";
    return matcher.match(name);
  });
});

const displayedFilteredItems = computed(() => {
  return filteredItems.value.filter(
    (item) => showCompletedDirectories.value || !item.isCompleted,
  );
});

const renderLimit = ref(40);

const visibleFilteredItems = computed(() => {
  return displayedFilteredItems.value.slice(0, renderLimit.value);
});

watch(
  () => currentDirectory?.id,
  () => {
    renderLimit.value = 40;
  },
);

function handleScroll(e: Event) {
  const target = e.target as HTMLElement;
  if (target.scrollTop + target.clientHeight >= target.scrollHeight - 100) {
    if (renderLimit.value < displayedFilteredItems.value.length) {
      renderLimit.value += 40;
    }
  }
}

const completedCount = computed(() => {
  return items.value.reduce((sum, item) => sum + (item.isCompleted ? 1 : 0), 0);
});

watch(
  filteredItems,
  (newItems) => {
    const navigableDirectoryIds = newItems
      .filter((item) => {
        const keepCount =
          item.stats?.ratingCounts.reduce(
            (sum: number, rc: { rating: number; count: number }) =>
              sum + (filterRating.includes(rc.rating) ? rc.count : 0),
            0,
          ) ?? 0;
        return keepCount > targetKeep;
      })
      .map((item) => item.dir.id);

    if (currentDirectory) {
      recordDirectoryOrder(currentDirectory.id, navigableDirectoryIds);
    }
  },
  { immediate: true },
);

function goToParent() {
  emit("go-to-parent");
}
</script>
