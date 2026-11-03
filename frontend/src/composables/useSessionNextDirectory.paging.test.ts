// #region 说明
// 复现「筛选完成后自动切换回主页、第二页的待筛选目录被跳过」(#170)。
//
// 接缝选在假 GraphQL 服务端（ApolloLink），其余全部走真实实现——useQuery、
// useRelayConnection、utils/fetchMore 的合并、useDirectoryBrowse 的页锚点推算、
// useDirectoryStats 的缓存、useSessionNextDirectory 的游标导航。
// 此前的同类测试都 mock 掉 useDirectoryBrowse 并手工填写 pageAnchor/pageAfter，
// 页锚点推算与真实分页合并从未被执行，因此「测试通过、实测失败」无法被发现。
//
// 已确认的缺陷：`utils/fetchMore` 的 promise resolve 时，合并后的第二页尚未广播到
// ObservableQuery（Apollo 4 在宏任务中才推送），`resolveNextDirectory` 里
// 「边数没增长就放弃」的判断因此命中，直接返回 undefined，CompletedView 便回到主页。
// #endregion

import { beforeEach, describe, expect, test, vi } from "vitest";
import { effectScope, nextTick, type EffectScope } from "vue";

// #region 假服务端装配
interface FakeServer {
  /** 当前用例配置的处理函数；返回 null 表示订阅（保持沉默） */
  handle?: (operation: FakeOperation) => unknown;
  /** 记录收到的请求，用于断言分页确实推进过 */
  requests: { operationName: string; variables: Record<string, unknown> }[];
}

interface FakeOperation {
  operationName: string;
  variables: Record<string, unknown>;
}

const server = vi.hoisted<FakeServer>(() => ({ handle: undefined, requests: [] }));

vi.mock("@/graphql/client", async () => {
  const { ApolloClient, ApolloLink, InMemoryCache, Observable } =
    await import("@apollo/client/core");

  const link = new ApolloLink(
    (operation) =>
      new Observable((observer) => {
        const handler = server.handle;
        if (!handler) {
          observer.error(new Error("假服务端未配置"));
          return () => {};
        }
        const operationName = operation.operationName ?? "";
        server.requests.push({
          operationName,
          variables: { ...operation.variables },
        });
        const data = handler({
          operationName,
          variables: operation.variables as Record<string, unknown>,
        });
        // 订阅：不推送任何事件，模拟「期间没有实时变更」
        if (data === null) return () => {};
        queueMicrotask(() => {
          observer.next({ data });
          observer.complete();
        });
        return () => {};
      }),
  );

  const client = new ApolloClient({
    link,
    cache: new InMemoryCache(),
    assumeImmutableResults: true,
    // 与生产客户端一致
    defaultOptions: { watchQuery: { fetchPolicy: "cache-and-network" } },
  });
  return { default: client, client };
});
// #endregion

import { client } from "@/graphql/client";
import type { DirectoryFragment } from "@/graphql/generated";
import useDirectories from "./useDirectories";
import useDirectoryBrowse from "./domain/useDirectoryBrowse";
import useSessionNextDirectory from "./useSessionNextDirectory";
import { directoryCursorKey } from "./useDirectoryCursor";
import { sortDirectoryOrder, type DirectoryOrdered } from "@/utils/directoryOrder";

// #region 假数据：父目录下 250 个子目录，服务端按目录名分页
const PARENT_ID = "parent-1";
const PAGE_SIZE = 100;

function directoryName(index: number): string {
  return `dir-${String(index).padStart(3, "0")}`;
}

/** 服务端游标：与 Go 端 TextToCursor 一致，采用十六进制编码，字典序等于目录名顺序 */
function toCursor(name: string): string {
  return Array.from(new TextEncoder().encode(name), (b) => b.toString(16).padStart(2, "0")).join(
    "",
  );
}

