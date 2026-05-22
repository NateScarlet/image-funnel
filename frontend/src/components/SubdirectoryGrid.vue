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
          :loading="backgroundLoadingCount > 0"
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

<script lang="ts">
import useStorage from "@/composables/useStorage";

const { model: maxUnratedCount } = useStorage<number>(
  localStorage,
  "max_unrated_count_sub_dir_bf16419b",
  () => 0,
);

const { model: showLargeUnrated } = useStorage<boolean>(
  localStorage,
  "show_large_unrated_sub_dir_3dfc6a37",
  () => false,
);
</script>

<script setup lang="ts">
import { computed, ref } from "vue";
import { mdiFolder } from "@mdi/js";
import { sortBy } from "es-toolkit";
import DirectoryDisplay from "./DirectoryDisplay.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import useAsyncTask from "@/composables/useAsyncTask";
import useDirectoryStats from "@/composables/useDirectoryStats";
import type { DirectoryFragment } from "@/graphql/generated";

// #region 属性与事件定义
const { directories, filterRating } = defineProps<{
  directories: DirectoryFragment[];
  filterRating: readonly number[];
}>();

const emit = defineEmits<(e: "navigate", id: string) => void>();
// #endregion

// #region 目录名解析
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion

// #region 统计数据缓存与后台拉取
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
// #endregion

// #region 未评级图片过滤逻辑
const largeUnratedCount = computed(() => {
  return directories.filter((dir) => {
    const stats = getCachedStats(dir.id);
    if (!stats) return false;
    const unratedCount =
      stats.ratingCounts.find(
        (rc: { rating: number; count: number }) => rc.rating === 0,
      )?.count ?? 0;
    return (
      stats.subdirectoryCount === 0 && unratedCount > maxUnratedCount.value
    );
  }).length;
});
// #endregion

// #region 目录倒序排列
const sortedDirectories = computed(() => {
  const items = directories.map((dir) => {
    const stats = getCachedStats(dir.id);
    const unratedCount =
      stats?.ratingCounts.find(
        (rc: { rating: number; count: number }) => rc.rating === 0,
      )?.count ?? 0;
    const isLargeUnrated =
      stats?.subdirectoryCount === 0 && unratedCount > maxUnratedCount.value;
    return {
      dir,
      stats,
      isLargeUnrated,
    };
  });

  const filteredItems = items.filter((item) => {
    if (!showLargeUnrated.value && item.isLargeUnrated) {
      return false;
    }
    return true;
  });

  return sortBy(filteredItems, [
    // 无统计数据的排在最后，避免初始状态下的布局闪烁
    (item) => !item.stats,
    // 无图片数据的排在最后
    (item) => item.stats?.imageCount === 0,
    // 最新图片时间倒序（最新的排最前），通过取反时间戳实现
    (item) => {
      const modTime = item.stats?.latestImage?.modTime;
      return modTime ? -new Date(modTime).getTime() : 0;
    },
  ]).map((item) => item.dir);
});
// #endregion
</script>
