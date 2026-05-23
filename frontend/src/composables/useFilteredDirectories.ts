import { computed, ref, toValue, type MaybeRefOrGetter } from "vue";
import useStorage from "@/composables/useStorage";
import useAsyncTask from "@/composables/useAsyncTask";
import useDirectoryStats from "@/composables/useDirectoryStats";
import { sortBy } from "es-toolkit";
import type { DirectoryFragment } from "@/graphql/generated";

/**
 * 过滤并排序目录列表的 composable
 * @param directories 原始目录列表
 * @param query 可选的搜索关键字
 * @returns 过滤与排序后的目录及相关的状态控制
 */
export default function useFilteredDirectories(
  directories: MaybeRefOrGetter<DirectoryFragment[]>,
  query?: MaybeRefOrGetter<string | undefined>,
) {
  // 从 localStorage 读取未评级过滤阈值，使用全局唯一的 Key
  const { model: maxUnratedCount } = useStorage<number>(
    localStorage,
    "max_unrated_count_sub_dir_bf16419b",
    () => 0,
  );

  // 从 localStorage 读取是否显示大量未评级目录的开关
  const { model: showLargeUnrated } = useStorage<boolean>(
    localStorage,
    "show_large_unrated_sub_dir_3dfc6a37",
    () => false,
  );

  const { getCachedStats, refetchStats } = useDirectoryStats();
  const loadingCount = ref(0);

  // 在后台批量加载未获取统计信息的目录，避免同时发起大量查询
  useAsyncTask({
    loadingCount,
    args() {
      const dirs = toValue(directories);
      const toLoad = dirs.map((d) => d.id);
      return toLoad.length > 0 ? [toLoad] : undefined;
    },
    async task(toLoad, ctx) {
      await refetchStats(toLoad, ctx.signal());
    },
  });

  // 计算未评级图片多于设定阈值的目录数量
  const largeUnratedCount = computed(() => {
    const dirs = toValue(directories);
    return dirs.filter((dir) => {
      const stats = getCachedStats(dir.id);
      if (!stats) return false;
      const unratedCount =
        stats.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      // 满足条件：无子目录且未评级图片数量超过阈值
      return (
        stats.subdirectoryCount === 0 && unratedCount > maxUnratedCount.value
      );
    }).length;
  });

  // 排序并过滤后的目录列表
  const sortedDirectories = computed(() => {
    const dirs = toValue(directories);
    const queryStr = query ? toValue(query)?.trim().toLowerCase() : "";

    // #region 过滤与排序逻辑
    const items = dirs.map((dir) => {
      const stats = getCachedStats(dir.id);
      const unratedCount =
        stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      const isLargeUnrated =
        stats?.subdirectoryCount === 0 && unratedCount > maxUnratedCount.value;
      return {
        dir,
        stats,
        isLargeUnrated,
      };
    });

    const filteredItems = items.filter((item) => {
      // 过滤大量未评级目录
      if (!showLargeUnrated.value && item.isLargeUnrated) {
        return false;
      }
      // 过滤搜索关键字（仅匹配最后一级目录名称，即同级目录名过滤）
      if (queryStr) {
        const dirName = getDirName(item.dir.relPath).toLowerCase();
        if (!dirName.includes(queryStr)) {
          return false;
        }
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
    // #endregion
  });

  return {
    maxUnratedCount,
    showLargeUnrated,
    largeUnratedCount,
    sortedDirectories,
    loading: computed(() => loadingCount.value > 0),
  };
}

// #region 辅助函数
/**
 * 从相对路径中解析出最后一级目录的名称
 */
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
