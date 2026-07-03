import isUndefined from "@/utils/isUndefined";
import isBoolean from "@/utils/isBoolean";
import isNull from "@/utils/isNull";
import isString from "@/utils/isString";
import isUnion from "@/utils/isUnion";

export interface PageInfo {
  __typename: "PageInfo";
  hasNextPage?: boolean;
  hasPreviousPage?: boolean;
  startCursor?: string | null;
  endCursor?: string | null;
}

export default function isPageInfo(v: unknown): v is PageInfo {
  try {
    const obj = v as unknown as Record<string, unknown>;
    return (
      obj.__typename === "PageInfo" &&
      isUnion(obj.hasNextPage, isBoolean, isUndefined) &&
      isUnion(obj.hasPreviousPage, isBoolean, isUndefined) &&
      isUnion(obj.startCursor, isString, isNull, isUndefined) &&
      isUnion(obj.endCursor, isString, isNull, isUndefined)
    );
  } catch {
    return false;
  }
}
