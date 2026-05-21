import type { RelayNode } from "@/utils/isRelayNode";
import isRelayNode from "@/utils/isRelayNode";
import isString from "@/utils/isString";
import isUndefined from "@/utils/isUndefined";
import isUnion from "@/utils/isUnion";

export interface RelayEdge<T extends RelayNode = RelayNode> {
  node: T | null;
  cursor?: string;
}

export default function isRelayEdge(v: unknown): v is RelayEdge {
  try {
    return (
      isRelayNode((v as { node: unknown }).node) &&
      isUnion((v as { cursor: unknown }).cursor, isString, isUndefined)
    );
  } catch {
    return false;
  }
}
