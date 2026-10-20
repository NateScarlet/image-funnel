import {
  evaluateDirectoryCompletion,
  type DirectoryLike,
  type DirectoryStatsLike,
} from "./directoryCompletion";

// #region 类型定义
export interface DirectorySelectionVisibilityInput {
  /** 是否显示已达标目录 */
  showCompleted: boolean;
  /** 是否显示未评级图片较少的目录 */
  showSmallUnrated: boolean;
  /** 未评级图片数阈值，小于该值的叶子目录在开关关闭时隐藏 */
  minUnratedCount: number;
}

export interface DirectorySelectionVisibility {
  completion: ReturnType<typeof evaluateDirectoryCompletion>;
  unratedCount: number;
  /** 无子目录且未评级图片数小于阈值 */
  isSmallUnrated: boolean;
  /** 被筛选排除、但因含有子目录而强制显示 */
  isForcedShown: boolean;
  /** 是否通过筛选开关（不含「有子目录而强制显示」） */
  passesFilter: boolean;
  /** 非搜索状态下是否显示在筛选列表中 */
  isShown: boolean;
}
// #endregion

// #region 核心计算导出
/**
 * 判定目录在「创建筛选会话」列表中的可见性。
 * 规则与 DirectorySelector 保持一致：隐藏已达标目录与未评级图片较少的叶子目录，
 * 但含有子目录的目录会强制显示，供用户继续向下选择。
 */
export function evaluateDirectorySelectionVisibility(
  dir: DirectoryLike,
  stats: DirectoryStatsLike | undefined,
  input: DirectorySelectionVisibilityInput,
): DirectorySelectionVisibility {
  const completion = evaluateDirectoryCompletion(dir, stats);
  const unratedCount = stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;

  // 未评级图片较少的判定只针对叶子目录，含子目录的目录需要保留以便继续导航
  const isSmallUnrated = stats?.subdirectoryCount === 0 && unratedCount < input.minUnratedCount;

  const isForcedShown = Boolean(
    stats &&
    stats.subdirectoryCount > 0 &&
    ((!input.showCompleted && completion.isCompleted) ||
      (!input.showSmallUnrated && unratedCount < input.minUnratedCount)),
  );

  const passesFilter =
    (input.showCompleted || !completion.isCompleted) && (input.showSmallUnrated || !isSmallUnrated);

  return {
    completion,
    unratedCount,
    isSmallUnrated,
    isForcedShown,
    passesFilter,
    isShown: passesFilter || isForcedShown,
  };
}
// #endregion
