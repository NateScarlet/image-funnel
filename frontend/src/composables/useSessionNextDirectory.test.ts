import { beforeEach, describe, expect, test, vi } from "vitest";
import { computed, effectScope, ref, type Ref } from "vue";
import useSessionNextDirectory from "./useSessionNextDirectory";
import type { DirectoryFragment, DirectoryStatsFragment } from "@/graphql/generated";

// #region Mock Setup
const mocks = vi.hoisted(() => ({
  browseMock: vi.fn(),
  refetchStatsMock: vi.fn(),
  statsRefs: new Map<string, Ref<DirectoryStatsFragment | undefined>>(),
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

// #region 挂载辅助
let directories: Ref<DirectoryFragment[]>;
let hasNextPage: Ref<boolean>;
let fetchMore: ReturnType<typeof vi.fn>;

beforeEach(() => {
  directories = ref([]);
  hasNextPage = ref(false);
  fetchMore = vi.fn(async () => {});
  showCompletedDirectories.value = false;
  minUnratedCount.value = 1;
  showSmallUnrated.value = false;
  mocks.browseMock.mockReset();
  mocks.browseMock.mockImplementation(() => ({
    liveDirectories: computed(() => directories.value),
    hasNextPage: computed(() => hasNextPage.value),
    fetchMore,
    data: ref({ node: {} }),
    query: {
      getCurrentResult: () => ({ data: {} }),
      subscribe: () => ({ unsubscribe: vi.fn() }),
    },
  }));
  mocks.refetchStatsMock.mockReset();
  mocks.refetchStatsMock.mockResolvedValue(undefined);
  mocks.statsRefs.clear();
  sessionStorage.clear();
});

function setup(sessionId: string, currentDirectory: DirectoryFragment) {
  // 先为当前已加载目录建立 stats ref，确保后续补拉结果能让 computed 失效
  for (const dir of directories.value) {
    if (!mocks.statsRefs.has(dir.id)) setStats(dir.id, undefined);
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
    directories.value = [current, hidden, next];
    setStats("dir-a", statsWithUnrated(10));
    setStats("dir-b", statsWithUnrated(0));
    setStats("dir-c", statsWithUnrated(10));
    showSmallUnrated.value = false;
    minUnratedCount.value = 8;

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
    directories.value = [current, forced];
    setStats("dir-a", statsWithUnrated(10));
    setStats("dir-b", statsWithUnrated(0, 1));
    showSmallUnrated.value = false;
    minUnratedCount.value = 8;

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
    directories.value = [current, small];
    setStats("dir-a", statsWithUnrated(10));
    setStats("dir-b", statsWithUnrated(0));
    showSmallUnrated.value = true;
    minUnratedCount.value = 8;

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
    directories.value = [current, completed, next];
    setStats("dir-a", makeStats());
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      scope.stop();
    }
  });

  test("当前目录在进入后被本次筛选改成完成，仍按游标定位到其后一项", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const next = makeDir("dir-c");
    directories.value = [current, completed, next];
    setStats("dir-a", makeStats());
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");

      // 会话提交后当前目录从待筛选列表中消失
      setStats("dir-a", completedStats());
      await expect(resolveNextDirectory()).resolves.toBe("dir-c");
    } finally {
      scope.stop();
    }
  });

  test("会话入口挂载时即生成游标，提交时当前目录已完成也能继续", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    directories.value = [current, next];
    setStats("dir-a", makeStats());
    setStats("dir-b", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      // 不手动触发 prepare，验证挂载任务已在进入时写入游标
      const { getSessionCursor } = await import("./useDirectoryCursor");
      await vi.waitFor(() => expect(getSessionCursor("session-1")).toBeDefined());

      // 筛选完成：当前目录从待筛选列表中消失
      setStats("dir-a", completedStats());
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("父目录超过一页时先加载完整列表再导航", async () => {
    const current = makeDir("dir-1");
    const firstPage = [current, ...Array.from({ length: 99 }, (_, i) => makeDir(`dir-${i + 2}`))];
    const lastPage = [makeDir("dir-101"), makeDir("dir-102")];
    directories.value = firstPage;
    hasNextPage.value = true;

    for (const dir of [...firstPage, ...lastPage]) {
      // 当前目录与第二页末尾目录未完成，其余目录均已达标
      setStats(dir.id, ["dir-1", "dir-102"].includes(dir.id) ? makeStats() : completedStats());
    }

    fetchMore.mockImplementation(async () => {
      directories.value = [...firstPage, ...lastPage];
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

  test("候选目录 stats 未缓存时先补拉，再按补拉结果跳过已完成目录", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const unknown = makeDir("dir-c");
    directories.value = [current, completed, unknown];
    setStats("dir-a", makeStats());

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
    directories.value = [current, unknown];
    setStats("dir-a", makeStats());

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
    directories.value = [current, completed];
    setStats("dir-a", makeStats());
    setStats("dir-b", completedStats());

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
    directories.value = [current, next];
    setStats("dir-a", completedStats());
    setStats("dir-b", makeStats());
    showCompletedDirectories.value = true;

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBe("dir-b");
    } finally {
      scope.stop();
    }
  });

  test("直接打开已完成会话且没有游标时，无法取得排序键并回首页", async () => {
    const current = makeDir("dir-a");
    const next = makeDir("dir-b");
    directories.value = [current, next];
    setStats("dir-a", completedStats());
    setStats("dir-b", makeStats());

    const { scope, resolveNextDirectory } = setup("session-1", current);
    try {
      await expect(resolveNextDirectory()).resolves.toBeUndefined();
    } finally {
      scope.stop();
    }
  });

  test("会话期间刷新后从 sessionStorage 复用同一游标", async () => {
    const current = makeDir("dir-a");
    const completed = makeDir("dir-b");
    const next = makeDir("dir-c");
    directories.value = [current, completed, next];
    setStats("dir-a", makeStats());
    setStats("dir-b", completedStats());
    setStats("dir-c", makeStats());

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
