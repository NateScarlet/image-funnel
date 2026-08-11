import optionalArray from "./optionalArray";

// #region 类型定义
export interface DirectoryCompletionSessionInfo {
  filter: {
    rating?: readonly number[] | null;
    label?: readonly string[] | null;
    query?: string | null;
  };
  targetKeep: number;
}

export interface DirectoryLike {
  lastSession?: DirectoryCompletionSessionInfo | null;
  state?: {
    lastSession?: DirectoryCompletionSessionInfo | null;
  } | null;
}

export interface DirectoryStatsLike {
  subdirectoryCount: number;
  ratingCounts: readonly { rating: number; count: number }[];
}

export interface DirectoryCompletionResult {
  lastSession?: {
    filter: {
      rating: number[];
      label?: string[];
      query?: string;
    };
    targetKeep: number;
  };
  filterRating: number[];
  targetKeep: number;
  keepCount: number;
  isCompleted: boolean;
}
// #endregion

// #region 核心计算导出
/**
 * 基于目录自身的默认设置（lastSession 配置）判定其是否达标。
 * 有默认设置时，统计数据存在且符合条件的图片数 <= 目标保留数即视为达标；
 * 无默认设置时，仅当恰好有 1 张图片且无子目录（单图无需筛选）视为达标。
 */
export function evaluateDirectoryCompletion(
  dir: DirectoryLike,
  stats?: DirectoryStatsLike | null,
): DirectoryCompletionResult {
  const session = dir.lastSession ?? dir.state?.lastSession ?? undefined;

  // 如果没默认设置，则只在恰好有 1 张图片且无子目录时视为已达标（单图无需筛选）
  if (!session) {
    const totalCount = stats?.ratingCounts.reduce((sum, rc) => sum + rc.count, 0) ?? 0;
    const isCompleted = stats != null && stats.subdirectoryCount === 0 && totalCount === 1;
    return {
      lastSession: undefined,
      filterRating: [],
      targetKeep: 0,
      keepCount: 0,
      isCompleted,
    };
  }

  const rawRating = session.filter?.rating;
  const filterRating = rawRating != null ? Array.from(rawRating) : undefined;
  const filterLabel = optionalArray(session.filter?.label?.slice());
  const targetKeep = session.targetKeep;

  const keepCount =
    stats?.ratingCounts.reduce((sum, rc) => {
      // null 或 undefined 表示未指定 rating 筛选，即匹配所有评级的图片
      if (filterRating == null) {
        return sum + rc.count;
      }
      return sum + (filterRating.includes(rc.rating) ? rc.count : 0);
    }, 0) ?? 0;

  // 判定为有统计数据且符合条件的图片数 <= 目标保留数
  const isCompleted = stats != null && keepCount <= targetKeep;

  return {
    lastSession: {
      filter: {
        rating: filterRating ?? [],
        label: filterLabel,
        query: session.filter?.query || undefined,
      },
      targetKeep,
    },
    filterRating: filterRating ?? [],
    targetKeep,
    keepCount,
    isCompleted,
  };
}
// #endregion
