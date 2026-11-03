// #region 说明
// 覆盖 #162：提交后会话统计变化，「下一个目录」预览与导航目标必须基于同一份刷新后的统计；
// 导航决策期间预览不得闪烁，最终展示的目标必须与实际跳转目标一致，决策失败后必须解除冻结。
//
// 接缝选在假 GraphQL 服务端（ApolloLink），其余全部走真实实现——useQuery、
// useRelayConnection、分页合并、useDirectoryStats 的响应式统计缓存、
// useSessionNextDirectory 的游标导航。
//
// 完成度只认已缓存的统计：用例先按「提交前」的统计写入缓存（此时预览停在 dir-001），
// 再让服务端在刷新时返回「提交后」的统计。统计刷新按每批 5 个目录串行执行，
// 用例用闸门挂起后续批次，从而在决策窗口内断言预览仍保持冻结，直到决策结束。
// #endregion

import { beforeEach, describe, expect, test, vi } from "vitest";
import { effectScope, nextTick, ref, watch, type EffectScope } from "vue";

// #region 假服务端装配
interface FakeOperation {
  operationName: string;
  variables: Record<string, unknown>;
}

/** 挂起的统计请求：由用例决定何时返回「提交后」的统计或失败 */
interface StatsGate {
  promise: Promise<unknown>;
  respond: () => void;
  fail: (error: Error) => void;
}

interface FakeServer {
  /** 服务端当前的完成度：提交后统计会在这里发生变化 */
  completed: Set<number>;
  /** 让指定目录的统计请求失败 */
  failingStats: Set<number>;
  /** 需要挂起的统计请求：按目录下标索引 */
  statsGates?: Map<number, StatsGate>;
  handle?: (operation: FakeOperation) => unknown;
  requests: { operationName: string; variables: Record<string, unknown> }[];
}

const server = vi.hoisted<FakeServer>(() => ({
  completed: new Set(),
  failingStats: new Set(),
  statsGates: undefined,
  handle: undefined,
  requests: [],
}));

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
          // 模拟统计/分页请求失败
          queueMicrotask(() => observer.error(err));
          return () => {};
        }
        // 订阅：不推送任何事件，模拟「期间没有实时变更」
        if (data === null) return () => {};
        if (data instanceof Promise) {
          // 统计请求闸门：响应时机由用例控制，用来制造稳定的决策窗口
          let cancelled = false;
          data.then(
            (response) => {
              if (cancelled) return;
              queueMicrotask(() => {
                observer.next({ data: response });
                observer.complete();
              });
            },
            (err) => {
              if (cancelled) return;
              queueMicrotask(() => observer.error(err));
            },
          );
          return () => {
            cancelled = true;
          };
        }
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
    defaultOptions: { watchQuery: { fetchPolicy: "cache-and-network" } },
  });
  return { default: client, client };
});
// #endregion

import { client } from "@/graphql/client";
import {
  DirectoryStatsDocument,
  type DirectoryFragment,
  type DirectoryStatsFragment,
} from "@/graphql/generated";
import useSessionNextDirectory, { type SessionDirectory } from "./useSessionNextDirectory";
import { directoryCursorKey } from "./useDirectoryCursor";
import { createDirectorySortKey } from "@/utils/directoryOrder";

// #region 假数据：单页目录，当前目录为 dir-000
const PARENT_ID = "parent-1";
const PAGE_SIZE = 100;

function directoryName(index: number): string {
  return `dir-${String(index).padStart(3, "0")}`;
}

