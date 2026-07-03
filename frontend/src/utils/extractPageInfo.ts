import type { PageInfo } from "@/utils/isPageInfo";

export interface Connection {
  pageInfo?: PageInfo;
}

export default function extractPageInfo(connection: Connection | null | undefined): PageInfo {
  return (
    connection?.pageInfo ?? {
      __typename: "PageInfo",
      hasNextPage: false,
      hasPreviousPage: false,
      startCursor: null,
      endCursor: null,
    }
  );
}
