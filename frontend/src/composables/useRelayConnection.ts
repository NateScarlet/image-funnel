import extractNodes from "@/utils/extractNodes";
import extractPageInfo from "@/utils/extractPageInfo";
import fetchMore from "@/utils/fetchMore";
import type { RelayConnection } from "@/utils/isRelayConnection";
import type { RelayNode } from "@/utils/isRelayNode";
import type { ObservableQuery } from "@apollo/client";
import { computed } from "vue";

export default function useRelayConnection<
  TData extends object,
  TVariables extends Record<string, unknown>,
  TNode extends RelayNode,
>(
  connection: () => RelayConnection<TNode> | null | undefined,
  query: () =>
    | Pick<ObservableQuery<TData, TVariables>, "fetchMore">
    | undefined,
) {
  const nodes = computed(() => extractNodes(connection()));
  const pageInfo = computed(() => extractPageInfo(connection()));
  let skipFetchMore = false;
  async function connectionFetchMore() {
    if (skipFetchMore) {
      return;
    }
    skipFetchMore = true;
    try {
      const q = query();
      if (q) {
        await fetchMore(q, pageInfo.value);
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
