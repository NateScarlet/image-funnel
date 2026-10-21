import extractNodes from "@/utils/extractNodes";
import extractPageInfo from "@/utils/extractPageInfo";
import fetchMore from "@/utils/fetchMore";
import type { RelayConnection } from "@/utils/isRelayConnection";
import type { RelayNode } from "@/utils/isRelayNode";
import type { ObservableQuery } from "@apollo/client";
import { computed, toValue, type MaybeRefOrGetter } from "vue";

export default function useRelayConnection<
  TData extends object,
  TVariables extends Record<string, unknown>,
  TNode extends RelayNode,
>(
  connection: () => RelayConnection<TNode> | null | undefined,
  query: () => Pick<ObservableQuery<TData, TVariables>, "fetchMore"> | undefined,
  options?: {
    /** 反向取页的页大小；before 需与 last 搭配 */
    backwardPageSize?: MaybeRefOrGetter<number | undefined>;
  },
) {
  const nodes = computed(() => extractNodes(connection()));
  const pageInfo = computed(() => extractPageInfo(connection()));
  let skipFetchMore = false;
  async function connectionFetchMore(direction?: "forward" | "backward") {
    if (skipFetchMore) {
      return;
    }
    skipFetchMore = true;
    try {
      const q = query();
      if (q) {
        await fetchMore(q, pageInfo.value, {
          direction,
          backwardPageSize: toValue(options?.backwardPageSize),
        });
      }
    } finally {
      skipFetchMore = false;
    }
  }

  return {
    nodes,
    pageInfo,
    fetchMore: connectionFetchMore,
  };
}
