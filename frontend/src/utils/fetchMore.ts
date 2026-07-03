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
      const more = fetchMoreResult[k as keyof T];
      if (isRelayConnection(v) && isRelayConnection(more)) {
        return [
          k,
          backward ? concatRelayConnection(more, v as typeof more) : concatRelayConnection(v, more),
        ];
      }

      if (isObject(v) && isObject(more)) {
        return [k, mergeResultDeep(v, more, backward)];
      }

      return [k, v];
    }),
  ) as T;
}

export default async function fetchMore<
  T extends object,
  V extends {
    first?: number | null;
    after?: string | null;
    last?: number | null;
    before?: string | null;
  },
>(query: Pick<ObservableQuery<T, V>, "fetchMore">, pageInfo: PageInfo): Promise<void> {
  try {
    if (pageInfo.hasNextPage) {
      await query.fetchMore({
        variables: {
          after: pageInfo.endCursor,
        } as Partial<V>,

        updateQuery(previousResult, { fetchMoreResult }) {
          return mergeResultDeep(previousResult, fetchMoreResult, false);
        },
      });
    } else if (pageInfo.hasPreviousPage) {
      await query.fetchMore({
        variables: {
          before: pageInfo.startCursor,
        } as Partial<V>,
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
