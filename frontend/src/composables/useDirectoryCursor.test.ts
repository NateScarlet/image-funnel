import { beforeEach, describe, expect, test } from "vitest";
import {
  directoryCursorKey,
  getBrowseCursor,
  setBrowseCursor,
  setDirectoryCursor,
  type SessionDirectoryCursor,
} from "./useDirectoryCursor";
import type { DirectorySortKey } from "@/utils/directoryOrder";

const sortKey: DirectorySortKey = "page-1\x000\x000\x000\x001700000000000\x00cursor-a";

const cursor: SessionDirectoryCursor = {
  after: "page-2",
  sortKey,
  query: "portrait",
};

/** 按存储键原样读出游标，模拟会话页通过 useStorage 的读取 */
function readCursor(directoryId: string): SessionDirectoryCursor | undefined {
  const raw = sessionStorage.getItem(directoryCursorKey(directoryId));
  return raw ? (JSON.parse(raw) as SessionDirectoryCursor) : undefined;
}

describe("useDirectoryCursor", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setBrowseCursor(undefined);
  });

  test("筛选场景的游标按目录 ID 持久化到 sessionStorage", () => {
    setDirectoryCursor("dir-a", cursor);

    expect(readCursor("dir-a")).toEqual(cursor);
    expect(readCursor("dir-b")).toBeUndefined();
  });

  test("筛选游标保留页锚点、排序键与搜索词", () => {
    setDirectoryCursor("dir-a", { after: null, sortKey, query: null });

    expect(readCursor("dir-a")).toEqual({ after: null, sortKey, query: null });
  });

  test("浏览场景的游标只保存在模块内存中", () => {
    setBrowseCursor(sortKey);

    expect(getBrowseCursor()).toEqual(sortKey);
    expect(sessionStorage.length).toBe(0);
  });

  test("游标原样存读，不做运行时校验", () => {
    // 失效的游标不做兼容处理，原样交给服务端分页按字符串比较
    const invalid = { after: "page-2", sortKey: "失效排序键" } as unknown as SessionDirectoryCursor;
    setDirectoryCursor("dir-a", invalid);

    expect(readCursor("dir-a")).toEqual(invalid);
  });
});
