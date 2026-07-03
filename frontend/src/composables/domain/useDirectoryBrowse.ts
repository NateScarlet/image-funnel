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

export default function useDirectoryBrowse(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  const directoryId = computed(() => toValue(variables).id);
  const loadingCount = options?.loadingCount;

  const { data: directoriesData, query: directoriesQuery } = useQuery(BrowseDirectoriesDocument, {
    variables,
    loadingCount,
  });

  const currentDirectory = computed(() => {
    const node = directoriesData.value?.node;
    return node?.__typename === "Directory" ? node : undefined;
  });

  const directoryConnection = useRelayConnection(
    () =>
      directoriesData.value?.node?.__typename === "Directory"
        ? directoriesData.value.node.directoriesV2
        : undefined,
    () => directoriesQuery,
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
    variables: () => ({
      id: directoryId.value ? [directoryId.value] : undefined,
    }),
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
    variables: () => ({ directoryId: directoryId.value }),
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

  return {
    currentDirectory,
    liveDirectories,
    query: directoriesQuery,
    data: directoriesData,
    hasNextPage: computed(() => directoryConnection.pageInfo.value.hasNextPage),
    fetchMore: directoryConnection.fetchMore,
  };
}

// #region 目录统计信息

const loadingStates = shallowReactive<Record<string, number>>({});

export function useDirectoryStats() {
  function isStatsLoading(directoryId: string): boolean {
    return (loadingStates[directoryId] || 0) > 0;
  }

  function useStats(directoryId: MaybeRefOrGetter<string>, loadingCount?: Ref<number>) {
    const { data, refresh } = useQuery(DirectoryStatsDocument, {
      variables: () => ({ id: toValue(directoryId) }),
      loadingCount,
      fetchPolicy: "cache-first",
      context: { transport: "batch-http:direcotry-stats" },
    });

    const debouncedRefetch = debounce(() => {
      refresh();
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

  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose());

  const statsCache = shallowReactive(new Map<string, DirectoryStatsFragment | undefined>());

  function getCachedStats(directoryId: string): DirectoryStatsFragment | undefined {
    if (!statsCache.has(directoryId)) {
      const initialNode = client.readQuery({
        query: DirectoryStatsDocument,
        variables: { id: directoryId },
      })?.node;
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
            const node = (result.data as DirectoryStatsQuery)?.node;
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

    using stack = new DisposableStack();
    const queue = stack.adopt([...directoryIds], (q) => {
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
