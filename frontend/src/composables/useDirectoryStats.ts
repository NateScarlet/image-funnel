import {
  type MaybeRefOrGetter,
  toValue,
  type Ref,
  onScopeDispose,
  shallowReactive,
} from "vue";
import { debounce } from "es-toolkit";
import useQuery from "../graphql/utils/useQuery";
import useSubscription from "../graphql/utils/useSubscription";
import {
  DirectoryStatsDocument,
  DirectoryChangedDocument,
} from "../graphql/generated";
import type {
  DirectoryStatsFragment,
  DirectoryStatsQuery,
} from "../graphql/generated";
import { apolloClient } from "../graphql/client";
import toStableValue from "@/utils/toStableValue";

/**
 * 目录统计信息的 composable
 * 提供全局缓存和响应式访问
 */
export default function useDirectoryStats() {
  /**
   * 获取指定目录的统计信息（自动查询和缓存）
   * @param directoryId 目录 ID
   * @param loadingCount 可选的加载计数器，用于追踪加载状态
   * @returns GraphQL 查询结果
   */
  function useStats(
    directoryId: MaybeRefOrGetter<string>,
    loadingCount?: Ref<number>,
  ) {
    // 执行 GraphQL 查询
    const { data, query } = useQuery(DirectoryStatsDocument, {
      variables: () => ({ id: toValue(directoryId) }),
      loadingCount,
      context: {
        transport: "batch-http:direcotry-stats",
      },
    });

    // 防抖的 refetch 函数
    const debouncedRefetch = debounce(() => {
      query.refetch();
    }, 1000);

    // 订阅目录变化
    useSubscription(DirectoryChangedDocument, {
      variables: () => ({ id: [toValue(directoryId)] }),
      onNext: (result) => {
        const changedId = result.data?.directoryChanged.id;
        // 当收到当前目录的变更通知时，重新获取数据
        if (changedId === toValue(directoryId)) {
          debouncedRefetch();
        }
      },
    });

    return data;
  }

  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose());

  const statsCache = shallowReactive(
    new Map<string, DirectoryStatsFragment | undefined>(),
  );

  /**
   * 获取指定目录的统计信息（仅从缓存读取，不触发查询）
   */
  function getCachedStats(
    directoryId: string,
  ): DirectoryStatsFragment | undefined {
    if (!statsCache.has(directoryId)) {
      // 初始化缓存
      const initial = apolloClient.readQuery({
        query: DirectoryStatsDocument,
        variables: { id: directoryId },
      })?.node?.stats;
      statsCache.set(directoryId, initial || undefined);

      // 建立订阅
      stack.adopt(
        apolloClient
          .watchQuery({
            query: DirectoryStatsDocument,
            variables: { id: directoryId },
            fetchPolicy: "cache-only",
          })
          .subscribe((result) => {
            statsCache.set(
              directoryId,
              toStableValue(
                (result.data as DirectoryStatsQuery)?.node?.stats || undefined,
                statsCache.get(directoryId),
              ),
            );
          }),
        (i) => i.unsubscribe(),
      );
    }

    return statsCache.get(directoryId);
  }

  return {
    useStats,
    getCachedStats,
  };
}