/** 服务端游标：与 Go 端 TextToCursor 一致，十六进制编码，字典序等于目录名顺序 */
function toCursor(name: string): string {
  return Array.from(new TextEncoder().encode(name), (b) => b.toString(16).padStart(2, "0")).join(
    "",
  );
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

/** 无默认设置时：恰好 1 张未评级图片且无子目录视为达标，多张视为未达标 */
function statsDto(index: number, completed: Set<number>): DirectoryStatsFragment {
  const imageCount = completed.has(index) ? 1 : 2;
  return {
    __typename: "DirectoryStats",
    imageCount,
    subdirectoryCount: 0,
    latestImage: null,
    ratingCounts: [{ __typename: "RatingCount", rating: 0, count: imageCount }],
  };
}

/** 统计请求的响应体：按当前服务端完成度计算 */
function statsResponse(index: number) {
  return {
    node: {
      ...directoryDto(index),
      __typename: "Directory",
      stats: statsDto(index, server.completed),
    },
  };
}

/** 按 Go 端 ByTextV2 语义分页：游标 = 目录名，after 为严格大于的起点，first 为页大小 */
function browseDirectoriesPage(variables: Record<string, unknown>, directoryCount: number) {
  const first = (variables.first as number | undefined) ?? PAGE_SIZE;
  const after = variables.after as string | undefined;

  const all = Array.from({ length: directoryCount }, (_, index) => ({
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

function installServer(directoryCount: number) {
  server.handle = (operation) => {
    switch (operation.operationName) {
      case "BrowseDirectories":
        return browseDirectoriesPage(operation.variables, directoryCount);
      case "DirectoryStats": {
        const index = directoryIndexById(String(operation.variables.id));
        if (server.failingStats.has(index)) throw new Error("目录统计请求失败");
        const gate = server.statsGates?.get(index);
        if (gate) return gate.promise;
        return statsResponse(index);
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

// #region 用例辅助
function runInScope<T>(fn: () => T): { scope: EffectScope; value: T } {
  const scope = effectScope();
  const value = scope.run(fn);
  if (value === undefined) throw new Error("effectScope 未返回结果");
  return { scope, value };
}

/** 按目录名顺序预先把统计写入缓存：完成度判定只读已缓存统计 */
function seedStats(completed: Set<number>, directoryCount: number): void {
  for (let index = 0; index < directoryCount; index += 1) {
    client.writeQuery({
      query: DirectoryStatsDocument,
      variables: { id: directoryName(index) },
      data: {
        __typename: "Query",
        node: { ...directoryDto(index), stats: statsDto(index, completed) },
      },
    });
  }
}

/** 模拟从筛选列表进入当前目录：游标使用列表渲染该目录时的排序键 */
function enterDirectory(index: number, stats: DirectoryStatsFragment): void {
  const dir = directoryDto(index);
  const sortKey = createDirectorySortKey(
    {
      id: dir.id,
      // 单页：页锚点就是首条目录的服务端游标
      pageAnchor: toCursor(directoryName(0)),
      serverCursor: toCursor(directoryName(index)),
      stats,
      isForcedShown: false,
    },
    "oldest-first",
  );
  sessionStorage.setItem(
    directoryCursorKey(dir.id),
    JSON.stringify({ after: null, sortKey, query: null }),
  );
}

function createNavigator(index: number) {
  return runInScope(() => useSessionNextDirectory("session-1", () => directoryDto(index)));
}

/** 挂起指定目录的统计请求，用来在决策窗口内观察预览是否被冻结 */
function holdStats(indexes: number[]): Map<number, StatsGate> {
  const gates = new Map<number, StatsGate>();
  for (const index of indexes) {
    let respond!: () => void;
    let fail!: (error: Error) => void;
    const promise = new Promise<unknown>((resolve, reject) => {
      respond = () => resolve(statsResponse(index));
      fail = (error) => reject(error);
    });
    gates.set(index, { promise, respond, fail });
  }
  server.statsGates = gates;
  return gates;
}

/** 放行全部挂起的统计请求；对已结算的闸门重复调用无副作用 */
function releaseAll(gates: Map<number, StatsGate>): void {
  for (const gate of gates.values()) gate.respond();
}
// #endregion

beforeEach(async () => {
  server.handle = undefined;
  server.requests = [];
  server.completed = new Set();
  server.failingStats = new Set();
  server.statsGates = undefined;
  sessionStorage.clear();
  localStorage.clear();
  await client.clearStore();
});

describe("完成视图的导航目标与预览快照（真实统计接缝）", () => {
  // #162 回归：提交后统计变化，导航必须按刷新后的统计重新判定；
  // 决策窗口内预览保持冻结，结束后一次性切到判定结果。
  test("提交后统计变化时，导航目标与最终预览一致，且决策窗口内预览保持冻结", async () => {
    const directoryCount = 14;
    installServer(directoryCount);
    seedStats(new Set(), directoryCount);
    enterDirectory(0, statsDto(0, new Set()));
    // 提交后服务端统计：dir-001..dir-012 已达标，只有 dir-013 仍需筛选
    server.completed = new Set(Array.from({ length: 12 }, (_, index) => index + 1));
    // 第一批（dir-000..dir-004）立即返回；后续批次挂起，让决策停留在稳定窗口内
    const gates = holdStats(Array.from({ length: 9 }, (_, index) => index + 5));

    const { scope, value } = createNavigator(0);
    const previewChanges: (string | undefined)[] = [];
    let stop: (() => void) | undefined;
    try {
      await vi.waitFor(() => expect(value.nextDirectoryId.value).toBe(directoryName(1)), {
        timeout: 20_000,
        interval: 50,
      });

      stop = watch(value.nextDirectoryId, (id) => previewChanges.push(id));

      const pending = value.resolveNextDirectory();
      // 第一批统计已写入响应式缓存：实时候选变成 dir-005，说明决策确实在推进
      await vi.waitFor(() => expect(value.nextDirectory.value?.dir.id).toBe(directoryName(5)), {
        timeout: 20_000,
        interval: 50,
      });
      await nextTick();

      // 决策窗口内预览保持冻结：没有切到中间候选，也没有卡片消失
      expect(previewChanges).toEqual([]);
      expect(value.nextDirectoryId.value).toBe(directoryName(1));

      releaseAll(gates);
      const target = await pending;
      await nextTick();
      stop();

      // 决策结束后一次性切到判定结果，且最终展示的目标就是跳转目标
      expect(previewChanges).toEqual([directoryName(13)]);
      expect(target).toBe(directoryName(13));
      expect(value.nextDirectoryId.value).toBe(target);
    } finally {
      releaseAll(gates);
      stop?.();
      scope.stop();
    }
  }, 30_000);

  // 分页/统计请求失败不能被解释成「没有下一个目录」：
  // 缓存中的旧统计恰好显示序列已耗尽，若吞掉刷新失败就会直接回首页。
  test("统计刷新失败时向上抛出，而不是把序列当成已耗尽", async () => {
    installServer(4);
    // 缓存里除当前目录外都已达标
    seedStats(new Set([1, 2, 3]), 4);
    enterDirectory(0, statsDto(0, new Set()));
    server.failingStats = new Set([0, 1, 2, 3]);

    const { scope, value } = createNavigator(0);
    try {
      await expect(value.resolveNextDirectory()).rejects.toThrow("目录统计请求失败");
    } finally {
      scope.stop();
    }
  }, 30_000);

  // 决策失败不是「预览永远停在旧值」：必须解除冻结，回到实时候选，用户可在提示后重试。
  test("决策失败后预览解除冻结，回到实时候选", async () => {
    const directoryCount = 6;
    installServer(directoryCount);
    seedStats(new Set(), directoryCount);
    enterDirectory(0, statsDto(0, new Set()));
    // 提交后：第一批里的 dir-001..dir-004 已达标，dir-005 仍需筛选
    server.completed = new Set([1, 2, 3, 4]);
    // 只挂起第二批（dir-005）的统计请求
    const gates = holdStats([5]);

    const { scope, value } = createNavigator(0);
    const previewChanges: (string | undefined)[] = [];
    let stop: (() => void) | undefined;
    try {
      await vi.waitFor(() => expect(value.nextDirectoryId.value).toBe(directoryName(1)), {
        timeout: 20_000,
        interval: 50,
      });
      stop = watch(value.nextDirectoryId, (id) => previewChanges.push(id));

      const pending = value.resolveNextDirectory();
      // 第一批已刷新：实时候选变成 dir-005，但预览仍冻结在决策开始时的 dir-001
      await vi.waitFor(() => expect(value.nextDirectory.value?.dir.id).toBe(directoryName(5)), {
        timeout: 20_000,
        interval: 50,
      });
      await nextTick();
      expect(value.nextDirectoryId.value).toBe(directoryName(1));

      // 等第二批统计请求真正发出后再让它失败
      await vi.waitFor(
        () =>
          expect(
            server.requests.some(
              (request) =>
                request.operationName === "DirectoryStats" &&
                request.variables.id === directoryName(5),
            ),
          ).toBe(true),
        { timeout: 20_000, interval: 50 },
      );
      const gate = gates.get(5);
      if (!gate) throw new Error("缺少 dir-005 的统计闸门");
      gate.fail(new Error("目录统计请求失败"));

      await expect(pending).rejects.toThrow("目录统计请求失败");
      await nextTick();

      // 冻结已解除：预览回到第一批统计后的实时候选，而不是停留在旧目标
      expect(previewChanges).toEqual([directoryName(5)]);
      expect(value.nextDirectoryId.value).toBe(directoryName(5));
    } finally {
      stop?.();
      scope.stop();
    }
  }, 30_000);

  // 连续会话回归（SessionView 在路由参数变化时复用，同一 composable 实例跨会话存活）：
  // A 会话完成导航时把预览冻结到即将进入的 B，进入 B 后这份冻结必须失效，
  // 否则 B 的完成页会显示「下一个目录 = B 自己」，并在决策窗口内保持旧目标后才跳到真实候选。
  test("连续会话切换后，上一会话冻结的预览不再可见", async () => {
    const directoryCount = 14;
    installServer(directoryCount);
    seedStats(new Set(), directoryCount);
    enterDirectory(0, statsDto(0, new Set()));

    const sessionId = ref("session-1");
    const currentDirectory = ref<SessionDirectory>(directoryDto(0));
    const { scope, value } = runInScope(() => useSessionNextDirectory(sessionId, currentDirectory));
    const previewChanges: (string | undefined)[] = [];
    let stop: (() => void) | undefined;
    try {
      // A 会话完成：预览冻结到 B（dir-001），并实际跳转进入 B
      await vi.waitFor(() => expect(value.nextDirectoryId.value).toBe(directoryName(1)), {
        timeout: 20_000,
        interval: 50,
      });
      expect(await value.resolveNextDirectory()).toBe(directoryName(1));

      // 进入 B：会话 ID 与当前目录原子切换（复用实例，不重建 composable）
      sessionId.value = "session-2";
      currentDirectory.value = directoryDto(1);
      // B 提交后服务端统计：dir-002..dir-012 已达标，只剩 dir-013 仍需筛选
      server.completed = new Set(Array.from({ length: 11 }, (_, index) => index + 2));

      // B 的预览从它自己的候选重新开始，而不是沿用 A 留下的 dir-001
      await vi.waitFor(() => expect(value.nextDirectoryId.value).toBe(directoryName(2)), {
        timeout: 20_000,
        interval: 50,
      });
      stop = watch(value.nextDirectoryId, (id) => previewChanges.push(id));

      // 第一批（dir-000..dir-004）立即返回；后续批次挂起，让决策停留在稳定窗口内
      const gates = holdStats(Array.from({ length: 9 }, (_, index) => index + 5));
      const pending = value.resolveNextDirectory();
      // 第一批统计已写入响应式缓存：B 的实时候选变成 dir-005，说明决策确实在推进
      await vi.waitFor(() => expect(value.nextDirectory.value?.dir.id).toBe(directoryName(5)), {
        timeout: 20_000,
        interval: 50,
      });
      await nextTick();

      // 决策窗口内冻结的是 B 自己的候选，而不是上一会话遗留的目标
      expect(previewChanges).toEqual([]);
      expect(value.nextDirectoryId.value).toBe(directoryName(2));

      releaseAll(gates);
      const target = await pending;
      await nextTick();
      stop();

      // 决策结束后一次性切到判定结果，且最终展示的目标就是跳转目标
      expect(target).toBe(directoryName(13));
      expect(value.nextDirectoryId.value).toBe(target);
      expect(previewChanges).toEqual([directoryName(13)]);
    } finally {
      stop?.();
      scope.stop();
    }
  }, 30_000);
});
