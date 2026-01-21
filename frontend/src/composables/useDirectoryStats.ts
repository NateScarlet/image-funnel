import {
  watch,
  shallowReactive,
  type MaybeRefOrGetter,
  toValue,
  type Ref,
} from "vue";
import useQuery from "../graphql/utils/useQuery";
import { GetDirectoryStatsDocument } from "../graphql/generated";
import type { DirectoryStatsFragment } from "../graphql/generated";

// 全局统计信息缓存
const statsCache = shallowReactive(
  new Map<string, DirectoryStatsFragment | null>(),
);

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
    const { data } = useQuery(GetDirectoryStatsDocument, {
      variables: () => ({ id: toValue(directoryId) }),
      loadingCount,
      context: {
        transport: "http",
      },
    });

    // 自动同步到全局缓存
    watch(
      () => data.value?.node,
      (node) => {
        if (node?.__typename === "Directory") {
          statsCache.set(node.id, node.stats ?? null);
        }
      },
      { immediate: true },
    );

    return data;
  }

  /**
   * 获取指定目录的统计信息（仅从缓存读取，不触发查询）
   */
  function getCachedStats(directoryId: MaybeRefOrGetter<string>) {
    return statsCache.get(toValue(directoryId)) ?? null;
  }

  return {
    useStats,
    getCachedStats,
  };
}
