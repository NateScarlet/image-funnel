<template>
  <div>
    <div class="flex flex-wrap items-center justify-between gap-4 mb-4">
      <label class="block text-sm font-medium text-primary-300">
        选择目录
      </label>
      <div class="flex flex-wrap items-center gap-4">
        <!-- 筛选未评级图片目录开关 -->
        <ToggleSwitch v-model="showSmallUnrated">
          <span class="text-sm text-primary-400">
            显示未评级图片 &lt;
            <input
              v-model.number="minUnratedCount"
              type="number"
              class="w-12 bg-primary-800 text-primary-100 border border-primary-600 rounded px-2 py-0.5 text-xs focus:outline-none focus:border-secondary-500 mx-1"
              min="0"
              @click.stop
            />
            的目录（{{ smallUnratedCount }}）
          </span>
        </ToggleSwitch>
        <!-- 筛选已达标目录开关 -->
        <template v-if="completedCount">
          <ToggleSwitch
            v-model="showCompletedDirectories"
            :label="`显示已达标目录（${completedCount}）`"
          />
        </template>
      </div>
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
        ref="containerRef"
        class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
      >
        <template v-for="item in visibleFilteredItems" :key="item.key">
          <DirectoryItem
            v-model="selectedId"
            :directory="item.dir"
            :filter-rating="filterRating"
            :target-keep="targetKeep"
            :loading="backgroundLoadingCount > 0"
            :filtered-out="item.filteredOut"
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
import { computed, ref, watch, useTemplateRef } from "vue";
import { sortBy } from "es-toolkit";
import DirectoryItem from "./DirectoryItem.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import useStorage from "../composables/useStorage";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import useDirectoryStats from "../composables/useDirectoryStats";
import ExactSearchMatcher from "../utils/ExactSearchMatcher";
import useDirectories from "../composables/useDirectories";
import useInfiniteScroll from "../composables/useInfiniteScroll";

const { filterRating, targetKeep } = defineProps<{
  filterRating: readonly number[];
  targetKeep: number;
  rootPath: string;
}>();

const selectedId = defineModel<string>();

const { model: showCompletedDirectories } = useStorage<boolean>(
  localStorage,
  "showCompletedDirectories@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
  () => false,
);

const { model: minUnratedCount } = useStorage<number>(
  localStorage,
  "minUnratedCount@fab36720-bc2d-4876-8800-47b85f20658f",
  () => 1,
);

const { model: showSmallUnrated } = useStorage<boolean>(
  localStorage,
  "showSmallUnrated@5918e244-67ad-4971-8608-f404494c25f4",
  () => false,
);

const { recordDirectoryOrder } = useDirectoryProgress();

// 从缓存中获取统计信息
const { getCachedStats } = useDirectoryStats();

const backgroundLoadingCount = ref(0);

// 使用 useDirectories，自治地实现内部数据拉取与 Relay 分页
const {
  sortedDirectories: directories,
  currentDirectory,
  loading,
  hasNextPage,
  fetchMore,
} = useDirectories(
  () => ({
    id: selectedId.value || "",
  }),
  { loadingCount: backgroundLoadingCount },
);

const items = computed(() => {
  return sortBy(
    directories.value.map((dir) => {
      const stats = getCachedStats(dir.id);
      const keepCount =
        stats?.ratingCounts.reduce(
          (sum: number, rc: { rating: number; count: number }) =>
            sum + (filterRating.includes(rc.rating) ? rc.count : 0),
          0,
        ) ?? 0;

      const unratedCount =
        stats?.ratingCounts.find(
          (rc: { rating: number; count: number }) => rc.rating === 0,
        )?.count ?? 0;

      const isCompleted =
        stats?.subdirectoryCount === 0 && keepCount <= targetKeep;
      const isSmallUnrated =
        stats?.subdirectoryCount === 0 && unratedCount < minUnratedCount.value;

      return {
        key: dir.id,
        dir,
        stats,
        isCompleted,
        unratedCount,
        keepCount,
        isSmallUnrated,
      };
    }),
    [
      (item) => !item.stats,
      (item) => item.stats?.imageCount === 0,
      (item) => item.stats?.latestImage?.modTime || "",
    ],
  );
});

/** 判定目录是否应该根据当前筛选设置显示 */
const isVisible = (item: (typeof items.value)[number]) => {
  if (!showCompletedDirectories.value && item.isCompleted) {
    return false;
  }
  if (!showSmallUnrated.value && item.isSmallUnrated) {
    return false;
  }
  return true;
};

const searchableItems = computed(() => {
  return items.value.filter(isVisible);
});

const searchState = ref({ query: "", directoryId: "" });

const searchQuery = computed({
  get: () =>
    searchState.value.directoryId === (currentDirectory.value?.id ?? "")
      ? searchState.value.query
      : "",
  set: (val: string) => {
    searchState.value = {
      query: val,
      directoryId: currentDirectory.value?.id ?? "",
    };
  },
});

const filteredItems = computed(() => {
  const matcher = new ExactSearchMatcher(searchQuery.value);
  return items.value.filter((item) => {
    const name = item.dir.relPath.split(/[\\/]/).pop() ?? "";
    return matcher.match(name);
  });
});

const displayedFilteredItems = computed(() => {
  // 判断当前是否处于有效搜索状态下
  const isSearching = searchQuery.value.trim() !== "";
  return filteredItems.value
    .map((item) => ({
      ...item,
      // 计算每一项是否不满足当前的全局过滤筛选条件
      filteredOut: !isVisible(item),
    }))
    .filter((item) => {
      // 搜索时忽略筛选条件（即显示所有匹配搜索的项目）
      if (isSearching) {
        return true;
      }
      // 非搜索时，只显示归档/隐藏以外的符合筛选条件的项目
      return !item.filteredOut;
    });
});

const renderLimit = ref(40);

const visibleFilteredItems = computed(() => {
  return displayedFilteredItems.value.slice(0, renderLimit.value);
});

watch(
  () => currentDirectory.value?.id,
  () => {
    renderLimit.value = 40;
  },
);

const containerRef = useTemplateRef<HTMLElement>("containerRef");

useInfiniteScroll(containerRef, async () => {
  if (renderLimit.value < displayedFilteredItems.value.length) {
    renderLimit.value += 40;
  }
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});

const completedCount = computed(() => {
  return items.value.filter((item) => item.isCompleted).length;
});

const smallUnratedCount = computed(() => {
  return items.value.filter((item) => item.isSmallUnrated).length;
});

watch(
  filteredItems,
  (newItems) => {
    const navigableDirectoryIds = newItems
      .filter((item) => item.keepCount > targetKeep)
      .map((item) => item.dir.id);

    if (currentDirectory.value) {
      recordDirectoryOrder(currentDirectory.value.id, navigableDirectoryIds);
    }
  },
  { immediate: true },
);

function goToParent() {
  if (!currentDirectory.value || !currentDirectory.value.parentId) {
    selectedId.value = "";
    return;
  }
  selectedId.value = currentDirectory.value.parentId;
}
</script>
