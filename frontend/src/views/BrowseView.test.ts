// #region Mock Setup
import { describe, test, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { computed, ref, toValue, type Ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

const { browseMock, statsByDirId, storageModels } = vi.hoisted(() => ({
  browseMock: vi.fn(),
  statsByDirId: {} as Record<string, unknown>,
  storageModels: {} as Record<string, Ref<unknown>>,
}));

vi.mock("@/graphql/utils/useQuery", () => ({ default: vi.fn() }));
vi.mock("@/graphql/utils/useSubscription", () => ({
  default: vi.fn(() => ({ [Symbol.dispose]() {} })),
}));
// jsdom 无 IndexedDB：真实 client 模块加载期会恢复缓存报错，与本测试关注的导航行为无关
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));
vi.mock("@/composables/useHotkeys", () => ({ useHotkeys: vi.fn() }));
vi.mock("@/composables/useDevices.ts", () => ({
  useDevices: () => ({ pairingRequests: ref([]) }),
}));
vi.mock("@/composables/useDirectoryState", () => ({
  useDirectoryState: () => ({ filterRating: ref([]), lastSession: ref(undefined) }),
}));
vi.mock("@/composables/useOpenDir", () => ({
  useOpenDir: () => ({ revealInExplorer: vi.fn() }),
}));
// 筛选开关是 useDirectories 的模块级状态，按 key 片段定位后由各用例覆写
vi.mock("@/composables/useStorage", () => ({
  default: (_s: unknown, key: string) => {
    const model = ref<unknown>(undefined);
    storageModels[key] = model;
    return { model, flush: vi.fn() };
  },
}));
vi.mock("@/composables/domain/useDirectoryBrowse", () => ({
  default: (v: unknown) => browseMock(v),
  useDirectoryStats: () => ({
    getCachedStats: (id: string) => statsByDirId[id],
    refetchStats: vi.fn(),
  }),
}));
// #endregion

import useQuery from "@/graphql/utils/useQuery";
import BrowseView from "./BrowseView.vue";
import type { DirectoryFragment, DirectoryStatsFragment } from "@/graphql/generated";

// #region 测试数据工厂
function makeDir(id: string, relPath: string, parentId: string | null): DirectoryFragment {
  return {
    __typename: "Directory",
    id,
    parentId,
    relPath,
    root: false,
    lastSession: null,
    state: null,
  };
}

function makeSiblings(count: number) {
  return Array.from({ length: count }, (_, i) =>
    makeDir(`dir-${i + 1}`, `p/dir-${i + 1}`, "parent-id"),
  );
}

function makeStats(overrides: Partial<DirectoryStatsFragment>): DirectoryStatsFragment {
  return {
    __typename: "DirectoryStats",
    imageCount: 1,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts: [],
    ...overrides,
  };
}

/** 单张已评级图片、无子目录的目录：达标且不会被任何筛选隐藏 */
function makeCompletedStats(): DirectoryStatsFragment {
  return makeStats({ ratingCounts: [{ __typename: "RatingCount", rating: 5, count: 1 }] });
}

function setFilterSwitches(values: {
  maxUnratedCount?: number;
  showLargeUnrated?: boolean;
  showUncompletedDirectories?: boolean;
}) {
  for (const [key, model] of Object.entries(storageModels)) {
    if (key.includes("max_unrated")) model.value = values.maxUnratedCount;
    if (key.includes("show_large")) model.value = values.showLargeUnrated;
    if (key.includes("show_uncompleted")) model.value = values.showUncompletedDirectories;
  }
}

// #endregion

// #region 模拟 Relay 连接
/** 模拟 Relay 连接的分页行为：首页随查询返回，其余页等待 fetchMore */
function createPagedSource(
  items: DirectoryFragment[],
  pageSize: number,
  mode: { stalled?: boolean; controlled?: boolean } = {},
) {
  const nodes = ref<DirectoryFragment[]>([]) as Ref<DirectoryFragment[]>;
  const hasNext = ref(false);
  /** 受控模式下由用例决定何时放行 fetchMore，用于观察加载过程中的中断 */
  let release: (() => void) | undefined;
  const fetchMore = vi.fn(async (_direction?: "forward" | "backward") => {
    if (mode.stalled) return;
    if (mode.controlled) await new Promise<void>((resolve) => (release = resolve));
    nodes.value = [
      ...nodes.value,
      ...items.slice(nodes.value.length, nodes.value.length + pageSize),
    ];
    hasNext.value = nodes.value.length < items.length;
  });
  return {
    nodes,
    hasNext,
    fetchMore,
    /** 首页查询返回 */
    settle() {
      nodes.value = items.slice(0, pageSize);
      hasNext.value = nodes.value.length < items.length;
    },
    /** 放行一次受控的 fetchMore */
    releaseFetchMore() {
      release?.();
      release = undefined;
    },
  };
}

