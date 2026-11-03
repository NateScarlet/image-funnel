// #region 说明
// 覆盖「筛选完成后自动切换回主页、第二页的待筛选目录被跳过」(#170)。
//
// 接缝选在假 GraphQL 服务端（ApolloLink），其余全部走真实实现——useQuery、
// useRelayConnection、utils/fetchMore 的合并、useDirectoryBrowse 的页锚点推算与
// 取页可见性等待、useDirectoryStats 的缓存、useSessionNextDirectory 的游标导航。
// 此前的同类测试都 mock 掉 useDirectoryBrowse 并手工填写 pageAnchor/pageAfter，
// 页锚点推算与真实分页合并从未被执行，因此「测试通过、实测失败」无法被发现。
//
// 关键时序：`utils/fetchMore` 的 promise 在缓存合并之后、ObservableQuery 广播之前
// resolve（Apollo 4 在宏任务中才推送）。useDirectoryBrowse 对外暴露的 fetchMore
// 会等到合并结果在响应式数据中可见后才返回，因此取页返回即代表新页可读。
// #endregion

import { beforeEach, describe, expect, test, vi } from "vitest";
import { effectScope, ref, type EffectScope } from "vue";

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
        let data: unknown;
        try {
          data = handler({
            operationName,
            variables: operation.variables as Record<string, unknown>,
          });
        } catch (err) {
          // 模拟服务端/分页请求失败
          queueMicrotask(() => observer.error(err));
          return () => {};
        }
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
import {
  DirectoryStatsDocument,
  type BrowseDirectoriesQueryVariables,
  type DirectoryFragment,
  type DirectoryStatsFragment,
} from "@/graphql/generated";
import useDirectories from "./useDirectories";
import useDirectoryBrowse from "./domain/useDirectoryBrowse";
import useSessionNextDirectory from "./useSessionNextDirectory";
import { directoryCursorKey } from "./useDirectoryCursor";
import { isDirectoryPageWaitCancelled } from "@/utils/directoryPageWaitCancelled";
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

