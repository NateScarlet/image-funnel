import { describe, test, expect, vi, beforeEach } from "vitest";
import { ref } from "vue";
import useDirectories from "./useDirectories";
import type { PositionedDirectory } from "./domain/useDirectoryBrowse";
import type { DirectoryFragment, DirectoryStatsFragment } from "@/graphql/generated";

// #region 数据源 mock 与测试数据工厂
const mockDirs = ref<DirectoryFragment[]>([]);
const mockPositioned = ref<PositionedDirectory[]>([]);

// 以 relPath 为键的统计缓存，模拟 useDirectoryStats 的 getCachedStats
let statsByRelPath: Record<string, DirectoryStatsFragment | undefined> = {};

vi.mock("./domain/useDirectoryBrowse", () => ({
  default: () => ({
    currentDirectory: ref(undefined),
    liveDirectories: mockDirs,
    directories: mockPositioned,
    edges: ref([]),
    pageInfo: ref({
      __typename: "PageInfo",
      hasNextPage: false,
      hasPreviousPage: false,
      startCursor: null,
      endCursor: null,
    }),
    hasNextPage: ref(false),
    hasPreviousPage: ref(false),
    fetchMore: vi.fn(),
  }),
  useDirectoryStats: () => ({
    getCachedStats: (id: string) =>
      statsByRelPath[mockDirs.value.find((d) => d.id === id)?.relPath ?? ""],
    refetchStats: vi.fn(),
  }),
}));

vi.mock("./useAsyncTask", () => ({ default: vi.fn() }));

vi.mock("./useStorage", () => ({
  default: () => ({ model: ref(undefined), flush: vi.fn() }),
}));

function makeDir(id: string, relPath: string): DirectoryFragment {
  return {
    __typename: "Directory",
    id,
    parentId: null,
    relPath,
    root: false,
    lastSession: null,
    state: null,
  };
}

function makeStats(
  overrides?: Partial<Omit<DirectoryStatsFragment, "__typename">>,
): DirectoryStatsFragment {
  return {
    __typename: "DirectoryStats",
    imageCount: 0,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts: [],
    ...overrides,
  };
}
// #endregion

describe("useDirectories 大未评级目录隐藏计数", () => {
  beforeEach(() => {
    // dir-a：叶子目录且未评级图片超限；dir-b：普通叶子目录；dir-c：未评级超限但含有子目录
    mockDirs.value = [makeDir("dir-a", "a"), makeDir("dir-b", "b"), makeDir("dir-c", "c")];
    mockPositioned.value = mockDirs.value.map((dir) => ({
      dir,
      pageAnchor: `cursor-${dir.id}`,
      pageAfter: null,
      serverCursor: `cursor-${dir.id}`,
    }));
    statsByRelPath = {
      a: makeStats({ ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 5 }] }),
      b: makeStats(),
      c: makeStats({
        subdirectoryCount: 1,
        ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 10 }],
      }),
    };
  });

  test("开关关闭时只统计叶子且未评级超限的目录，结果不含它们", () => {
    const { largeUnratedHiddenCount, directories } = useDirectories(() => ({ id: "root" }), {
      maxUnratedCount: ref(0),
      showLargeUnrated: ref(false),
    });

    expect(largeUnratedHiddenCount.value).toBe(1);
    expect(directories.value.map((entry) => entry.dir.id)).toEqual(["dir-b", "dir-c"]);
  });

  test("打开显示开关后计数归零且隐藏目录恢复显示", () => {
    const showLarge = ref(false);
    const { largeUnratedHiddenCount, directories } = useDirectories(() => ({ id: "root" }), {
      maxUnratedCount: ref(0),
      showLargeUnrated: showLarge,
    });

    expect(largeUnratedHiddenCount.value).toBe(1);

    showLarge.value = true;

    expect(largeUnratedHiddenCount.value).toBe(0);
    expect(directories.value.map((entry) => entry.dir.id).toSorted()).toEqual([
      "dir-a",
      "dir-b",
      "dir-c",
    ]);
  });

  test("未启用该筛选选项时恒为无隐藏", () => {
    const { largeUnratedHiddenCount } = useDirectories(() => ({ id: "root" }));

    expect(largeUnratedHiddenCount.value).toBe(0);
  });
});

describe("useDirectories 目录位置", () => {
  beforeEach(() => {
    mockDirs.value = [makeDir("dir-a", "p/A"), makeDir("dir-b", "p/B")];
    mockPositioned.value = [
      {
        dir: makeDir("dir-a", "p/A"),
        pageAnchor: "cursor-A",
        pageAfter: null,
        serverCursor: "cursor-A",
      },
    ];
    statsByRelPath = {};
  });

  test("只暴露已加载目录的位置，实时新增目录没有位置", () => {
    const { directories } = useDirectories(() => ({ id: "p" }));

    expect(directories.value.map((entry) => entry.dir.id)).toEqual(["dir-a"]);
    expect(directories.value[0]).toMatchObject({
      pageAfter: null,
      pageAnchor: "cursor-A",
      serverCursor: "cursor-A",
    });
  });
});
