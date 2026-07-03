import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useDirectoryBrowse, {
  useDirectoryStats,
} from "./domain/useDirectoryBrowse";
import useAsyncTask from "@/composables/useAsyncTask";
import useStorage from "@/composables/useStorage";
import { sortBy } from "es-toolkit";
import type { BrowseDirectoriesQueryVariables } from "@/graphql/generated";

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

export default function useDirectories(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables>,
  options?: {
    loadingCount?: Ref<number>;
    maxUnratedCount?: MaybeRefOrGetter<number | undefined>;
    showLargeUnrated?: MaybeRefOrGetter<boolean | undefined>;
  },
) {
  const maxUnratedCountVal = computed(() => toValue(options?.maxUnratedCount));

  const { currentDirectory, liveDirectories, hasNextPage, fetchMore } =
    useDirectoryBrowse(variables, { loadingCount: options?.loadingCount });
  const { getCachedStats, refetchStats } = useDirectoryStats();

  const largeUnratedCount = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;
    if (limit === undefined) return 0;
    return dirs.filter((dir) => {
      const stats = getCachedStats(dir.id);
      if (!stats) return false;
      const unratedCount =
        stats.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
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

  const sortedDirectories = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;
    const showLarge = toValue(options?.showLargeUnrated) ?? false;

    const items = dirs.map((dir) => {
      const stats = getCachedStats(dir.id);
      const unratedCount =
        stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      const isLargeUnrated =
        !showLarge &&
        limit !== undefined &&
        stats?.subdirectoryCount === 0 &&
        unratedCount > limit;
      return { dir, stats, isLargeUnrated };
    });

    const filteredItems = items.filter((item) => {
      if (item.isLargeUnrated) return false;
      return true;
    });

    return sortBy(filteredItems, [
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
    sortedDirectories,
    hasNextPage,
    fetchMore,
  };
}
