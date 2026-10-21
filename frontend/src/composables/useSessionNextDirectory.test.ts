import { beforeEach, describe, expect, test, vi } from "vitest";
import { computed, effectScope, ref, toValue, type Ref } from "vue";
import useSessionNextDirectory from "./useSessionNextDirectory";
import type {
  BrowseDirectoriesQueryVariables,
  DirectoryFragment,
  DirectoryStatsFragment,
} from "@/graphql/generated";
import type { PositionedDirectory } from "@/composables/domain/useDirectoryBrowse";
import { createDirectorySortKey } from "@/utils/directoryOrder";
import { directoryCursorKey, type SessionDirectoryCursor } from "./useDirectoryCursor";

// #region Mock Setup
const mocks = vi.hoisted(() => ({
  browseMock: vi.fn(),
  refetchStatsMock: vi.fn(),
  statsRefs: new Map<string, Ref<DirectoryStatsFragment | undefined>>(),
  /** 记录 useSessionNextDirectory 传给 useDirectoryBrowse 的查询变量 */
  observedVariables: { current: undefined as unknown },
}));

vi.mock("@/composables/domain/useDirectoryBrowse", () => ({
  default: (variables: unknown) => mocks.browseMock(variables),
  useDirectoryStats: () => ({
    getCachedStats: (id: string) => mocks.statsRefs.get(id)?.value,
    refetchStats: mocks.refetchStatsMock,
  }),
}));

vi.mock("./useDirectorySelectionFilters", async () => {
  const vue = await import("vue");
  return {
    showCompletedDirectories: vue.ref(false),
    minUnratedCount: vue.ref(1),
    showSmallUnrated: vue.ref(false),
  };
});
// #endregion

import {
  minUnratedCount,
  showCompletedDirectories,
  showSmallUnrated,
} from "./useDirectorySelectionFilters";

// #region 测试数据工厂
function makeDir(id: string, parentId = "parent-id"): DirectoryFragment {
  return {
    __typename: "Directory",
    id,
    parentId,
    relPath: `p/${id}`,
    root: false,
    lastSession: null,
    state: null,
  };
}

function makeStats(overrides: Partial<DirectoryStatsFragment> = {}): DirectoryStatsFragment {
  return {
    __typename: "DirectoryStats",
    imageCount: 2,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 2 }],
    ...overrides,
  };
}

/** 无默认设置、恰好 1 张图片且无子目录：按现有规则视为已达标 */
function completedStats(): DirectoryStatsFragment {
  return makeStats({
    imageCount: 1,
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 1 }],
  });
}

function statsWithModTime(modTime: string): DirectoryStatsFragment {
  // 排序只关心 modTime，其余字段与本用例无关
  return makeStats({ latestImage: { modTime } as DirectoryStatsFragment["latestImage"] });
}

/** 未评级图片数为 count 的目录统计 */
function statsWithUnrated(count: number, subdirectoryCount = 0): DirectoryStatsFragment {
  return makeStats({
    subdirectoryCount,
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count }],
  });
}

function setStats(id: string, stats: DirectoryStatsFragment | undefined) {
  const existing = mocks.statsRefs.get(id);
  if (existing) {
    existing.value = stats;
  } else {
    mocks.statsRefs.set(id, ref(stats));
  }
}
// #endregion

// #region 已加载页与位置
let directories: Ref<PositionedDirectory[]>;
let hasNextPage: Ref<boolean>;
let fetchMore: ReturnType<typeof vi.fn>;
let pageIndex: number;

/** 追加一页已加载目录：页锚点取该页首条目录的游标 */
function addPage(dirs: DirectoryFragment[], options: { after?: string | null } = {}) {
  const pageAnchor = `page-${pageIndex}`;
  const pageAfter = options.after ?? (pageIndex === 0 ? null : `page-${pageIndex - 1}-end`);
  pageIndex += 1;
  directories.value = [
    ...directories.value,
    ...dirs.map((dir) => ({
      dir,
      pageAnchor,
      pageAfter,
      serverCursor: `cursor-${dir.id}`,
    })),
  ];
}

