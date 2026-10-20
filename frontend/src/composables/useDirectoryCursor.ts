import type { DirectoryCursor } from "@/utils/directoryOrder";

// 会话游标键前缀，后缀用于避免与其他存储键意外冲突
const SESSION_CURSOR_KEY_PREFIX = "directoryCursor_0ea737683c03-";

// 浏览场景的游标只保存于模块内存：刷新页面视为一次新的进入操作
let browseCursor: DirectoryCursor | undefined;

function getSessionStorageKey(sessionId: string): string {
  return `${SESSION_CURSOR_KEY_PREFIX}${sessionId}`;
}

function isDirectoryCursor(value: unknown): value is DirectoryCursor {
  if (typeof value !== "object" || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.isForcedShown === "boolean" &&
    typeof candidate.hasStats === "boolean" &&
    typeof candidate.isEmpty === "boolean" &&
    typeof candidate.modTime === "string"
  );
}

export function getBrowseCursor(): DirectoryCursor | undefined {
  return browseCursor;
}

export function setBrowseCursor(cursor: DirectoryCursor | undefined): void {
  browseCursor = cursor;
}

export function getSessionCursor(sessionId: string): DirectoryCursor | undefined {
  if (!sessionId) return undefined;

  const rawValue = sessionStorage.getItem(getSessionStorageKey(sessionId));
  if (!rawValue) return undefined;

  try {
    const parsed = JSON.parse(rawValue) as unknown;
    return isDirectoryCursor(parsed) ? parsed : undefined;
  } catch {
    // 存储是外部边界，损坏的数据按缺失处理，等待会话页面迟到生成
    return undefined;
  }
}

export function setSessionCursor(sessionId: string, cursor: DirectoryCursor | undefined): void {
  if (!sessionId) return;

  const key = getSessionStorageKey(sessionId);
  if (cursor) {
    sessionStorage.setItem(key, JSON.stringify(cursor));
  } else {
    sessionStorage.removeItem(key);
  }
}
