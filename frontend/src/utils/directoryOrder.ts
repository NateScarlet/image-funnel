// #region 类型定义
export interface DirectoryOrderStats {
  imageCount: number;
  latestImage?: { modTime: string } | null;
}

export interface DirectoryOrderEntry {
  id: string;
  /** 所在页首条目录的服务端游标：页序优先由它决定 */
  pageAnchor: string;
  /** 目录自身的服务端游标：同页同维度并列时用于定位 */
  serverCursor: string;
  stats?: DirectoryOrderStats | null;
  /** 目录因筛选规则被排除、但因含有子目录而保留显示，总是排在普通目录之后 */
  isForcedShown: boolean;
}

/**
 * 目录排序键：单个可比较字符串。
 *
 * 编码为「页锚点 \x00 isForcedShown \x00 hasStats \x00 isEmpty \x00 modTime \x00 服务端游标」，
 * `\x00` 小于任何普通字符，维度边界不会产生歧义。页锚点是该目录所在页首条目录的服务端游标，
 * 其字符串序等于页序，因此对任何一组已加载目录做一次全局排序即得到「页序优先、页内排序」，
 * 不需要分段结构。两个方向各自编码，不互相比较。
 */
export type DirectorySortKey = string;

/** 带着列表渲染时所用排序键的条目 */
export type DirectoryOrdered<T extends DirectoryOrderEntry> = T & { sortKey: DirectorySortKey };

/**
 * 最新图片时间的排序方向。
 * 浏览视图（子目录网格与同级切换）按新到旧；创建筛选会话的列表按旧到新，
 * 两处列表的顺序不同，因此方向必须由调用方显式指定。
 */
export type DirectorySortDirection = "newest-first" | "oldest-first";
// #endregion

// #region 排序键编码
// 时间戳固定宽度（毫秒时间戳 13 位，留余量），不足时左补零，保证字典序等于数值序
const TIMESTAMP_WIDTH = 14;
const MAX_TIMESTAMP = 10 ** TIMESTAMP_WIDTH - 1;
// 无最新图片（或时间不可解析）时排在所有数字之后
const NO_TIME_SORT_KEY = "~";
const SEPARATOR = "\x00";

function modTimeSortKey(entry: DirectoryOrderEntry, direction: DirectorySortDirection): string {
  const modTime = entry.stats?.latestImage?.modTime;
  const timestamp = modTime ? Date.parse(modTime) : Number.NaN;
  if (!Number.isFinite(timestamp)) return NO_TIME_SORT_KEY;

  const clamped = Math.min(Math.max(Math.trunc(timestamp), 0), MAX_TIMESTAMP);
  const value = direction === "newest-first" ? MAX_TIMESTAMP - clamped : clamped;
  return String(value).padStart(TIMESTAMP_WIDTH, "0");
}

/** 生成目录排序键；方向决定最新图片时间维度的编码，两个方向的键不可互相比较 */
export function createDirectorySortKey(
  entry: DirectoryOrderEntry,
  direction: DirectorySortDirection,
): DirectorySortKey {
  const hasStats = entry.stats != null;
  const isEmpty = entry.stats?.imageCount === 0;

  return [
    entry.pageAnchor,
    entry.isForcedShown ? "1" : "0",
    hasStats ? "0" : "1",
    isEmpty ? "1" : "0",
    modTimeSortKey(entry, direction),
    entry.serverCursor,
  ].join(SEPARATOR);
}

/** 排序键可直接按字符串比较 */
export function compareDirectorySortKeys(a: DirectorySortKey, b: DirectorySortKey): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}
// #endregion

// #region 排序与导航
/**
 * 排序并附上每个条目渲染时使用的排序键。
 * 页锚点位于键首，因此全局排序一次即得到「页序优先、页内排序」。
 */
export function sortDirectoryOrder<T extends DirectoryOrderEntry>(
  entries: readonly T[],
  direction: DirectorySortDirection,
): DirectoryOrdered<T>[] {
  return entries
    .map((entry) => ({ ...entry, sortKey: createDirectorySortKey(entry, direction) }))
    .toSorted((a, b) => compareDirectorySortKeys(a.sortKey, b.sortKey));
}

/** 查找游标之后的下一项目录；游标与条目的排序键必须处于同一方向编码 */
export function findNextDirectory<T extends DirectoryOrderEntry>(
  entries: readonly DirectoryOrdered<T>[],
  cursor: DirectorySortKey,
): DirectoryOrdered<T> | undefined {
  return entries.find((entry) => compareDirectorySortKeys(entry.sortKey, cursor) > 0);
}

/** 查找游标之前的上一项目录，语义与 {@link findNextDirectory} 对称 */
export function findPreviousDirectory<T extends DirectoryOrderEntry>(
  entries: readonly DirectoryOrdered<T>[],
  cursor: DirectorySortKey,
): DirectoryOrdered<T> | undefined {
  let previous: DirectoryOrdered<T> | undefined;
  for (const entry of entries) {
    if (compareDirectorySortKeys(entry.sortKey, cursor) >= 0) break;
    previous = entry;
  }
  return previous;
}
// #endregion
