import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useRelayConnection from "@/composables/useRelayConnection";
import useLiveConnection from "@/composables/useLiveConnection";
import client from "@/graphql/client";
import {
  BrowseDirectoriesDocument,
  DirectoryChangedDocument,
  DirEntryDeletedDocument,
  DirectoryFragmentDoc,
  DirectoryStatsDocument,
  type BrowseDirectoriesQuery,
  type BrowseDirectoriesQueryVariables,
  type DirectoryFragment,
  type DirectoryStatsFragment,
  type DirectoryStatsQuery,
} from "@/graphql/generated";
import { isEqual, throttle, debounce } from "es-toolkit";
import { onScopeDispose, shallowReactive, watch } from "vue";
import toStableValue from "@/utils/toStableValue";
import type { RelayConnection } from "@/utils/isRelayConnection";
import { DirectoryPageWaitCancelledError } from "@/utils/directoryPageWaitCancelled";

/** 已加载目录及其位置：排序键所需的页锚点、页请求锚点与目录自身游标 */
export interface PositionedDirectory {
  dir: DirectoryFragment;
  /** 页锚点：该目录所在页首条目录的服务端游标，其字符串序等于页序 */
  pageAnchor: string;
  /** 该页的 forward 请求锚点（首页为 null） */
  pageAfter: string | null;
  /** 目录自身的服务端游标，用作排序键的最后一级 */
  serverCursor: string;
}

