import { describe, expect, test } from "vitest";
import {
  createDirectoryCursor,
  findNextDirectory,
  findPreviousDirectory,
  sortDirectoryOrder,
  type DirectoryOrderEntry,
  type DirectoryOrderStats,
} from "./directoryOrder";

// #region 测试夹具
function makeStats(overrides: Partial<DirectoryOrderStats> = {}): DirectoryOrderStats {
  return {
    imageCount: 2,
    latestImage: { modTime: "2024-01-02T00:00:00.000Z" },
    ...overrides,
  };
}

function makeEntry(
  id: string,
  overrides: Partial<Omit<DirectoryOrderEntry, "id">> = {},
): DirectoryOrderEntry {
  return {
    id,
    stats: makeStats(),
    isForcedShown: false,
    ...overrides,
  };
}
// #endregion

describe("createDirectoryCursor", () => {
  test("游标由强制显示、统计数据、空目录与最新图片时间组成", () => {
    const cursor = createDirectoryCursor(
      makeEntry("dir-a", {
        isForcedShown: true,
        stats: makeStats({ imageCount: 0, latestImage: null }),
      }),
    );

    expect(cursor).toEqual({
      isForcedShown: true,
      hasStats: true,
      isEmpty: true,
      modTime: "",
    });
  });

  test("stats 缺失时游标记录无统计数据", () => {
    const cursor = createDirectoryCursor(makeEntry("dir-a", { stats: undefined }));

    expect(cursor).toEqual({
      isForcedShown: false,
      hasStats: false,
      isEmpty: false,
      modTime: "",
    });
  });
});

describe("sortDirectoryOrder", () => {
  test("依次按强制显示、是否有 stats、是否空目录、最新图片时间倒序排列", () => {
    const entries = [
      makeEntry("no-stats", { stats: undefined }),
      makeEntry("empty", {
        stats: makeStats({ imageCount: 0, latestImage: null }),
      }),
      makeEntry("older", {
        stats: makeStats({ latestImage: { modTime: "2024-01-01T00:00:00.000Z" } }),
      }),
      makeEntry("newer", {
        stats: makeStats({ latestImage: { modTime: "2024-01-03T00:00:00.000Z" } }),
      }),
      makeEntry("forced", { isForcedShown: true }),
    ];

    expect(sortDirectoryOrder(entries).map((entry) => entry.id)).toEqual([
      "newer",
      "older",
      "empty",
      "no-stats",
      "forced",
    ]);
  });

  test("排序键完全相同时保持输入顺序，不再用 id 重排列表", () => {
    const entries = [makeEntry("dir-2"), makeEntry("dir-10"), makeEntry("dir-1")];

    expect(sortDirectoryOrder(entries).map((entry) => entry.id)).toEqual([
      "dir-2",
      "dir-10",
      "dir-1",
    ]);
  });
});

describe("findNextDirectory", () => {
  test("当前目录不在列表中时仍按游标定位下一项", () => {
    const cursor = createDirectoryCursor(makeEntry("removed"));
    const entries = [makeEntry("dir-a"), makeEntry("forced", { isForcedShown: true })];

    expect(findNextDirectory(entries, cursor, "removed")?.id).toBe("forced");
  });

  test("排序键并列时按当前目录 id 比较", () => {
    const cursor = createDirectoryCursor(makeEntry("dir-b"));
    const entries = [makeEntry("dir-c"), makeEntry("dir-a"), makeEntry("dir-d")];

    expect(findNextDirectory(entries, cursor, "dir-b")?.id).toBe("dir-c");
  });

  test("目录被重命名后使用新 id 定位到新列表中的正确位置", () => {
    // 目录移动/重命名后排序键不变，id 变成 new-name，但仍应取其后的一项
    const cursor = createDirectoryCursor(makeEntry("old-name"));
    const entries = [makeEntry("next"), makeEntry("new-name")];

    expect(findNextDirectory(entries, cursor, "new-name")?.id).toBe("next");
  });

  test("调用方预先过滤候选后，导航只在过滤后的列表中查找", () => {
    const cursor = createDirectoryCursor(makeEntry("dir-b", { stats: undefined }));
    // dir-c 已完成，被调用方提前过滤掉
    const entries = [
      makeEntry("dir-d", { stats: undefined }),
      makeEntry("dir-e", { stats: undefined }),
    ];

    expect(findNextDirectory(entries, cursor, "dir-b")?.id).toBe("dir-d");
  });

  test("当前位置之后没有候选时返回 undefined", () => {
    const cursor = createDirectoryCursor(makeEntry("dir-d"));
    const entries = [makeEntry("dir-a"), makeEntry("dir-b")];

    expect(findNextDirectory(entries, cursor, "dir-d")).toBeUndefined();
  });
});

describe("findPreviousDirectory", () => {
  test("当前目录不在列表中时仍按游标定位上一项", () => {
    const cursor = createDirectoryCursor(
      makeEntry("removed", {
        stats: makeStats({ latestImage: { modTime: "2024-01-01T00:00:00.000Z" } }),
      }),
    );
    const entries = [
      makeEntry("dir-a", {
        stats: makeStats({ latestImage: { modTime: "2024-01-03T00:00:00.000Z" } }),
      }),
      makeEntry("empty", { stats: makeStats({ imageCount: 0, latestImage: null }) }),
    ];

    expect(findPreviousDirectory(entries, cursor, "removed")?.id).toBe("dir-a");
  });

  test("排序键并列时按当前目录 id 比较", () => {
    const cursor = createDirectoryCursor(makeEntry("dir-b"));
    const entries = [makeEntry("dir-c"), makeEntry("dir-a"), makeEntry("dir-d")];

    expect(findPreviousDirectory(entries, cursor, "dir-b")?.id).toBe("dir-a");
  });

  test("目录被重命名后使用新 id 定位到新列表中的正确位置", () => {
    const cursor = createDirectoryCursor(makeEntry("old-name"));
    const entries = [makeEntry("new-name"), makeEntry("earlier")];

    expect(findPreviousDirectory(entries, cursor, "new-name")?.id).toBe("earlier");
  });
});