/**
 * 按查询变量的目录 id 动态分发模拟连接，贴近真实查询在变量变化后重新解析的时序：
 * 先拿到当前目录才能推导出父目录，父目录连接再独立返回首页。
 */
function setupBrowse(options: {
  currentDir: DirectoryFragment;
  siblings: DirectoryFragment[];
  stalledFetchMore?: boolean;
  controlledFetchMore?: boolean;
}) {
  const parentId = options.siblings[0]?.parentId ?? null;
  const parentSource = createPagedSource(options.siblings, 100, {
    stalled: options.stalledFetchMore,
    controlled: options.controlledFetchMore,
  });
  const currentDirectory = ref<DirectoryFragment>();
  const parentDirectory = ref<DirectoryFragment>();
  const otherFetchMores: ReturnType<typeof vi.fn>[] = [];

  browseMock.mockImplementation((v: unknown) => {
    // 真实 ObservableQuery 只在作用域存活期间通过 setVariables 改变变量，
    // 因此这里记住最后一次非空取值，卸载后连接不会凭空消失
    let queriedId = "";
    const isParent = computed(() => {
      const dirId = (toValue(v as never) as { id?: string }).id;
      if (dirId) queriedId = dirId;
      return !!parentId && queriedId === parentId;
    });
    const otherFetchMore = vi.fn();
    otherFetchMores.push(otherFetchMore);
    // 模拟服务端页：每页 100 条，页锚点取该页首条目录的服务端游标
    const directories = computed(() => {
      if (!isParent.value) return [];

      const nodes = parentSource.nodes.value;
      const pageSize = 100;
      return nodes.map((node, index) => {
        const start = Math.floor(index / pageSize) * pageSize;
        return {
          dir: node,
          pageAnchor: nodes[start]?.relPath ?? node.relPath,
          pageAfter: start === 0 ? null : (nodes[start - 1]?.relPath ?? null),
          serverCursor: node.relPath,
        };
      });
    });

    return {
      currentDirectory: computed(() =>
        isParent.value ? parentDirectory.value : currentDirectory.value,
      ),
      liveDirectories: computed(() => (isParent.value ? parentSource.nodes.value : [])),
      directories,
      edges: computed(() =>
        isParent.value
          ? parentSource.nodes.value.map((node) => ({ cursor: node.relPath, node }))
          : [],
      ),
      pageInfo: computed(() => ({
        __typename: "PageInfo" as const,
        hasNextPage: isParent.value && parentSource.hasNext.value,
        hasPreviousPage: false,
        startCursor: null,
        endCursor: isParent.value ? (parentSource.nodes.value.at(-1)?.relPath ?? null) : null,
      })),
      hasNextPage: computed(() => isParent.value && parentSource.hasNext.value),
      hasPreviousPage: computed(() => false),
      fetchMore: (direction?: "forward" | "backward") =>
        isParent.value ? parentSource.fetchMore(direction) : otherFetchMore(),
    };
  });

  return {
    parentSource,
    otherFetchMores,
    /** 当前目录与父目录连接先后到达 */
    async settle() {
      currentDirectory.value = options.currentDir;
      await flushPromises();
      parentDirectory.value = makeDir("parent-id", "p", null);
      parentSource.settle();
      await flushPromises();
    },
  };
}
// #endregion

// #region 挂载辅助
async function mountBrowse(dirId: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div/>" } },
      { path: "/browse", name: "browse", component: BrowseView },
      { path: "/session/:id", name: "session", component: { template: "<div/>" } },
    ],
  });
  await router.replace({ path: "/browse", query: dirId ? { dir: dirId } : {} });
  await router.isReady();
  const wrapper = mount(BrowseView, {
    global: {
      plugins: [router],
      stubs: {
        SubdirectoryGrid: true,
        ImageGrid: true,
        NoteList: true,
        DirectoryBreadcrumb: true,
        RenameDirectoryForm: true,
        DeviceManagerButton: true,
        NotificationCenterButton: true,
        TrashHistoryButton: true,
      },
    },
  });
  await flushPromises();
  return wrapper;
}

/** 页面上同时存在大屏与小屏两套布局，按标题关键词取第一个匹配项 */
function findNavLink(wrapper: Awaited<ReturnType<typeof mountBrowse>>, keyword: string) {
  return wrapper.findAll("a").find((a) => a.attributes("title")?.includes(keyword));
}