beforeEach(() => {
  directories = ref([]);
  hasNextPage = ref(false);
  pageIndex = 0;
  fetchMore = vi.fn(async () => {});
  showCompletedDirectories.value = false;
  minUnratedCount.value = 1;
  showSmallUnrated.value = false;
  mocks.browseMock.mockReset();
  mocks.browseMock.mockImplementation((variables: BrowseDirectoriesQueryVariables) => {
    mocks.observedVariables.current = variables;
    return {
      directories: computed(() => directories.value),
      hasNextPage: computed(() => hasNextPage.value),
      fetchMore,
      data: ref({ node: {} }),
      query: {
        get variables() {
          return toValue(variables as BrowseDirectoriesQueryVariables) ?? {};
        },
        getCurrentResult: () => ({ data: {} }),
        subscribe: () => ({ unsubscribe: vi.fn() }),
      },
    };
  });
  mocks.refetchStatsMock.mockReset();
  mocks.refetchStatsMock.mockResolvedValue(undefined);
  mocks.statsRefs.clear();
  mocks.observedVariables.current = undefined;
  sessionStorage.clear();
});

/** 当前会话实际使用的查询变量（用于断言 after / filterBy 是否随游标传递） */
function observedVariables(): BrowseDirectoriesQueryVariables {
  return toValue(mocks.observedVariables.current as BrowseDirectoriesQueryVariables);
}

/** 按存储键原样读出游标，模拟会话页通过 useStorage 的读取 */
function readCursor(directoryId: string): SessionDirectoryCursor | undefined {
  const raw = sessionStorage.getItem(directoryCursorKey(directoryId));
  return raw ? (JSON.parse(raw) as SessionDirectoryCursor) : undefined;
}

/** 构造目录在进入时的游标：直接使用列表渲染该目录所用的排序键 */
function enter(id: string, stats: DirectoryStatsFragment | undefined, query: string | null = null) {
  const entry = directories.value.find((item) => item.dir.id === id);
  if (!entry) throw new Error(`missing position for ${id}`);
  const sortKey = createDirectorySortKey(
    {
      id,
      pageAnchor: entry.pageAnchor,
      serverCursor: entry.serverCursor,
      stats,
      isForcedShown: false,
    },
    "oldest-first",
  );
  sessionStorage.setItem(
    directoryCursorKey(id),
    JSON.stringify({ after: entry.pageAfter, sortKey, query }),
  );
  return sortKey;
}

function setup(sessionId: string, currentDirectory: DirectoryFragment) {
  // 先为当前已加载目录建立 stats ref，确保后续补拉结果能让 computed 失效
  for (const entry of directories.value) {
    if (!mocks.statsRefs.has(entry.dir.id)) setStats(entry.dir.id, undefined);
  }

  const scope = effectScope();
  const result = scope.run(() => useSessionNextDirectory(sessionId, () => currentDirectory));
  if (!result) throw new Error("failed to create composable");
  return { scope, ...result };
}
// #endregion

