import type { DirectorySortKey } from "@/utils/directoryOrder";

/**
 * 筛选导航游标：进入目录时确定的位置。
 *
 * 位置由「页锚点 + 页内排序键」编码成的排序键表示，目录内容变化后位置不漂移；
 * `query` 承载进入时列表的搜索词，自动切换沿用同一查询。
 * 游标原样存读，不做运行时校验：失效的游标交给服务端分页按字符串比较，
 * 表现为查询结果异常，刷新页面即可恢复。
 */
export interface SessionDirectoryCursor {
  /**
   * 当前目录所在服务端页的请求锚点（首页为 null），按 forward 分页使用。
   * 服务端游标对前端不透明，只可做相等与字符串比较；没有锚点时省略 `after`，等价于序列起点。
   */
  after: string | null;
  /** 进入时列表渲染该目录所用的排序键 */
  sortKey: DirectorySortKey;
  /** 进入时列表的 basename 搜索词；只承载搜索词，不含其他筛选条件 */
  query?: string | null;
}

// 目录游标键前缀，后缀用于避免与其他存储键意外冲突
const DIRECTORY_CURSOR_KEY_PREFIX = "directoryEntryCursor_0ea737683c03-";

// 浏览场景的游标只保存于模块内存：刷新页面视为一次新的进入操作
let browseCursor: DirectorySortKey | undefined;

/**
 * 目录游标的存储键。按目录 ID 保存：自动切换时上一个会话会直接写入目标目录的游标，
 * 目标目录的会话尚不存在，因此不能以会话 ID 为键。
 */
export function directoryCursorKey(directoryId: string): string {
  return `${DIRECTORY_CURSOR_KEY_PREFIX}${directoryId}`;
}

export function getBrowseCursor(): DirectorySortKey | undefined {
  return browseCursor;
}

export function setBrowseCursor(cursor: DirectorySortKey | undefined): void {
  browseCursor = cursor;
}

/** 写入目标目录的进入位置；读取方通过 useStorage 响应式读取同一键 */
export function setDirectoryCursor(
  directoryId: string,
  cursor: SessionDirectoryCursor | undefined,
): void {
  if (!directoryId) return;

  const key = directoryCursorKey(directoryId);
  if (cursor) {
    sessionStorage.setItem(key, JSON.stringify(cursor));
  } else {
    sessionStorage.removeItem(key);
  }
}