function statsDto(index: number, fixture: Fixture): DirectoryStatsFragment {
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

function installServer(options: { total: number; fixture: Fixture; failForwardPage?: boolean }) {
  server.handle = (operation) => {
    switch (operation.operationName) {
      case "BrowseDirectories":
        if (options.failForwardPage && operation.variables.after != null) {
          throw new Error("目录分页请求失败");
        }
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

/** 已发起的正向分页请求数，用于断言「没有多余请求」 */
function forwardRequestCount(): number {
  return server.requests.filter(
    (request) => request.operationName === "BrowseDirectories" && request.variables.after != null,
  ).length;
}

/** 预先把统计写入缓存：完成度判定只认已缓存的统计，避免用例依赖 refetchStats 的广播时机 */
function seedStats(indexes: number[], fixture: Fixture): void {
  for (const index of indexes) {
    client.writeQuery({
      query: DirectoryStatsDocument,
      variables: { id: directoryName(index) },
      data: {
        __typename: "Query",
        node: { ...directoryDto(index), stats: statsDto(index, fixture) },
      },
    });
  }
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
  // #170 回归：第二页有待筛选目录时，自动切换应进入它。
  test("第一页全部达标时，应继续取第二页并进入其中第一个未达标目录", async () => {
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

  // 取页可见性接缝：fetchMore 返回时新页已可读，消费无需再额外等待广播。
  test("fetchMore 返回时，合并后的下一页已在已加载目录中可见", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const { scope, value } = runInScope(() => useDirectoryBrowse(() => ({ id: PARENT_ID })));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    await value.fetchMore("forward");
    expect(value.directories.value.length).toBe(PAGE_SIZE * 2);

    // 第二页首条的页锚点严格大于第一页首条，页序比较成立
    expect(
      value.directories.value[PAGE_SIZE].pageAnchor > value.directories.value[0].pageAnchor,
    ).toBe(true);
    scope.stop();
  }, 30_000);

  // 序列耗尽：只有所有后续页都没有候选时才返回空（此时回主页才是正确行为）
  test("后续页都没有候选时，取完所有页后才返回空", async () => {
    const fixture: Fixture = { completed: () => true };
    installServer({ total: 250, fixture });
    seedStats(
      Array.from({ length: 250 }, (_, index) => index),
      fixture,
    );

    const entry = await readListEntry(directoryName(0));
    enterDirectory(entry);

    const { scope, value } = createNavigator(directoryName(0));
    const nextDirectoryId = await value.resolveNextDirectory();
    scope.stop();

    expect(nextDirectoryId).toBeUndefined();
    // 三页（100 + 100 + 50）都被取过，而不是在第一页无候选时就放弃
    expect(
      server.requests.filter(
        (request) =>
          request.operationName === "BrowseDirectories" && request.variables.after != null,
      ).length,
    ).toBeGreaterThanOrEqual(2);
  }, 30_000);

  // 真实错误必须向上暴露，不能当作「没有下一个目录」而回主页
  test("分页请求失败时向上抛出，而不是返回空", async () => {
    installServer({
      total: 250,
      fixture: { completed: (index) => index < PAGE_SIZE },
      failForwardPage: true,
    });

    const entry = await readListEntry(directoryName(0));
    enterDirectory(entry);

    const { scope, value } = createNavigator(directoryName(0));
    try {
      await expect(value.resolveNextDirectory()).rejects.toThrow("目录分页请求失败");
    } finally {
      scope.stop();
    }
  }, 30_000);

  // #region 取页等待的生命周期与读取失败
  // 作用域销毁后没有消费者：等待必须结束，不能因为广播不会再来而永久悬挂
  test("取页等待期间作用域销毁时，fetchMore 以取消错误结束而不是悬挂", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const { scope, value } = runInScope(() => useDirectoryBrowse(() => ({ id: PARENT_ID })));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    const pending = value.fetchMore("forward");
    scope.stop();

    await expect(pending.catch((error: unknown) => error)).resolves.toSatisfy(
      isDirectoryPageWaitCancelled,
    );
  }, 30_000);

  // 作用域销毁后不应再发起请求（卸载期间的滚动回调不能触发网络请求）
  test("作用域销毁后再调用 fetchMore 不会发起请求", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const { scope, value } = runInScope(() => useDirectoryBrowse(() => ({ id: PARENT_ID })));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    scope.stop();
    const requestsBefore = forwardRequestCount();

    await expect(value.fetchMore("forward").catch((error: unknown) => error)).resolves.toSatisfy(
      isDirectoryPageWaitCancelled,
    );
    expect(forwardRequestCount()).toBe(requestsBefore);
  }, 30_000);

  // 变量切换后旧变量的合并目标不会再出现在新数据里，等待必须以取消错误结束
  test("取页期间查询变量切换时，fetchMore 以取消错误结束", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const variables = ref<BrowseDirectoriesQueryVariables>({ id: PARENT_ID });
    const { scope, value } = runInScope(() => useDirectoryBrowse(variables));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    const pending = value.fetchMore("forward");
    // 变量引用切换即视为旧合并目标失效；内容保持相同，避免 Apollo 换用另一组缓存字段
    variables.value = { id: PARENT_ID };

    await expect(pending.catch((error: unknown) => error)).resolves.toSatisfy(
      isDirectoryPageWaitCancelled,
    );
    scope.stop();
  }, 30_000);

  // 列表侧（浏览视图 / 目录选择器 / 子目录网格）的取页取消必须静默结束，
  // 否则滚动回调会留下未处理的 Promise 拒绝，且与既有分页行为不一致
  test("目录列表在取页在途时切换变量，fetchMore 静默结束", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const variables = ref<BrowseDirectoriesQueryVariables>({ id: PARENT_ID });
    const { scope, value } = runInScope(() => useDirectories(variables));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    const pending = value.fetchMore("forward");
    variables.value = { id: PARENT_ID };

    await expect(pending).resolves.toBeUndefined();
    scope.stop();
  }, 30_000);

  // 缓存读不到合并结果时无法判断新页是否可见，必须暴露而不是按旧页静默返回
  test("缓存无法读取合并结果时，fetchMore 抛出而不是按旧页返回", async () => {
    installServer({ total: 250, fixture: { completed: (index) => index < PAGE_SIZE } });

    const { scope, value } = runInScope(() => useDirectoryBrowse(() => ({ id: PARENT_ID })));
    await vi.waitFor(() => {
      expect(value.directories.value.length).toBe(PAGE_SIZE);
    });

    const readQuery = vi.spyOn(client, "readQuery").mockImplementation(() => null);
    try {
      await expect(value.fetchMore("forward")).rejects.toThrow("无法从缓存读取");
    } finally {
      readQuery.mockRestore();
      scope.stop();
    }
  }, 30_000);
  // #endregion
});