describe("useSessionNextDirectory", () => {
  test("筛选列表隐藏未评级图片较少的叶子目录后，自动切换跳过它", async () => {
    const current = makeDir("dir-a");
    const hidden = makeDir("dir-b");
    const next = makeDir("dir-c");
    const statsA = statsWithUnrated(10);
    addPage([current, hidden, next]);
    setStats("dir-a", statsA);
    setStats("dir-b", statsWithUnrated(0));
    setStats("dir-c", statsWithUnrated(10));
    showSmallUnrated.value = false;
    minUnratedCount.value = 8;
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      scope.stop();
    }
  });

  test("含子目录的未评级较少目录在筛选列表中强制显示，自动切换仍可进入", async () => {
    const current = makeDir("dir-a");
    const forced = makeDir("dir-b");
    const statsA = statsWithUnrated(10);
    addPage([current, forced]);
    setStats("dir-a", statsA);
    setStats("dir-b", statsWithUnrated(0, 1));
    showSmallUnrated.value = false;
    minUnratedCount.value = 8;
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("打开「显示未评级较少目录」开关后，自动切换不再跳过该目录", async () => {
    const current = makeDir("dir-a");
    const small = makeDir("dir-b");
    const statsA = statsWithUnrated(10);
    addPage([current, small]);
    setStats("dir-a", statsA);
    setStats("dir-b", statsWithUnrated(0));
    showSmallUnrated.value = true;
    minUnratedCount.value = 8;
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("进入未完成目录后，跳到进入时位置之后第一个未完成目录", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const next = makeDir("dir-c");
    const statsA = makeStats();
    addPage([current, completed, next]);
    setStats("dir-a", statsA);
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      scope.stop();
    }
  });

  test("当前目录在进入后被本次筛选改成完成，仍按进入位置定位到其后一项", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const next = makeDir("dir-c");
    const statsA = makeStats();
    addPage([current, completed, next]);
    setStats("dir-a", statsA);
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");

      // 会话提交后当前目录从待筛选列表中变成已完成
      setStats("dir-a", completedStats());
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      scope.stop();
    }
  });

  test("会话页只读取游标，不重新生成", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    addPage([current, next]);
    setStats("dir-a", makeStats());
    setStats("dir-b", makeStats());

    const { scope } = setup("session-1", current);
    try {
      await vi.waitFor(() => expect(mocks.browseMock).toHaveBeenCalled());
      expect(readCursor("dir-a")).toBeUndefined();
    } finally {
      scope.stop();
    }
  });

  test("同一会话内目录内容变化后不改变进入位置", async () => {
    const current = makeDir("dir-a");
    const older = makeDir("dir-b");
    const newer = makeDir("dir-c");
    const statsA = statsWithModTime("2024-01-02T00:00:00.000Z");
    addPage([current, older, newer]);
    setStats("dir-a", statsA);
    setStats("dir-b", statsWithModTime("2024-01-01T00:00:00.000Z"));
    setStats("dir-c", statsWithModTime("2024-01-03T00:00:00.000Z"));
    const before = enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      // 目录内容变化：最新图片时间变新，若重新计算会排到 dir-c 之后
      setStats("dir-a", statsWithModTime("2024-12-31T00:00:00.000Z"));

      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
      expect(readCursor("dir-a")?.sortKey).toBe(before);
    } finally {
      scope.stop();
    }
  });

  test("当前页没有候选时按服务端分页继续查找，不做全量加载", async () => {
    const current = makeDir("dir-1");
    const firstPage = [current, ...Array.from({ length: 99 }, (_, i) => makeDir(`dir-${i + 2}`))];
    const lastPage = [makeDir("dir-101"), makeDir("dir-102")];
    const statsA = makeStats();
    addPage(firstPage);
    hasNextPage.value = true;

    for (const dir of [...firstPage, ...lastPage]) {
      // 当前目录与第二页末尾目录未完成，其余目录均已达标
      setStats(dir.id, ["dir-1", "dir-102"].includes(dir.id) ? makeStats() : completedStats());
    }
    enter("dir-1", statsA);

    fetchMore.mockImplementation(async () => {
      addPage(lastPage, { after: `page-0-end` });
      hasNextPage.value = false;
    });

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-102");
      expect(fetchMore).toHaveBeenCalledTimes(1);
    } finally {
      scope.stop();
    }
  });

  test("当前目录位于第二页时，仍应继续加载后续页并找到下一个目录", async () => {
    const firstPage = Array.from({ length: 100 }, (_, i) => makeDir(`dir-${i + 1}`));
    const current = makeDir("dir-101");
    const secondPage = [current, makeDir("dir-102")];
    const thirdPage = [makeDir("dir-103")];
    const statsA = makeStats();
    addPage(firstPage);
    addPage(secondPage);
    hasNextPage.value = true;

    for (const dir of [...firstPage, ...secondPage, ...thirdPage]) {
      setStats(dir.id, dir.id === "dir-103" ? makeStats() : completedStats());
    }
    setStats(current.id, statsA);
    enter("dir-101", statsA);

    fetchMore.mockImplementation(async () => {
      addPage(thirdPage);
      hasNextPage.value = false;
    });

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-103");
      expect(fetchMore).toHaveBeenCalledTimes(1);
    } finally {
      scope.stop();
    }
  });

  test("下一页的候选目录会写入带该页锚点的游标供下一会话复用", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current]);
    hasNextPage.value = true;
    setStats(current.id, statsA);
    setStats(next.id, makeStats());
    enter("dir-a", statsA);

    fetchMore.mockImplementation(async () => {
      addPage([next]);
      hasNextPage.value = false;
    });

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");

      const target = readCursor("dir-b");
      expect(target?.after).toBe("page-0-end");
      expect(typeof target?.sortKey).toBe("string");
    } finally {
      scope.stop();
    }
  });

  test("同一页内的下一个目录也会写入游标，供下一会话复用当前页", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, next]);
    setStats(current.id, statsA);
    setStats(next.id, makeStats());
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");

      const target = readCursor("dir-b");
      expect(target?.after).toBeNull();
      expect(typeof target?.sortKey).toBe("string");
    } finally {
      scope.stop();
    }
  });

  test("游标带页锚点时直接从该页开始查询", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, next], { after: "page-2" });
    setStats("dir-a", statsA);
    setStats("dir-b", makeStats());
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-2", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
      expect(observedVariables()).toEqual({ id: "parent-id", after: "page-2" });
      expect(fetchMore).not.toHaveBeenCalled();
    } finally {
      scope.stop();
    }
  });

  test("搜索态下自动切换的查询与列表带同一 filterBy.query", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, next]);
    setStats("dir-a", statsA);
    setStats("dir-b", makeStats());
    enter("dir-a", statsA, "dir-");

    const { scope, resolveNextDirectory } = setup("session-2", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
      expect(observedVariables()).toEqual({ id: "parent-id", filterBy: { query: "dir-" } });
    } finally {
      scope.stop();
    }
  });

  test("无游标进入时请求不带分页参数，自动切换从序列第一个未完成目录开始", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    addPage([makeDir("dir-0"), current, next]);
    setStats("dir-0", completedStats());
    setStats("dir-a", completedStats());
    setStats("dir-b", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
      expect(observedVariables()).toEqual({ id: "parent-id" });
    } finally {
      scope.stop();
    }
  });

  test("两页最新图片时间交错时，页序与页内顺序共同决定下一个目录", async () => {
    // 服务端页 1：[p3(03), p1(01)]，页 2：[p2(02), p0(00)]
    // 列表顺序：[p1, p3, p0, p2]
    const p3 = makeDir("p3");
    const p1 = makeDir("p1");
    const p2 = makeDir("p2");
    const p0 = makeDir("p0");
    addPage([p3, p1]);
    hasNextPage.value = true;

    setStats("p3", statsWithModTime("2024-01-03T00:00:00.000Z"));
    setStats("p1", statsWithModTime("2024-01-01T00:00:00.000Z"));
    setStats("p2", statsWithModTime("2024-01-02T00:00:00.000Z"));
    setStats("p0", statsWithModTime("2024-01-01T00:00:00.000Z"));
    enter("p1", statsWithModTime("2024-01-01T00:00:00.000Z"));

    fetchMore.mockImplementation(async () => {
      addPage([p2, p0]);
      hasNextPage.value = false;
    });

    const first = setup("session-1", p1);
    try {
      // 页 1 内下一个是 p3，而不是全局续排的 p2
      await expect(first.resolveNextDirectory()).resolves.toBe("p3");
    } finally {
      first.scope.stop();
    }

    // 从 p3 继续：页 1 已无候选，只推进一页并取页 2 内排序后的第一项 p0
    const second = setup("session-2", p3);
    try {
      await expect(second.resolveNextDirectory()).resolves.toBe("p0");
    } finally {
      second.scope.stop();
    }
  });

  test("候选目录 stats 未缓存时先补拉，再按补拉结果跳过已完成目录", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const unknown = makeDir("dir-c");
    const statsA = makeStats();
    addPage([current, completed, unknown]);
    setStats("dir-a", statsA);
    enter("dir-a", statsA);

    mocks.refetchStatsMock.mockImplementation(async (ids: string[]) => {
      // 只补拉成功的目录进入缓存；未返回的目录按未完成处理
      if (ids.includes("dir-b")) setStats("dir-b", completedStats());
    });

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
      expect(mocks.refetchStatsMock.mock.calls[0]?.[0]).toEqual(
        expect.arrayContaining(["dir-b", "dir-c"]),
      );
    } finally {
      scope.stop();
    }
  });

  test("补拉 stats 失败时按未完成处理并继续跳转", async () => {
    const current = makeDir("dir-a");
    const unknown = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, unknown]);
    setStats("dir-a", statsA);
    enter("dir-a", statsA);

    mocks.refetchStatsMock.mockResolvedValue(undefined);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("当前位置之后没有未完成目录时返回 undefined", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, completed]);
    setStats("dir-a", statsA);
    setStats("dir-b", completedStats());
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBeUndefined();
    } finally {
      scope.stop();
    }
  });

  test("筛选列表显示已完成目录时，已完成当前位置仍能定位到下一个未完成目录", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    const statsA = makeStats();
    addPage([current, next]);
    setStats("dir-a", completedStats());
    setStats("dir-b", makeStats());
    showCompletedDirectories.value = true;
    enter("dir-a", statsA);

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("当前目录不在已加载列表中且没有游标时，从序列第一个未完成目录开始", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    addPage([makeDir("dir-x"), next]);
    setStats("dir-x", completedStats());
    setStats("dir-a", completedStats());
    setStats("dir-b", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("会话期间刷新后从 sessionStorage 复用同一游标", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const next = makeDir("dir-c");
    const statsA = makeStats();
    addPage([current, completed, next]);
    setStats("dir-a", statsA);
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());
    enter("dir-a", statsA);

    const first = setup("session-1", current);
    try {
      await expect(first.resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      first.scope.stop();
    }

    // 模拟刷新：页面状态重建，当前目录已被本次筛选改成完成
    setStats("dir-a", completedStats());
    const reloaded = setup("session-1", current);
    try {
      await expect(reloaded.resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      reloaded.scope.stop();
    }
  });
});