export default function useDirectoryBrowse(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables | undefined>,
  options?: { loadingCount?: Ref<number> },
) {
  const directoryId = computed(() => toValue(variables)?.id);
  const loadingCount = options?.loadingCount;

  const DEFAULT_PAGE_SIZE = 100;

  // schema 不再为 first 提供默认值：正向取页必须显式给出页大小，反向取页则不带 first
  const queryVariables = computed<BrowseDirectoriesQueryVariables | undefined>(() => {
    const value = toValue(variables);
    if (!value || value.last != null) return value;
    return { ...value, first: value.first ?? DEFAULT_PAGE_SIZE };
  });

  const { data: directoriesData, query: directoriesQuery } = useQuery(BrowseDirectoriesDocument, {
    variables: queryVariables,
    loadingCount,
  });

  const currentDirectory = computed(() => {
    const node = directoriesData.value?.node;
    return node?.__typename === "Directory" ? node : undefined;
  });

  // 原始分页边：保留服务端顺序与每条边的游标，供上层计算页锚点
  const directoryEdges = computed(() => currentDirectory.value?.directoriesV2.edges ?? []);

  const directoryConnection = useRelayConnection(
    () =>
      directoriesData.value?.node?.__typename === "Directory"
        ? directoriesData.value.node.directoriesV2
        : undefined,
    () => directoriesQuery,
    {
      // before 需与 last 搭配，页大小沿用查询的 first
      backwardPageSize: () => queryVariables.value?.first ?? DEFAULT_PAGE_SIZE,
    },
  );

  const {
    nodes: liveDirectories,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => directoryConnection.nodes.value, {
    identity: (d: DirectoryFragment) => d.relPath,
    compare: (a: DirectoryFragment, b: DirectoryFragment) => a.relPath.localeCompare(b.relPath),
    subscribe: (item, callback) => {
      const observable = client.watchFragment<DirectoryFragment>({
        fragment: DirectoryFragmentDoc,
        fragmentName: "Directory",
        from: item,
      });
      const sub = observable.subscribe((result) => {
        if (result.complete && result.data) {
          callback(result.data);
        }
      });
      return () => sub.unsubscribe();
    },
  });

  useSubscription(DirectoryChangedDocument, {
    variables: () => (directoryId.value ? { id: [directoryId.value] } : undefined),
    onNext: (result) => {
      const savedDir = result.data?.directoryChanged;
      if (savedDir) {
        client.writeFragment({
          id: client.cache.identify(savedDir),
          fragment: DirectoryFragmentDoc,
          fragmentName: "Directory",
          data: savedDir,
        });
        if (savedDir.parentId === directoryId.value) {
          onSaved(savedDir);
        }
      }
    },
  });

  const pendingRelPathDeletion = new Set<string>();
  function doFlushRelPathDeletion() {
    if (pendingRelPathDeletion.size === 0) return;
    for (const d of liveDirectories.value) {
      if (pendingRelPathDeletion.has(d.relPath)) {
        onDeleted(d);
      }
    }
    pendingRelPathDeletion.clear();
  }
  const flushRelPathDeletion = throttle(doFlushRelPathDeletion, 1e3, {
    edges: ["leading", "trailing"],
  });

  useSubscription(DirEntryDeletedDocument, {
    variables: () => (directoryId.value ? { directoryId: directoryId.value } : undefined),
    onNext: (result) => {
      const deletedEntries = result.data?.dirEntryDeleted;
      if (deletedEntries && deletedEntries.length > 0) {
        for (const entry of deletedEntries) {
          pendingRelPathDeletion.add(entry.relPath);
        }
        flushRelPathDeletion();
      }
    },
  });

  // #region 目录位置
  // 每次取页的页大小固定（查询的 first，默认 100），已加载的边因此按页大小连续分页：
  // 页锚点就是该页首条边的游标，不需要额外记录页边界。
  const pageSize = computed(() => queryVariables.value?.first ?? DEFAULT_PAGE_SIZE);

  /** 第 index 条边所在页的首条边下标 */
  function pageStartAt(index: number): number {
    return Math.floor(index / pageSize.value) * pageSize.value;
  }

  /** 第 index 条边所在页的页锚点：该页首条边的游标 */
  function pageAnchorAt(index: number): string | undefined {
    return directoryEdges.value[pageStartAt(index)]?.cursor;
  }

  /** 第 index 条边所在页的 forward 请求锚点 */
  function pageAfterAt(index: number): string | null {
    const start = pageStartAt(index);
    if (start === 0) return queryVariables.value?.after ?? null;
    return directoryEdges.value[start - 1]?.cursor ?? null;
  }

  /** 已加载目录及其位置，按服务端顺序排列；不在 edges 中的实时新增目录没有位置 */
  const directories = computed<PositionedDirectory[]>(() => {
    const liveById = new Map(liveDirectories.value.map((dir) => [dir.id, dir]));

    return directoryEdges.value.flatMap((edge, index) => {
      const dir = liveById.get(edge.node.id);
      const pageAnchor = pageAnchorAt(index);
      if (!dir || pageAnchor === undefined) return [];

      return [{ dir, pageAnchor, pageAfter: pageAfterAt(index), serverCursor: edge.cursor }];
    });
  });
  // #endregion

  // #region 取页结果可见性
  // fetchMore 的 promise 在合并结果写入缓存之后、ObservableQuery 广播之前 resolve，
  // 此刻响应式数据仍是旧页。消费者若在取页返回时立刻读取已加载目录，
  // 会把「结果尚未广播」误判成「分页没有推进」。
  // 因此以缓存中已合并的连接为目标，等响应式数据追平后才让 fetchMore 返回。

  /** 进行中的取页等待的结束函数；等待期间作用域销毁或变量切换必须把它们结束掉 */
  const pendingPageWaitEnds = new Set<(reason?: Error) => void>();
  /** 查询变量每切换一次就失效一批等待：旧的合并目标属于旧变量，不再适用 */
  let pageWaitVariablesVersion = 0;
  /** 作用域已销毁：此后不再依赖广播，也不应继续等待 */
  let pageScopeDisposed = false;

  function endPendingPageWaits(reason?: Error): void {
    for (const end of pendingPageWaitEnds) end(reason);
    pendingPageWaitEnds.clear();
  }

  onScopeDispose(() => {
    pageScopeDisposed = true;
    // 作用域销毁后没有消费者，结束等待而不是让 promise 永久悬挂
    endPendingPageWaits(new DirectoryPageWaitCancelledError("目录取页等待已取消：作用域已销毁"));
  });

  // 变量切换后，旧变量的合并目标永远不会出现在新数据里，必须结束等待
  watch(queryVariables, () => {
    pageWaitVariablesVersion += 1;
    endPendingPageWaits(new DirectoryPageWaitCancelledError("目录取页等待已取消：查询变量已切换"));
  });

  /** 读取缓存中当前查询的连接；fetchMore 返回时缓存已包含合并后的页 */
  function readMergedConnection(): RelayConnection<DirectoryFragment> | undefined {
    const data = client.readQuery<BrowseDirectoriesQuery>({
      query: BrowseDirectoriesDocument,
      variables: directoriesQuery.variables as BrowseDirectoriesQueryVariables,
    });
    const node = data?.node;
    return node?.__typename === "Directory" ? node.directoriesV2 : undefined;
  }

  /** 已合并的连接是否已经在响应式数据中可见：边数与页边界必须一致 */
  function isMergedConnectionVisible(expected: RelayConnection<DirectoryFragment>): boolean {
    const visible = currentDirectory.value?.directoriesV2;
    if (!visible) return false;

    return (
      (expected.edges?.length ?? 0) === (visible.edges?.length ?? 0) &&
      isEqual(expected.pageInfo ?? null, visible.pageInfo ?? null)
    );
  }

  /** 取一页，并在合并结果对消费者可见后返回 */
  async function fetchVisiblePage(direction?: "forward" | "backward"): Promise<void> {
    // 作用域销毁后没有消费者，也不应再发起请求
    if (pageScopeDisposed)
      throw new DirectoryPageWaitCancelledError("目录取页等待已取消：作用域已销毁");

    const variablesVersion = pageWaitVariablesVersion;
    await directoryConnection.fetchMore(direction);

    // 取页期间变量切换：本次合并结果属于旧变量，不能报告为「新页可见」
    if (variablesVersion !== pageWaitVariablesVersion) {
      throw new DirectoryPageWaitCancelledError("目录取页等待已取消：查询变量已切换");
    }
    // 取页期间作用域销毁：同样属于取消，等待目标已无人消费
    if (pageScopeDisposed) {
      throw new DirectoryPageWaitCancelledError("目录取页等待已取消：作用域已销毁");
    }

    const merged = readMergedConnection();
    // 缓存读不到合并结果说明无法判断新页是否可见，不能按旧页静默返回
    if (!merged) throw new Error("目录分页的合并结果无法从缓存读取");
    if (isMergedConnectionVisible(merged)) return;

    // 等 Apollo 广播把合并结果推送到响应式数据；不依赖固定延时
    await new Promise<void>((resolve, reject) => {
      let settled = false;
      function finish(reason?: Error): void {
        if (settled) return;
        settled = true;
        stopWatch();
        pendingPageWaitEnds.delete(cancel);
        if (reason) reject(reason);
        else resolve();
      }
      const cancel = (reason?: Error) => finish(reason);
      const stopWatch = watch(
        () => currentDirectory.value?.directoriesV2,
        () => {
          if (isMergedConnectionVisible(merged)) finish();
        },
      );
      pendingPageWaitEnds.add(cancel);
      // 注册后再复查一次：广播可能恰好在创建 watcher 之前落地
      if (isMergedConnectionVisible(merged)) finish();
    });
  }
  // #endregion

  return {
    currentDirectory,
    liveDirectories,
    directories,
    edges: directoryEdges,
    query: directoriesQuery,
    data: directoriesData,
    pageInfo: directoryConnection.pageInfo,
    hasNextPage: computed(() => directoryConnection.pageInfo.value.hasNextPage),
    hasPreviousPage: computed(() => directoryConnection.pageInfo.value.hasPreviousPage),
    fetchMore: fetchVisiblePage,
  };
}

