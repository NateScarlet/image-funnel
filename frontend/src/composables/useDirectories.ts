import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useDirectoryBrowse, { useDirectoryStats } from "./domain/useDirectoryBrowse";
import useAsyncTask from "@/composables/useAsyncTask";
import useStorage from "@/composables/useStorage";
import { sortBy } from "es-toolkit";
import type { BrowseDirectoriesQueryVariables } from "@/graphql/generated";

// #region 筛选开关全局响应式状态
export const { model: maxUnratedCount } = useStorage<number>(
  localStorage,
  "max_unrated_count_sub_dir_bf16419b",
  () => 0,
);

export const { model: showLargeUnrated } = useStorage<boolean>(
  localStorage,
  "show_large_unrated_sub_dir_3dfc6a37",
  () => false,
);

export const { model: showUncompletedDirectories } = useStorage<boolean>(
  localStorage,
  "show_uncompleted_directories_sub_dir_a72c49b1",
  () => false,
);
// #endregion

export default function useDirectories(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables>,
  options?: {
    loadingCount?: Ref<number>;
    maxUnratedCount?: MaybeRefOrGetter<number | undefined>;
    showLargeUnrated?: MaybeRefOrGetter<boolean | undefined>;
  },
) {
  const maxUnratedCountVal = computed(() => toValue(options?.maxUnratedCount));

  const { currentDirectory, liveDirectories, hasNextPage, fetchMore } = useDirectoryBrowse(
    variables,
    { loadingCount: options?.loadingCount },
  );
  const { getCachedStats, refetchStats } = useDirectoryStats();

  const largeUnratedCount = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;
    if (limit === undefined) return 0;
    return dirs.filter((dir) => {
      const stats = getCachedStats(dir.id);
      if (!stats) return false;
      const unratedCount = stats.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      return stats.subdirectoryCount === 0 && unratedCount > limit;
    }).length;
  });

  useAsyncTask({
    args() {
      const dirs = liveDirectories.value;
      const toLoad = dirs.map((d) => d.id);
      return toLoad.length > 0 ? [toLoad] : undefined;
    },
    async task(toLoad, ctx) {
      await refetchStats(toLoad, ctx.signal());
    },
  });

  // 每个目录与其"大未评级"判定结果，供隐藏计数与排序共同消费
  const directoryStates = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;
    const showLarge = toValue(options?.showLargeUnrated) ?? false;

    return dirs.map((dir) => {
      const stats = getCachedStats(dir.id);
      const unratedCount = stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      const isLargeUnrated =
        !showLarge && limit !== undefined && stats?.subdirectoryCount === 0 && unratedCount > limit;
      return { dir, stats, isLargeUnrated };
    });
  });

  // 因「显示未评级图片较多的目录」开关关闭而被隐藏的叶子目录数
  const largeUnratedHiddenCount = computed(() => {
    return directoryStates.value.filter((item) => item.isLargeUnrated).length;
  });

  const sortedDirectories = computed(() => {
    const visibleItems = directoryStates.value.filter((item) => !item.isLargeUnrated);

    return sortBy(visibleItems, [
      (item) => !item.stats,
      (item) => item.stats?.imageCount === 0,
      (item) => {
        const modTime = item.stats?.latestImage?.modTime;
        return modTime ? -new Date(modTime).getTime() : 0;
      },
    ]).map((item) => item.dir);
  });

  return {
    currentDirectory,
    largeUnratedCount,
    largeUnratedHiddenCount,
    sortedDirectories,
    hasNextPage,
    fetchMore,
  };
}
