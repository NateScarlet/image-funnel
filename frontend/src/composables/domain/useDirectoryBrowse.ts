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
  type BrowseDirectoriesQueryVariables,
  type DirectoryFragment,
  type DirectoryStatsFragment,
  type DirectoryStatsQuery,
} from "@/graphql/generated";
import { throttle, debounce } from "es-toolkit";
import { onScopeDispose, shallowReactive } from "vue";
import toStableValue from "@/utils/toStableValue";

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
    fetchMore: directoryConnection.fetchMore,
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

  async function refetchStats(directoryIds: string[], signal?: AbortSignal): Promise<void> {
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
      if (signal?.aborted) return;
      const batch = queue.splice(0, 5);

      await Promise.allSettled(
        batch.map(async (id) => {
          try {
            await client.query({
              query: DirectoryStatsDocument,
              variables: { id },
              fetchPolicy: "network-only",
              context: { transport: "batch-http:direcotry-stats" },
            });
          } finally {
            loadingStates[id] = (loadingStates[id] || 0) - 1;
          }
        }),
      );
    }
  }

  return { useStats, getCachedStats, refetchStats, isStatsLoading };
}

// #endregion
