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
    return (
      (v as { __typename: string }).__typename === "PageInfo" &&
      isUnion(
        (v as { hasNextPage: unknown }).hasNextPage,
        isBoolean,
        isUndefined,
      ) &&
      isUnion(
        (v as { hasPreviousPage: unknown }).hasPreviousPage,
        isBoolean,
        isUndefined,
      ) &&
      isUnion(
        (v as { startCursor: unknown }).startCursor,
        isString,
        isNull,
        isUndefined,
      ) &&
      isUnion(
        (v as { endCursor: unknown }).endCursor,
        isString,
        isNull,
        isUndefined,
      )
    );
  } catch {
    return false;
  }
}