interface Fixture {
  /** 已达标目录的下标：恰好 1 张未评级图片且无子目录（无默认设置时的达标判定） */
  completed: (index: number) => boolean;
}

function directoryDto(index: number): DirectoryFragment {
  const name = directoryName(index);
  return {
    __typename: "Directory",
    id: name,
    parentId: PARENT_ID,
    relPath: `p/${name}`,
    root: false,
    lastSession: null,
    state: null,
  };
}

function directoryIndexById(id: string): number {
  const match = /^dir-(\d+)$/.exec(id);
  if (!match) throw new Error(`未知目录 id: ${id}`);
  return Number(match[1]);
}

function statsDto(index: number, fixture: Fixture) {
  const imageCount = fixture.completed(index) ? 1 : 2;
  return {
    __typename: "DirectoryStats",
    imageCount,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count: imageCount }],
  };
}

/** 按 Go 端 ByTextV2 语义分页：游标 = 目录名，after 为严格大于的起点，first 为页大小 */
function browseDirectoriesPage(variables: Record<string, unknown>, total: number) {
  const first = (variables.first as number | undefined) ?? PAGE_SIZE;
  const after = variables.after as string | undefined;

  const all = Array.from({ length: total }, (_, index) => ({
    index,
    cursor: toCursor(directoryName(index)),
  }));
  const matched = after ? all.filter((item) => item.cursor > after) : all;
  const page = matched.slice(0, first);

  const edges = page.map((item) => ({
    __typename: "DirectoryEdge",
    cursor: item.cursor,
    node: directoryDto(item.index),
  }));

  return {
    node: {
      // 父目录本身只需要身份，不需要参与子目录列表
      ...directoryDto(0),
      __typename: "Directory",
      id: PARENT_ID,
      relPath: "p",
      directoriesV2: {
        __typename: "DirectoryConnection",
        edges,
        nodes: edges.map((edge) => edge.node),
        pageInfo: {
          __typename: "PageInfo",
          hasNextPage: matched.length > first,
          hasPreviousPage: after != null,
          startCursor: page.at(0)?.cursor ?? null,
          endCursor: page.at(-1)?.cursor ?? null,
        },
      },
    },
  };
}

function installServer(options: { total: number; fixture: Fixture }) {
  server.handle = (operation) => {
    switch (operation.operationName) {
      case "BrowseDirectories":
        return browseDirectoriesPage(operation.variables, options.total);
      case "DirectoryStats": {
        const index = directoryIndexById(String(operation.variables.id));
        return {
          node: {
            ...directoryDto(index),
            __typename: "Directory",
            stats: statsDto(index, options.fixture),
          },
        };
      }
      case "DirectoryChanged":
      case "DirEntryDeleted":
        return null;
      default:
        throw new Error(`未预期到的操作: ${operation.operationName}`);
    }
  };
}
// #endregion

// #region 挂载辅助
function runInScope<T>(fn: () => T): { scope: EffectScope; value: T } {
  const scope = effectScope();
  const value = scope.run(fn);
  if (value === undefined) throw new Error("effectScope 未返回结果");
  return { scope, value };
}

type ListEntry = ReturnType<typeof useDirectories>["directories"]["value"][number] & {
  id: string;
  isForcedShown: boolean;
};

/**
 * 模拟从创建筛选会话的列表进入目录：排序键与页请求锚点都取自列表渲染该目录时的数据。
 * 这里复用真实的 useDirectories + sortDirectoryOrder，避免手工拼装排序键。
 */
async function readListEntry(targetId: string): Promise<DirectoryOrdered<ListEntry>> {
  const { scope, value } = runInScope(() => useDirectories(() => ({ id: PARENT_ID })));

  await vi.waitFor(
    () => {
      expect(value.directories.value.length).toBeGreaterThan(0);
      expect(value.directories.value.every((entry) => entry.stats != null)).toBe(true);
    },
    { timeout: 20_000, interval: 50 },
  );

  const ordered = sortDirectoryOrder(
    value.directories.value.map((entry) => ({
      ...entry,
      id: entry.dir.id,
      isForcedShown: false,
    })),
    "oldest-first",
  );
  scope.stop();

  const entry = ordered.find((item) => item.id === targetId);
  if (!entry) throw new Error(`列表中找不到目录 ${targetId}`);
  return entry;
}