// #region 目录统计信息

const loadingStates = shallowReactive<Record<string, number>>({});

function useStats(directoryId: MaybeRefOrGetter<string>, loadingCount?: Ref<number>) {
  const { data, refresh } = useQuery(DirectoryStatsDocument, {
    variables: () => ({ id: toValue(directoryId) }),
    loadingCount,
    fetchPolicy: "cache-first",
    context: { transport: "batch-http:direcotry-stats" },
  });

  const debouncedRefetch = debounce(() => {
    void refresh();
  }, 1000);

  useSubscription(DirectoryChangedDocument, {
    variables: () => ({ id: [toValue(directoryId)] }),
    onNext: (result) => {
      const changedId = result.data?.directoryChanged.id;
      if (changedId === toValue(directoryId)) {
        debouncedRefetch();
      }
    },
  });

  return data;
}

export function useDirectoryStats() {
  function isStatsLoading(directoryId: string): boolean {
    return (loadingStates[directoryId] || 0) > 0;
  }

  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose());

  const statsCache = shallowReactive(new Map<string, DirectoryStatsFragment | undefined>());

  function getCachedStats(directoryId: string): DirectoryStatsFragment | undefined {
    if (!statsCache.has(directoryId)) {
      const initialNode = client.readQuery({
        query: DirectoryStatsDocument,
        variables: { id: directoryId },
      })?.node;
      // eslint-disable-next-line no-underscore-dangle
      const initial = initialNode?.__typename === "Directory" ? initialNode.stats : undefined;
      statsCache.set(directoryId, initial || undefined);

      stack.adopt(
        client
          .watchQuery({
            query: DirectoryStatsDocument,
            variables: { id: directoryId },
            fetchPolicy: "cache-only",
          })
          .subscribe((result) => {
            const node = (result.data as unknown as DirectoryStatsQuery)?.node;
            statsCache.set(
              directoryId,
              toStableValue(
                (node?.__typename === "Directory" ? node.stats : undefined) || undefined,
                statsCache.get(directoryId),
              ),
            );
          }),
        (i) => i.unsubscribe(),
      );
    }
    return statsCache.get(directoryId);
  }

  /**
   * 拉取单个目录的最新统计，并同步进响应式统计缓存。
   *
   * 必须先调用 getCachedStats 建立缓存订阅，否则后续来自订阅的统计变化不会进入 statsCache。
   * 网络结果直接写入缓存：缓存广播是异步的，只 await 请求本身会让消费者在返回后仍读到刷新前的统计。
   */
  async function refreshStats(directoryId: string): Promise<void> {
    getCachedStats(directoryId);

    const { data } = await client.query({
      query: DirectoryStatsDocument,
      variables: { id: directoryId },
      fetchPolicy: "network-only",
      context: { transport: "batch-http:direcotry-stats" },
    });

    const node = data?.node;
    const fresh = node?.__typename === "Directory" ? node.stats : undefined;
    statsCache.set(directoryId, toStableValue(fresh || undefined, statsCache.get(directoryId)));
  }

  /**
   * 刷新指定目录的统计，返回未能刷新的原因。
   *
   * 失败不在这里吞掉：调用方需要把失败与「目录已达标」区分开
   * （完成视图的自动切换既不能把失败当成未完成，也不能当成没有下一个目录）。
   */
  async function refetchStats(directoryIds: string[], signal?: AbortSignal): Promise<unknown[]> {
    const failures: unknown[] = [];

    directoryIds.forEach((id) => {
      loadingStates[id] = (loadingStates[id] || 0) + 1;
    });

    using batchStack = new DisposableStack();
    const queue = batchStack.adopt([...directoryIds], (q) => {
      q.forEach((id) => {
        loadingStates[id] = (loadingStates[id] || 0) - 1;
      });
    });

    while (queue.length > 0) {
      if (signal?.aborted) return failures;
      const batch = queue.splice(0, 5);

      const settled = await Promise.allSettled(
        batch.map(async (id) => {
          try {
            await refreshStats(id);
          } finally {
            loadingStates[id] = (loadingStates[id] || 0) - 1;
          }
        }),
      );
      for (const result of settled) {
        if (result.status === "rejected") failures.push(result.reason);
      }
    }

    return failures;
  }

  return { useStats, getCachedStats, refetchStats, isStatsLoading };
}

// #endregion
