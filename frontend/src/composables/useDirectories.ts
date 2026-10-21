import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useDirectoryBrowse, {
  useDirectoryStats,
  type PositionedDirectory,
} from "./domain/useDirectoryBrowse";
import useAsyncTask from "@/composables/useAsyncTask";
import useStorage from "@/composables/useStorage";
import type { BrowseDirectoriesQueryVariables, DirectoryStatsFragment } from "@/graphql/generated";

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

/**
 * 目录列表的数据源：只负责取数与现有统计，不做排序。
 * 排序由消费者用 `createDirectorySortKey` 完成——排序键以页锚点开头，
 * 一次全局排序即得到「页序优先、页内排序」。
 */
export default function useDirectories(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables>,
  options?: {
    loadingCount?: Ref<number>;
    maxUnratedCount?: MaybeRefOrGetter<number | undefined>;
    showLargeUnrated?: MaybeRefOrGetter<boolean | undefined>;
  },
) {
  const maxUnratedCountVal = computed(() => toValue(options?.maxUnratedCount));

  const {
    currentDirectory,
    liveDirectories,
    directories: loadedDirectories,
    edges,
    pageInfo,
    hasNextPage,
    hasPreviousPage,
    fetchMore,
  } = useDirectoryBrowse(variables, { loadingCount: options?.loadingCount });
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

  // 每个已加载目录与其"大未评级"判定结果，供隐藏计数与列表消费
  const directoryStates = computed(() => {
    const limit = maxUnratedCountVal.value;
    const showLarge = toValue(options?.showLargeUnrated) ?? false;

    return loadedDirectories.value.map((entry) => {
      const stats = getCachedStats(entry.dir.id);
      const unratedCount = stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      const isLargeUnrated =
        !showLarge && limit !== undefined && stats?.subdirectoryCount === 0 && unratedCount > limit;
      return { ...entry, stats, isLargeUnrated };
    });
  });

  // 因「显示未评级图片较多的目录」开关关闭而被隐藏的叶子目录数
  const largeUnratedHiddenCount = computed(() => {
    return directoryStates.value.filter((item) => item.isLargeUnrated).length;
  });

  /** 未被「未评级图片过多」隐藏的目录及其位置；顺序交给消费者的排序键 */
  const directories = computed<PositionedDirectoryWithStats[]>(() =>
    directoryStates.value
      .filter((item) => !item.isLargeUnrated)
      .map(({ isLargeUnrated: _isLargeUnrated, ...entry }) => entry),
  );

  return {
    currentDirectory,
    liveDirectories,
    directories,
    edges,
    pageInfo,
    hasNextPage,
    hasPreviousPage,
    fetchMore,
    largeUnratedCount,
    largeUnratedHiddenCount,
  };
}

export type PositionedDirectoryWithStats = PositionedDirectory & {
  stats: DirectoryStatsFragment | undefined;
};
