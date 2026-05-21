import isNull from "@/utils/isNull";
import type { PageInfo } from "@/utils/isPageInfo";
import isPageInfo from "@/utils/isPageInfo";
import type { RelayEdge } from "@/utils/isRelayEdge";
import isRelayEdge from "@/utils/isRelayEdge";
import type { RelayNode } from "@/utils/isRelayNode";
import isRelayNode from "@/utils/isRelayNode";
import isUndefined from "@/utils/isUndefined";
import isUnion from "@/utils/isUnion";
import isObject from "@/utils/isObject";
import isNumber from "@/utils/isNumber";

export interface RelayConnection<T extends RelayNode = RelayNode> {
  pageInfo?: PageInfo;
  nodes?: (T | null)[] | null;
  edges?: (RelayEdge<T> | null)[] | null;
  totalCount?: number;
}

/** check if value is github style relay connection. */
export default function isRelayConnection(v: unknown): v is RelayConnection {
  try {
    return (
      isObject(v) &&
      !!(v.nodes || v.edges) &&
      isUnion(v.pageInfo, isPageInfo, isUndefined) &&
      isUnion(
        v.nodes,
        (i): i is (RelayNode | null)[] =>
          Array.isArray(i) && i.every((j) => isUnion(j, isRelayNode, isNull)),
        isNull,
        isUndefined,
      ) &&
      isUnion(
        v.edges,
        (i): i is (RelayEdge | null)[] =>
          Array.isArray(i) && i.every((j) => isUnion(j, isRelayEdge, isNull)),
        isNull,
        isUndefined,
      ) &&
      isUnion(v.totalCount, isNumber, isUndefined)
    );
  } catch {
    return false;
  }
}
