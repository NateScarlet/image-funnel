// #region Mock Setup
import { describe, test, expect, vi, beforeEach } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { computed, nextTick, ref, shallowReactive } from "vue";
import type { DirectoryFragment, DirectoryStatsFragment } from "@/graphql/generated";

const mocks = vi.hoisted(() => ({
  useDirectories: vi.fn(),
  getCachedStats: vi.fn<(id: string) => DirectoryStatsFragment | undefined>(),
  refetchStats: vi.fn<(ids: string[], signal?: AbortSignal) => Promise<void>>(),
}));

// useDirectories 依赖全局筛选开关与真实查询，这里只关心它暴露的同级目录列表
vi.mock("./useDirectories", () => ({
  default: (...args: unknown[]) => mocks.useDirectories(...args),
}));

vi.mock("./domain/useDirectoryBrowse", () => ({
  useDirectoryStats: () => ({
    getCachedStats: mocks.getCachedStats,
    refetchStats: mocks.refetchStats,
  }),
}));

import useDirectoryProgress from "./useDirectoryProgress";
// #endregion

// #region 测试数据工厂
function makeDirectory(id: string): DirectoryFragment {
  return {
    __typename: "Directory",
    id,
    parentId: "parent-1",
    relPath: `p/${id}`,
    root: false,
    lastSession: null,
    state: null,
  };
}

function makeStats(totalImages: number): DirectoryStatsFragment {
  return {
    __typename: "DirectoryStats",
    imageCount: totalImages,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts:
      totalImages > 0 ? [{ __typename: "RatingCount", rating: 5, count: totalImages }] : [],
  };
}

/** 单图且无子目录：按现有完成度规则视为已完成 */
const completedStats = makeStats(1);
/** 多图：未完成 */
const uncompletedStats = makeStats(3);

function makeDirectories(ids: string[]): DirectoryFragment[] {
  return ids.map(makeDirectory);
}
// #endregion

// #region 分页数据源与统计缓存模拟
interface SetupOptions {
  directories: DirectoryFragment[];
  /** 首次读取时已缓存的统计 */
  cachedStats?: Record<string, DirectoryStatsFragment>;
  /** 补拉时能拿到的统计，缺失的 id 视为补拉失败 */
  remoteStats?: Record<string, DirectoryStatsFragment>;
  pageSize?: number;
}

function setup(options: SetupOptions) {
  const pageSize = options.pageSize ?? 100;
  const liveDirectories = ref(options.directories.slice(0, pageSize));
  const hasNextPage = ref(liveDirectories.value.length < options.directories.length);
  const fetchMore = vi.fn(async () => {
    liveDirectories.value = options.directories.slice(0, liveDirectories.value.length + pageSize);
    hasNextPage.value = liveDirectories.value.length < options.directories.length;
  });

  const statsCache = shallowReactive(new Map<string, DirectoryStatsFragment>());
  for (const [id, stats] of Object.entries(options.cachedStats ?? {})) {
    statsCache.set(id, stats);
  }
  mocks.getCachedStats.mockImplementation((id) => statsCache.get(id));

  mocks.refetchStats.mockImplementation(async (ids) => {
    for (const id of ids) {
      const remote = options.remoteStats?.[id];
      if (remote) statsCache.set(id, remote);
    }
  });

  mocks.useDirectories.mockReturnValue({
    currentDirectory: ref(undefined),
    liveDirectories,
    sortedDirectories: computed(() => liveDirectories.value),
    hasNextPage,
    fetchMore,
  });

  return { liveDirectories, hasNextPage, fetchMore };
}

/** 所有同级目录统计口径相同，避免逐条声明 */
function statsFor(
  directories: DirectoryFragment[],
  stats: DirectoryStatsFragment,
): Record<string, DirectoryStatsFragment> {
  return Object.fromEntries(directories.map((dir) => [dir.id, stats]));
}

async function createProgress(currentDirectoryId: string) {
  const progress = useDirectoryProgress(
    () => "parent-1",
    () => currentDirectoryId,
  );
  await flushPromises();
  await nextTick();
  return progress;
}
// #endregion

