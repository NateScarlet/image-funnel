import { computed, ref, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import useAsyncTask from "@/composables/useAsyncTask";
import useDirectoryStats from "@/composables/useDirectoryStats";
import { sortBy } from "es-toolkit";
import {
  BrowseDirectoriesDocument,
  DirectoryChangedDocument,
  DirEntryDeletedDocument,
  DirectoryFragmentDoc,
  type BrowseDirectoriesQueryVariables,
  type DirectoryFragment,
} from "@/graphql/generated";
import client from "@/graphql/client";

/**
 * useDirectories 提供子目录的查询、本地排序与过滤、实时增量订阅及分页功能
 * @param variables 查询变量，支持响应式对象或其 Getter
 * @param options 可选配置，支持本地搜索 query 以及全局的 loadingCount
 */
export default function useDirectories(
  variables: MaybeRefOrGetter<BrowseDirectoriesQueryVariables>,
  options?: {
    loadingCount?: Ref<number>;
    query?: MaybeRefOrGetter<string | undefined>;
    maxUnratedCount?: MaybeRefOrGetter<number | undefined>;
  },
) {
  const resolvedVariables = computed(() => {
    const vars = toValue(variables);
    const queryVal = options?.query
      ? toValue(options.query)?.trim()
      : undefined;
    return {
      ...vars,
      filterBy: {
        ...vars.filterBy,
        query: queryVal || undefined,
      },
    };
  });
  const directoryId = computed(() => toValue(variables).id);

  const maxUnratedCountVal = computed(() => toValue(options?.maxUnratedCount));

  const { getCachedStats, refetchStats } = useDirectoryStats();
  const internalLoadingCount = ref(0);
  const loadingCount = options?.loadingCount ?? internalLoadingCount;

  // 执行 GraphQL 查询获取目录的分页数据
  const { data: directoriesData, query: directoriesQuery } = useQuery(
    BrowseDirectoriesDocument,
    {
      variables: () => resolvedVariables.value,
      loadingCount,
    },
  );

  // 获取当前目录自身节点信息
  const currentDirectory = computed(() => {
    const node = directoriesData.value?.node;
    return node?.__typename === "Directory" ? node : undefined;
  });

  // 管理 Relay 格式的子连接
  const directoryConnection = useRelayConnection(
    () =>
      directoriesData.value?.node?.__typename === "Directory"
        ? directoriesData.value.node.directoriesV2
        : undefined,
    () => directoriesQuery,
  );

  // 接入 live connection 机制进行实时缓存操作
  const {
    nodes: liveDirectories,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => directoryConnection.nodes.value, {
    identity: (d) => d.relPath,
    compare: (a, b) => {
      return a.relPath.localeCompare(b.relPath);
    },
    subscribe: (item, callback) => {
      const observable = client.watchFragment<DirectoryFragment>({
        fragment: ImageFunnelDirectoryFragmentDoc,
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

  // 订阅目录变更事件
  useSubscription(DirectoryChangedDocument, {
    variables: () => {
      return {
        id: directoryId.value ? [directoryId.value] : undefined,
      };
    },
    onNext: (result) => {
      const savedDir = result.data?.directoryChanged;
      if (savedDir) {
        client.writeFragment({
          id: client.cache.identify(savedDir),
          fragment: ImageFunnelDirectoryFragmentDoc,
          fragmentName: "Directory",
          data: savedDir,
        });
        // 只有当变更的目录是当前目录的子目录时才加入/更新到列表中
        if (savedDir.parentId === directoryId.value) {
          onSaved(savedDir);
        }
      }
    },
  });

  // 订阅目录项删除事件
  useSubscription(DirEntryDeletedDocument, {
    variables: () => {
      return { directoryId: directoryId.value || undefined };
    },
    onNext: (result) => {
      const deletedEntry = result.data?.dirEntryDeleted;
      if (deletedEntry) {
        const match = liveDirectories.value.find(
          (d) => d.relPath === deletedEntry.relPath,
        );
        if (match) {
          onDeleted({ id: match.id });
        }
      }
    },
  });

  // 在后台批量加载未获取统计信息的目录，避免同时发起大量查询
  useAsyncTask({
    loadingCount,
    args() {
      const dirs = liveDirectories.value;
      const toLoad = dirs.map((d) => d.id);
      return toLoad.length > 0 ? [toLoad] : undefined;
    },
    async task(toLoad, ctx) {
      await refetchStats(toLoad, ctx.signal());
    },
  });

  // 计算未评级图片多于设定阈值的目录数量
  const largeUnratedCount = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;
    if (limit === undefined) return 0;
    return dirs.filter((dir) => {
      const stats = getCachedStats(dir.id);
      if (!stats) return false;
      const unratedCount =
        stats.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      return stats.subdirectoryCount === 0 && unratedCount > limit;
    }).length;
  });

  // 排序并过滤后的目录列表
  const sortedDirectories = computed(() => {
    const dirs = liveDirectories.value;
    const limit = maxUnratedCountVal.value;

    const items = dirs.map((dir) => {
      const stats = getCachedStats(dir.id);
      const unratedCount =
        stats?.ratingCounts.find((rc) => rc.rating === 0)?.count ?? 0;
      const isLargeUnrated =
        limit !== undefined &&
        stats?.subdirectoryCount === 0 &&
        unratedCount > limit;
      return {
        dir,
        stats,
        isLargeUnrated,
      };
    });

    const filteredItems = items.filter((item) => {
      // 过滤大量未评级目录
      if (item.isLargeUnrated) {
        return false;
      }
      return true;
    });

    return sortBy(filteredItems, [
      (item) => !item.stats,
      (item) => item.stats?.imageCount === 0,
      (item) => {
        const modTime = item.stats?.latestImage?.modTime;
        return modTime ? -new Date(modTime).getTime() : 0;
      },
    ]).map((item) => item.dir);
  });

  return {
    currentDirectory,
    largeUnratedCount,
    sortedDirectories,
    hasNextPage: computed(() => directoryConnection.pageInfo.value.hasNextPage),
    fetchMore: directoryConnection.fetchMore,
    loading: computed(() => loadingCount.value > 0),
  };
}

// 用来兼容可能的 GraphQL 导出的 Fragment 名称变动
const ImageFunnelDirectoryFragmentDoc = DirectoryFragmentDoc;
