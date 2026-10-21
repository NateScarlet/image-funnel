import { describe, expect, test } from "vitest";
import {
  compareDirectorySortKeys,
  createDirectorySortKey,
  findNextDirectory,
  findPreviousDirectory,
  sortDirectoryOrder,
  type DirectoryOrderEntry,
} from "./directoryOrder";

// #region 测试夹具
function makeEntry(
  id: string,
  overrides: Partial<Omit<DirectoryOrderEntry, "id">> = {},
): DirectoryOrderEntry {
  return {
    id,
    pageAnchor: "page-1",
    serverCursor: `cursor-${id}`,
    stats: {
      imageCount: 2,
      latestImage: { modTime: "2024-01-02T00:00:00.000Z" },
    },
    isForcedShown: false,
    ...overrides,
  };
}

function modTimeEntry(id: string, modTime: string, overrides?: Partial<DirectoryOrderEntry>) {
  return makeEntry(id, {
    stats: { imageCount: 2, latestImage: { modTime } },
    ...overrides,
  });
}
// #endregion

describe("createDirectorySortKey", () => {
  test("排序键是单个字符串，页锚点位于最前", () => {
    const key = createDirectorySortKey(
      makeEntry("dir-a", { pageAnchor: "page-2", serverCursor: "cursor-a" }),
      "oldest-first",
    );

    expect(typeof key).toBe("string");
    expect(key.startsWith("page-2\x00")).toBe(true);
    expect(key.endsWith("\x00cursor-a")).toBe(true);
  });

  test("页锚点决定页序，优先于页内各维度", () => {
    const page1 = modTimeEntry("dir-a", "2024-12-31T00:00:00.000Z", { pageAnchor: "page-1" });
    const page2 = modTimeEntry("dir-b", "2024-01-01T00:00:00.000Z", { pageAnchor: "page-2" });

    expect(
      compareDirectorySortKeys(
        createDirectorySortKey(page1, "oldest-first"),
        createDirectorySortKey(page2, "oldest-first"),
      ),
    ).toBeLessThan(0);
  });

  test("页内按最新图片时间排序：两个方向各自编码", () => {
    const older = modTimeEntry("dir-older", "2024-01-01T00:00:00.000Z");
    const newer = modTimeEntry("dir-newer", "2024-01-03T00:00:00.000Z");

    expect(
      compareDirectorySortKeys(
        createDirectorySortKey(older, "oldest-first"),
        createDirectorySortKey(newer, "oldest-first"),
      ),
    ).toBeLessThan(0);
    expect(
      compareDirectorySortKeys(
        createDirectorySortKey(older, "newest-first"),
        createDirectorySortKey(newer, "newest-first"),
      ),
    ).toBeGreaterThan(0);
  });

  test("可见维度相同时按服务端游标形成全序", () => {
    const a = modTimeEntry("dir-a", "2024-01-01T00:00:00.000Z", { serverCursor: "01" });
    const b = modTimeEntry("dir-b", "2024-01-01T00:00:00.000Z", { serverCursor: "02" });

    expect(
      compareDirectorySortKeys(
        createDirectorySortKey(a, "oldest-first"),
        createDirectorySortKey(b, "oldest-first"),
      ),
    ).toBeLessThan(0);
  });

  test("强制显示项与无统计数据的目录排在后面", () => {
    const normal = makeEntry("dir-a");
    const forced = makeEntry("dir-b", { isForcedShown: true });
    const withoutStats = makeEntry("dir-c", { stats: undefined });

    const order = sortDirectoryOrder([forced, withoutStats, normal], "oldest-first").map(
      (entry) => entry.id,
    );
    expect(order).toEqual(["dir-a", "dir-c", "dir-b"]);
  });
});

describe("sortDirectoryOrder", () => {
  test("页序优先、页内排序，不跨页重排", () => {
    // 服务端页 1：[p3(03), p1(01)]，页 2：[p2(02), p0(00)]
    const entries = [
      modTimeEntry("p3", "2024-01-03T00:00:00.000Z", { pageAnchor: "page-1" }),
      modTimeEntry("p1", "2024-01-01T00:00:00.000Z", { pageAnchor: "page-1" }),
      modTimeEntry("p2", "2024-01-02T00:00:00.000Z", { pageAnchor: "page-2" }),
      modTimeEntry("p0", "2024-01-01T00:00:00.000Z", { pageAnchor: "page-2" }),
    ];

    expect(sortDirectoryOrder(entries, "oldest-first").map((entry) => entry.id)).toEqual([
      "p1",
      "p3",
      "p0",
      "p2",
    ]);
    expect(sortDirectoryOrder(entries, "newest-first").map((entry) => entry.id)).toEqual([
      "p3",
      "p1",
      "p2",
      "p0",
    ]);
  });

  test("每个条目附带渲染时使用的排序键", () => {
    const [entry] = sortDirectoryOrder([makeEntry("dir-a")], "oldest-first");
    if (!entry) throw new Error("missing entry");
    expect(entry.sortKey).toBe(createDirectorySortKey(entry, "oldest-first"));
  });
});

describe("findNextDirectory / findPreviousDirectory", () => {
  const entries = sortDirectoryOrder(
    [
      modTimeEntry("dir-a", "2024-01-01T00:00:00.000Z"),
      modTimeEntry("dir-b", "2024-01-02T00:00:00.000Z"),
      modTimeEntry("dir-c", "2024-01-03T00:00:00.000Z"),
    ],
    "oldest-first",
  );

  test("按排序键直接定位下一项与上一项", () => {
    const middle = entries.find((entry) => entry.id === "dir-b");
    if (!middle) throw new Error("missing fixture");

    expect(findNextDirectory(entries, middle.sortKey)?.id).toBe("dir-c");
    expect(findPreviousDirectory(entries, middle.sortKey)?.id).toBe("dir-a");
  });

  test("序列两端返回空", () => {
    const first = entries[0];
    const last = entries.at(-1);
    if (!first || !last) throw new Error("missing fixture");

    expect(findPreviousDirectory(entries, first.sortKey)).toBeUndefined();
    expect(findNextDirectory(entries, last.sortKey)).toBeUndefined();
  });
});