describe("useDirectoryProgress 解析自动切换的下一个目录", () => {
  beforeEach(() => {
    mocks.useDirectories.mockReset();
    mocks.getCachedStats.mockReset();
    mocks.refetchStats.mockReset();
  });

  test("跳过紧邻的已完成目录，返回之后第一个未完成目录", async () => {
    const directories = makeDirectories(["d1", "d2", "d3", "d4", "d5"]);
    setup({
      directories,
      cachedStats: {
        ...statsFor(directories, completedStats),
        d4: uncompletedStats,
        d5: uncompletedStats,
      },
    });

    const { resolveNextDirectory, nextDirectoryId } = await createProgress("d1");

    await expect(resolveNextDirectory()).resolves.toBe("d4");
    expect(nextDirectoryId.value).toBe("d4");
  });

  test("候选目录统计未缓存时先补拉再判定", async () => {
    const directories = makeDirectories(["d1", "d2", "d3"]);
    setup({
      directories,
      cachedStats: { d1: completedStats },
      // d2 补拉后为已完成，应继续向后
      remoteStats: { d2: completedStats, d3: uncompletedStats },
    });

    const { resolveNextDirectory } = await createProgress("d1");

    await expect(resolveNextDirectory()).resolves.toBe("d3");
    expect(mocks.refetchStats).toHaveBeenCalledWith(["d2", "d3"], expect.anything());
  });

  test("统计补拉失败时按未完成处理并跳转", async () => {
    const directories = makeDirectories(["d1", "d2", "d3"]);
    setup({
      directories,
      cachedStats: { d1: completedStats },
      // d2 补拉失败（远端无数据）→ 仍应跳转，不阻断流程
      remoteStats: { d3: uncompletedStats },
    });

    const { resolveNextDirectory } = await createProgress("d1");

    await expect(resolveNextDirectory()).resolves.toBe("d2");
  });

  test("父目录存在后续分页时先加载完整列表再判定", async () => {
    const directories = makeDirectories(
      Array.from({ length: 120 }, (_, index) => `dir-${index + 1}`),
    );
    setup({
      directories,
      cachedStats: {
        ...statsFor(directories, completedStats),
        // 第一个未完成目录落在第二页
        "dir-105": uncompletedStats,
      },
      pageSize: 100,
    });

    const { resolveNextDirectory, nextDirectoryId } = await createProgress("dir-1");

    await expect(resolveNextDirectory()).resolves.toBe("dir-105");
    expect(nextDirectoryId.value).toBe("dir-105");
  });

  test("当前位置之后没有未完成目录时没有目标", async () => {
    const directories = makeDirectories(["d1", "d2", "d3"]);
    setup({ directories, cachedStats: statsFor(directories, completedStats) });

    const { resolveNextDirectory, nextDirectoryId } = await createProgress("d1");

    await expect(resolveNextDirectory()).resolves.toBeUndefined();
    expect(nextDirectoryId.value).toBeUndefined();
  });

  test("当前目录不在同级列表中时没有目标", async () => {
    const directories = makeDirectories(["d1", "d2"]);
    setup({ directories, cachedStats: statsFor(directories, uncompletedStats) });

    const { resolveNextDirectory } = await createProgress("other");

    await expect(resolveNextDirectory()).resolves.toBeUndefined();
  });

  test("当前目录是最后一项时没有目标", async () => {
    const directories = makeDirectories(["d1", "d2"]);
    setup({ directories, cachedStats: statsFor(directories, uncompletedStats) });

    const { resolveNextDirectory } = await createProgress("d2");

    await expect(resolveNextDirectory()).resolves.toBeUndefined();
  });

  test("分页与统计未就绪前不产出预览目标", async () => {
    const directories = makeDirectories(["d1", "d2"]);
    setup({
      directories,
      cachedStats: { d1: completedStats },
      remoteStats: { d2: uncompletedStats },
    });

    const { nextDirectoryId } = useDirectoryProgress(
      () => "parent-1",
      () => "d1",
    );

    expect(nextDirectoryId.value).toBeUndefined();

    await flushPromises();
    await nextTick();

    expect(nextDirectoryId.value).toBe("d2");
  });
});
