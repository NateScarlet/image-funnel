import { beforeEach, describe, expect, test, vi } from "vitest";
import { shallowMount } from "@vue/test-utils";
import { ref, type Ref } from "vue";
import DirectorySelector from "./DirectorySelector.vue";
import DirectoryItem from "./DirectoryItem.vue";
import type { DirectoryFragment, DirectoryStatsFragment } from "@/graphql/generated";
import { directoryCursorKey, type SessionDirectoryCursor } from "@/composables/useDirectoryCursor";
import type { PositionedDirectory } from "@/composables/domain/useDirectoryBrowse";
import { createDirectorySortKey } from "@/utils/directoryOrder";

// #region Mock Setup
const mocks = vi.hoisted(() => ({
  directories: undefined as unknown as Ref<PositionedDirectory[]>,
  statsById: {} as Record<string, DirectoryStatsFragment | undefined>,
}));

vi.mock("@/composables/useDirectories", () => ({
  default: () => ({
    // 取值为惰性的：测试在 beforeEach 中重建数据
    get directories() {
      return mocks.directories;
    },
    currentDirectory: ref(undefined),
    hasNextPage: ref(false),
    fetchMore: vi.fn(),
  }),
}));

vi.mock("@/composables/domain/useDirectoryBrowse", () => ({
  useDirectoryStats: () => ({ getCachedStats: (id: string) => mocks.statsById[id] }),
}));

vi.mock("@/composables/useDirectorySelectionFilters", async () => {
  const vue = await import("vue");
  return {
    showCompletedDirectories: vue.ref(false),
    minUnratedCount: vue.ref(1),
    showSmallUnrated: vue.ref(false),
  };
});

vi.mock("@/composables/useInfiniteScroll", () => ({ default: vi.fn() }));

// jsdom 无 IndexedDB：真实 client 模块加载期会恢复缓存报错，与本测试关注的列表顺序无关
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));
// #endregion

// #region 测试数据工厂
function makeDir(id: string, relPath: string): DirectoryFragment {
  return {
    __typename: "Directory",
    id,
    parentId: "parent-id",
    relPath,
    root: false,
    lastSession: null,
    state: null,
  };
}

function makeStats(modTime: string): DirectoryStatsFragment {
  return {
    __typename: "DirectoryStats",
    imageCount: 2,
    subdirectoryCount: 0,
    latestImage: { modTime } as DirectoryStatsFragment["latestImage"],
    // 未评级数量高于阈值，避免被「未评级图片较少」开关隐藏
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 2 }],
  };
}

/** 页 1：[p3(03), p1(01)]，页 2：[p2(02), p0(00)]；页序与时间顺序交错 */
function setInterleavedPages() {
  const p3 = makeDir("p3", "p/p3");
  const p1 = makeDir("p1", "p/p1");
  const p2 = makeDir("p2", "p/p2");
  const p0 = makeDir("p0", "p/p0");
  mocks.directories = ref([
    { dir: p3, pageAnchor: "page-1", pageAfter: null, serverCursor: "cursor-p3" },
    { dir: p1, pageAnchor: "page-1", pageAfter: null, serverCursor: "cursor-p1" },
    { dir: p2, pageAnchor: "page-2", pageAfter: "page-1-end", serverCursor: "cursor-p2" },
    { dir: p0, pageAnchor: "page-2", pageAfter: "page-1-end", serverCursor: "cursor-p0" },
  ]);
  mocks.statsById = {
    p3: makeStats("2024-01-03T00:00:00.000Z"),
    p1: makeStats("2024-01-01T00:00:00.000Z"),
    p2: makeStats("2024-01-02T00:00:00.000Z"),
    p0: makeStats("2024-01-01T00:00:00.000Z"),
  };
}
// #endregion

/** 按存储键原样读出游标，模拟会话页通过 useStorage 的读取 */
function readCursor(directoryId: string): SessionDirectoryCursor | undefined {
  const raw = sessionStorage.getItem(directoryCursorKey(directoryId));
  return raw ? (JSON.parse(raw) as SessionDirectoryCursor) : undefined;
}

function mountSelector() {
  return shallowMount(DirectorySelector, {
    props: {
      rootPath: "/root",
      modelValue: "parent-id",
    },
  });
}

function itemDirectory(component: { props: () => unknown }): DirectoryFragment {
  return (component.props() as { directory: DirectoryFragment }).directory;
}

/** 列表项的渲染顺序（跳过顶部的当前目录项） */
function renderedRelPaths(wrapper: ReturnType<typeof mountSelector>) {
  return wrapper
    .findAllComponents(DirectoryItem)
    .filter((item) => itemDirectory(item).id !== "parent-id")
    .map((item) => itemDirectory(item).relPath);
}

describe("DirectorySelector 列表顺序", () => {
  beforeEach(() => {
    sessionStorage.clear();
    mocks.directories = ref([]);
    mocks.statsById = {};
  });

  test("页间保持服务端页顺序，页内按 oldest-first 排列", () => {
    setInterleavedPages();

    const wrapper = mountSelector();

    expect(renderedRelPaths(wrapper)).toEqual(["p/p1", "p/p3", "p/p0", "p/p2"]);
  });

  test("选中目录时直接写入列表渲染所用的排序键", () => {
    setInterleavedPages();
    const wrapper = mountSelector();
    const item = wrapper
      .findAllComponents(DirectoryItem)
      .find((component) => itemDirectory(component).id === "p2");
    if (!item) throw new Error("directory item not rendered");

    item.vm.$emit("update:modelValue", "p2");

    const expectedSortKey = createDirectorySortKey(
      {
        id: "p2",
        pageAnchor: "page-2",
        serverCursor: "cursor-p2",
        stats: makeStats("2024-01-02T00:00:00.000Z"),
        isForcedShown: false,
      },
      "oldest-first",
    );
    expect(readCursor("p2")).toEqual({
      after: "page-1-end",
      sortKey: expectedSortKey,
      query: null,
    });
  });
});
