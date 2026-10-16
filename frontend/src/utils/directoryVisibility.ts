import { evaluateDirectoryCompletion } from "./directoryCompletion";
import type { DirectoryLike, DirectoryStatsLike } from "./directoryCompletion";

// #region 类型定义
export interface DirectoryVisibilityInput {
  /** 是否显示未达标目录 */
  showUncompleted: boolean;
  /** 是否显示未评级图片较多的目录 */
  showLargeUnrated: boolean;
  /** 未评级图片数上限，undefined 表示不限制 */
  unratedLimit?: number;
}

export interface DirectoryVisibility {
  /** 是否被当前筛选排除 */
  isFilteredOut: boolean;
  /** 被筛选排除，但因含子目录而强制显示 */
  isForcedShown: boolean;
  /** 因「显示未达标目录」开关关闭而真正被隐藏（即因未达标被过滤且非强制显示） */
  isHiddenByUncompleted: boolean;
}
// #endregion

// #region 核心计算导出
/**
 * 判定单个目录在当前客户端筛选开关下的可见性分类。
 * 达标与否复用 {@link evaluateDirectoryCompletion}，调用方无需再自行计算完成度。
 */
export function evaluateDirectoryVisibility(
  dir: DirectoryLike,
  stats: DirectoryStatsLike | undefined,
  input: DirectoryVisibilityInput,
): DirectoryVisibility {
  const { showUncompleted, showLargeUnrated, unratedLimit } = input;

  const isCompleted = evaluateDirectoryCompletion(dir, stats).isCompleted;
  const unratedCount = stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;

  const isFilteredOutByCompletion = !showUncompleted && !isCompleted;
  const isFilteredOutByUnrated =
    !showLargeUnrated && unratedLimit !== undefined && unratedCount > unratedLimit;
  const isFilteredOut = isFilteredOutByCompletion || isFilteredOutByUnrated;

  // 当有筛选限制且包含子目录时，虽然本身不满足筛选，但因为有子目录而被保留显示
  const isForcedShown = stats !== undefined && stats.subdirectoryCount > 0 && isFilteredOut;

  return {
    isFilteredOut,
    isForcedShown,
    // 因「显示未达标目录」开关被真正隐藏（排除因有子目录而强制显示的项）
    isHiddenByUncompleted: isFilteredOutByCompletion && !isForcedShown,
  };
}
// #endregion
