// #region 类型定义
export interface DirectoryOrderStats {
  imageCount: number;
  latestImage?: { modTime: string } | null;
}

export interface DirectoryOrderEntry {
  id: string;
  stats?: DirectoryOrderStats | null;
  /** 目录因筛选规则被排除、但因含有子目录而保留显示，总是排在普通目录之后 */
  isForcedShown: boolean;
}

/**
 * 目录导航游标：进入目录时的排序键快照。
 * 不含 id、父目录与筛选条件，因此目录被移动/重命名后仍能在新列表中定位。
 */
export interface DirectoryCursor {
  isForcedShown: boolean;
  hasStats: boolean;
  isEmpty: boolean;
  modTime: string;
}
// #endregion

// #region 排序键
export function createDirectoryCursor(entry: DirectoryOrderEntry): DirectoryCursor {
  const stats = entry.stats;
  return {
    isForcedShown: entry.isForcedShown,
    hasStats: stats != null,
    isEmpty: stats?.imageCount === 0,
    modTime: stats?.latestImage?.modTime || "",
  };
}

/** 最新图片时间倒序（越新越靠前），无图片时排在最后 */
function modTimeRank(modTime: string): number {
  if (!modTime) return 0;
  const timestamp = Date.parse(modTime);
  return Number.isNaN(timestamp) ? 0 : -timestamp;
}

/**
 * 比较两个排序键。
 * 列表展示与游标导航共用同一排序键口径，避免浏览视图与目录选择器出现两套排序。
 * 导航时排序键相同再按 id 比较，见 {@link findNextDirectory}。
 */
export function compareDirectoryCursors(a: DirectoryCursor, b: DirectoryCursor): number {
  if (a.isForcedShown !== b.isForcedShown) {
    return a.isForcedShown ? 1 : -1;
  }
  if (a.hasStats !== b.hasStats) {
    return a.hasStats ? -1 : 1;
  }
  if (a.isEmpty !== b.isEmpty) {
    return a.isEmpty ? 1 : -1;
  }
  return modTimeRank(a.modTime) - modTimeRank(b.modTime);
}

function compareIds(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

/** 展示排序：排序键相同时保持输入顺序（稳定排序），不用 id 打乱既有列表 */
export function sortDirectoryOrder<T extends DirectoryOrderEntry>(entries: readonly T[]): T[] {
  return entries.toSorted((a, b) =>
    compareDirectoryCursors(createDirectoryCursor(a), createDirectoryCursor(b)),
  );
}
// #endregion

// #region 游标导航
/** 导航排序：排序键相同再按 id 比较，形成可定位的全序 */
function compareNavigableEntries(a: DirectoryOrderEntry, b: DirectoryOrderEntry): number {
  const bySortKey = compareDirectoryCursors(createDirectoryCursor(a), createDirectoryCursor(b));
  return bySortKey !== 0 ? bySortKey : compareIds(a.id, b.id);
}

/**
 * 查找游标之后的下一项目录。
 * 当前目录不必存在于列表中；自动切换场景由调用方预先过滤候选列表。
 */
export function findNextDirectory<T extends DirectoryOrderEntry>(
  entries: readonly T[],
  cursor: DirectoryCursor,
  currentDirectoryId: string,
): T | undefined {
  let next: T | undefined;

  for (const entry of entries) {
    const bySortKey = compareDirectoryCursors(createDirectoryCursor(entry), cursor);
    const isAfterCursor =
      bySortKey > 0 || (bySortKey === 0 && compareIds(entry.id, currentDirectoryId) > 0);
    if (!isAfterCursor) continue;

    if (!next || compareNavigableEntries(entry, next) < 0) {
      next = entry;
    }
  }

  return next;
}

/** 查找游标之前的上一项目录，语义与 {@link findNextDirectory} 对称 */
export function findPreviousDirectory<T extends DirectoryOrderEntry>(
  entries: readonly T[],
  cursor: DirectoryCursor,
  currentDirectoryId: string,
): T | undefined {
  let previous: T | undefined;

  for (const entry of entries) {
    const bySortKey = compareDirectoryCursors(createDirectoryCursor(entry), cursor);
    const isBeforeCursor =
      bySortKey < 0 || (bySortKey === 0 && compareIds(entry.id, currentDirectoryId) < 0);
    if (!isBeforeCursor) continue;

    if (!previous || compareNavigableEntries(entry, previous) > 0) {
      previous = entry;
    }
  }

  return previous;
}
// #endregion
