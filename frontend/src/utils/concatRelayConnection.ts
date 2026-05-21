import type { RelayConnection } from "@/utils/isRelayConnection";
import type { RelayNode } from "@/utils/isRelayNode";

export default function concatRelayConnection<
  TConnection extends RelayConnection<TNode>,
  TNode extends RelayNode,
>(a: TConnection, b: RelayConnection<TNode> | null | undefined): TConnection {
  const ret = { ...a };
  if (!b) {
    return ret;
  }
  if (ret.edges) {
    ret.edges = ret.edges.concat(b.edges ?? []);
  }
  if (ret.nodes) {
    ret.nodes = ret.nodes.concat(b.nodes ?? []);
  }
  ret.pageInfo =
    a.pageInfo && b.pageInfo
      ? {
          ...a.pageInfo,
          endCursor: b.pageInfo.endCursor,
          hasNextPage: b.pageInfo.hasNextPage,
        }
      : undefined;
  return ret;
}