function enterDirectory(entry: { id: string; pageAfter: string | null; sortKey: string }): void {
  sessionStorage.setItem(
    directoryCursorKey(entry.id),
    JSON.stringify({ after: entry.pageAfter, sortKey: entry.sortKey, query: null }),
  );
}

function createNavigator(directoryId: string) {
  return runInScope(() =>
    useSessionNextDirectory("session-1", () => directoryDto(directoryIndexById(directoryId))),
  );
}

/** 让出一个宏任务，等待 Apollo 广播 fetchMore 的合并结果 */
function settleBroadcast(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}
// #endregion

beforeEach(async () => {
  server.handle = undefined;
  server.requests = [];
  sessionStorage.clear();
  localStorage.clear();
  await client.clearStore();
});

describe("筛选完成后自动切换到下一个目录（真实分页接缝）", () => {
  // 复现 #170：第二页有待筛选目录时，自动切换应进入它。
  // 用 test.fails 标记：缺陷存在期间用例以「预期失败」留在测试集中，修复后改为 test。
  test.fails("第一页全部达标时，应继续取第二页并进入其中第一个未达标目录", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const entry = await readListEntry(directoryName(0));
    enterDirectory(entry);

    const { scope, value } = createNavigator(directoryName(0));
    const nextDirectoryId = await value.resolveNextDirectory();
    scope.stop();

    // 第二页确实被请求过，说明失败点不在分页请求本身
    expect(
      server.requests.some(
        (request) =>
          request.operationName === "BrowseDirectories" && request.variables.after != null,
      ),
    ).toBe(true);

    expect(nextDirectoryId).toBe(directoryName(PAGE_SIZE));
  }, 30_000);

  test("对照：第一页内还有未达标目录时，进入同一页的下一项", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index === 0 } });

    const entry = await readListEntry(directoryName(0));
    enterDirectory(entry);

    const { scope, value } = createNavigator(directoryName(0));
    const nextDirectoryId = await value.resolveNextDirectory();
    scope.stop();

    expect(nextDirectoryId).toBe(directoryName(1));
  }, 30_000);

  // 定位缺陷位置：页锚点推算本身正确，问题只在「fetchMore 结果何时可见」的判断上。
  test("让出宏任务后，同一导航状态能算出 dir-100", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const { scope, value } = runInScope(() => useDirectoryBrowse(() => ({ id: PARENT_ID })));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    await value.fetchMore("forward");
    // 微任务阶段仍是旧数据：正是 resolveNextDirectory 放弃的时刻
    await nextTick();
    expect(value.directories.value.length).toBe(PAGE_SIZE);

    await settleBroadcast();
    expect(value.directories.value.length).toBe(PAGE_SIZE * 2);

    // 第二页首条的页锚点严格大于第一页首条，页序比较成立
    expect(
      value.directories.value[PAGE_SIZE].pageAnchor > value.directories.value[0].pageAnchor,
    ).toBe(true);
    scope.stop();

    const entry = await readListEntry(directoryName(0));
    enterDirectory(entry);

    const navigator = createNavigator(directoryName(0));
    // 缺陷：此时返回 undefined，CompletedView 因此回到主页
    expect(await navigator.value.resolveNextDirectory()).toBeUndefined();
    // 分页已经推进，只是合并结果在宏任务后才可见；可见后候选选择是正确的
    await settleBroadcast();
    expect(navigator.value.nextDirectoryId.value).toBe(directoryName(PAGE_SIZE));
    navigator.scope.stop();
  }, 30_000);
});
