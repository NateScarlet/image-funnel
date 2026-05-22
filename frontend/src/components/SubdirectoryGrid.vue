<template>
  <section class="space-y-2">
    <h2
      class="text-xs font-bold text-primary-400 tracking-wider uppercase select-none"
    >
      子目录
    </h2>
    <div
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
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { sortBy } from "es-toolkit";
import DirectoryDisplay from "./DirectoryDisplay.vue";
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

// #region 目录倒序排列
const sortedDirectories = computed(() => {
  return sortBy(
    directories.map((dir) => {
      const stats = getCachedStats(dir.id);
      return {
        dir,
        stats,
      };
    }),
    [
      // 无统计数据的排在最后，避免初始状态下的布局闪烁
      (item) => !item.stats,
      // 无图片数据的排在最后
      (item) => item.stats?.imageCount === 0,
      // 最新图片时间倒序（最新的排最前），通过取反时间戳实现
      (item) => {
        const modTime = item.stats?.latestImage?.modTime;
        return modTime ? -new Date(modTime).getTime() : 0;
      },
    ],
  ).map((item) => item.dir);
});
// #endregion
</script>