function navTitle(wrapper: Awaited<ReturnType<typeof mountBrowse>>, keyword: string) {
  return findNavLink(wrapper, keyword)?.attributes("title");
}
// #endregion

describe("BrowseView 同级目录导航补齐父目录分页", () => {
  beforeEach(() => {
    browseMock.mockReset();
    vi.mocked(useQuery).mockImplementation(() => ({ data: ref(undefined) }) as never);
    for (const key of Object.keys(statsByDirId)) delete statsByDirId[key];
    // 默认不启用任何客户端筛选，目录是否可见只取决于统计数据
    setFilterSwitches({
      maxUnratedCount: undefined,
      showLargeUnrated: false,
      showUncompletedDirectories: true,
    });
  });

  test("当前目录位于父目录第二页时仍能解析到相邻目录", async () => {
    for (const dir of makeSiblings(120)) statsByDirId[dir.id] = makeCompletedStats();
    const browse = setupBrowse({
      currentDir: makeDir("dir-101", "p/dir-101", "parent-id"),
      siblings: makeSiblings(120),
    });

    const wrapper = await mountBrowse("dir-101");
    await browse.settle();

    expect(browse.parentSource.nodes.value).toHaveLength(120);
    // 服务端顺序按目录名比较，"dir-100" 排在 "dir-99" 之前
    expect(navTitle(wrapper, "上一个目录")).toContain("dir-99");
    expect(navTitle(wrapper, "下一个目录")).toContain("dir-102");
  });

  test("跨页导航与子目录网格的筛选和排序一致", async () => {
    setFilterSwitches({
      maxUnratedCount: 0,
      showLargeUnrated: false,
      showUncompletedDirectories: false,
    });
    for (const dir of makeSiblings(120)) statsByDirId[dir.id] = makeCompletedStats();
    // 未评级图片超过上限的叶子目录被隐藏，上一个目录因此退回到 dir-100
    statsByDirId["dir-101"] = makeStats({
      ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 5 }],
    });
    // 未达标但含子目录的目录保留显示并排到最后，下一个目录因此跳过它
    statsByDirId["dir-103"] = makeStats({
      subdirectoryCount: 2,
      ratingCounts: [{ __typename: "RatingCount", rating: 0, count: 5 }],
    });
    const browse = setupBrowse({
      currentDir: makeDir("dir-102", "p/dir-102", "parent-id"),
      siblings: makeSiblings(120),
    });

    const wrapper = await mountBrowse("dir-102");
    await browse.settle();

    expect(navTitle(wrapper, "上一个目录")).toContain("dir-99");
    expect(navTitle(wrapper, "下一个目录")).toContain("dir-104");
  });

  test("父目录子目录不超过一页时不加载更多且导航行为不变", async () => {
    const browse = setupBrowse({
      currentDir: makeDir("dir-3", "p/dir-3", "parent-id"),
      siblings: makeSiblings(10),
    });

    const wrapper = await mountBrowse("dir-3");
    await browse.settle();

    expect(browse.parentSource.fetchMore).not.toHaveBeenCalled();
    expect(navTitle(wrapper, "上一个目录")).toContain("dir-2");
    expect(navTitle(wrapper, "下一个目录")).toContain("dir-4");
  });

  test("分页无法推进时停止继续请求", async () => {
    const browse = setupBrowse({
      currentDir: makeDir("dir-101", "p/dir-101", "parent-id"),
      siblings: makeSiblings(120),
      stalledFetchMore: true,
    });

    await mountBrowse("dir-101");
    await browse.settle();

    expect(browse.parentSource.fetchMore).toHaveBeenCalledTimes(1);
  });

  test("卸载时中止尚未完成的分页加载", async () => {
    const browse = setupBrowse({
      currentDir: makeDir("dir-101", "p/dir-101", "parent-id"),
      siblings: makeSiblings(300),
      controlledFetchMore: true,
    });

    const wrapper = await mountBrowse("dir-101");
    await browse.settle();
    expect(browse.parentSource.fetchMore).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    browse.parentSource.releaseFetchMore();
    await flushPromises();

    expect(browse.parentSource.fetchMore).toHaveBeenCalledTimes(1);
  });

  test("位于根目录时不加载任何后续页且没有同级目录导航", async () => {
    const browse = setupBrowse({ currentDir: makeDir("root-id", "", null), siblings: [] });

    const wrapper = await mountBrowse("");
    await browse.settle();

    expect(browse.otherFetchMores.length).toBeGreaterThan(0);
    expect(browse.otherFetchMores.every((fetchMore) => !fetchMore.mock.calls.length)).toBe(true);
    expect(navTitle(wrapper, "下一个目录")).toBe("没有下一个目录");
  });
});
