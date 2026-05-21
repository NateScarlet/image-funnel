import isNumber from "@/utils/isNumber";
import isObject from "@/utils/isObject";
import isString from "@/utils/isString";

type Scalar = string | number;
// https://relay.dev/graphql/connections.htm#sec-Node
export type RelayNode = Scalar | object | null;

export default function isRelayNode(v: unknown): v is RelayNode {
  try {
    return v === null || isString(v) || isNumber(v) || isObject(v);
  } catch {
    return false;
  }
}
