import type { ObservableQuery } from "@apollo/client/core";
import concatRelayConnection from "@/utils/concatRelayConnection";
import type { PageInfo } from "@/utils/isPageInfo";
import isRelayConnection from "@/utils/isRelayConnection";
import isObject from "@/utils/isObject";

function mergeResultDeep<T extends object>(
  previousResult: T,
  fetchMoreResult: T | undefined,
  backward: boolean,
): T {
  if (!fetchMoreResult) {
    return previousResult;
  }

  return Object.fromEntries(
    Object.entries(previousResult).map(([k, v]) => {
      const more = fetchMoreResult[k as unknown as keyof T];
      if (isRelayConnection(v) && isRelayConnection(more)) {
        return [
          k,
          backward
            ? concatRelayConnection(more, v as unknown as typeof more)
            : concatRelayConnection(v, more),
        ];
      }

      if (isObject(v) && isObject(more)) {
        return [k, mergeResultDeep(v, more, backward)];
      }

      return [k, v];
    }),
  ) as unknown as T;
}

export interface FetchMoreOptions {
  /** 指定取页方向；缺省时优先向后，没有后续页再向前 */
  direction?: "forward" | "backward";
  /** 反向分页的页大小：before 需要与 last 搭配才取紧邻游标之前的一页 */
  backwardPageSize?: number;
}

export default async function fetchMore<
  T extends object,
  V extends {
    first?: number | null;
    after?: string | null;
    last?: number | null;
    before?: string | null;
  },
>(
  query: Pick<ObservableQuery<T, V>, "fetchMore">,
  pageInfo: PageInfo,
  options?: FetchMoreOptions,
): Promise<void> {
  const direction = options?.direction ?? (pageInfo.hasNextPage ? "forward" : "backward");
  try {
    if (direction === "forward") {
      if (!pageInfo.hasNextPage) return;
      await query.fetchMore({
        variables: {
          after: pageInfo.endCursor,
        } as unknown as Partial<V>,

        updateQuery(previousResult, { fetchMoreResult }) {
          return mergeResultDeep(previousResult, fetchMoreResult, false);
        },
      });
    } else {
      if (!pageInfo.hasPreviousPage) return;
      const pageSize = options?.backwardPageSize;
      await query.fetchMore({
        variables: {
          before: pageInfo.startCursor,
          // 反向取页不带 first，避免与 last 同时出现
          ...(pageSize === undefined ? {} : { last: pageSize, first: undefined }),
        } as unknown as Partial<V>,
        updateQuery(previousResult, { fetchMoreResult }) {
          return mergeResultDeep(previousResult, fetchMoreResult, true);
        },
      });
    }
  } catch (err) {
    // https://github.com/apollographql/apollo-client/issues/4114
    if (String(err).startsWith("Invariant Violation:")) {
      if (import.meta.env.DEV) {
        console.error(err);
      }
    } else {
      throw err;
    }
  }
}
