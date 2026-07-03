import type {
  ObservableQuery,
  OperationVariables,
  TypedDocumentNode,
  ApolloClient,
} from "@apollo/client/core";
import { NetworkStatus } from "@apollo/client/core";
import type { MaybeRefOrGetter, Ref } from "vue";
import { computed, onScopeDispose, shallowRef, toValue, watch } from "vue";
import stableComputed from "@/composables/stableComputed";
import type OperationContext from "../OperationContext";
import getOperationName from "./getOperationName";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import { isEqual } from "es-toolkit";
import client from "../client";

function isLoading(v: ObservableQuery.Result<unknown> | undefined): boolean {
  return (
    v == null ||
    v.loading ||
    v.networkStatus === NetworkStatus.loading ||
    v.networkStatus === NetworkStatus.setVariables ||
    v.networkStatus === NetworkStatus.refetch ||
    v.networkStatus === NetworkStatus.fetchMore
  );
}

export default function useQuery<TData, TVariables extends OperationVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  {
    variables,
    loadingCount,
    ...options
  }: {
    variables?: MaybeRefOrGetter<TVariables | undefined>;
    context?: OperationContext;
    loadingCount?: Ref<number>;
  } & Pick<
    ApolloClient.WatchQueryOptions<TData, TVariables>,
    "fetchPolicy" | "nextFetchPolicy" | "errorPolicy" | "pollInterval"
  > = {},
): {
  data: Ref<TData | undefined>;
  query: Omit<ObservableQuery<TData, TVariables>, "refetch">;
  refresh: () => Promise<void>;
} & Disposable {
  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose(), true);
  import.meta.hot?.dispose(() => stack.dispose());
  const skip = computed(() => variables != null && !toValue(variables));
  let lastVariables = toValue(variables);
  const query = stack.adopt(
    client.watchQuery({
      ...options,
      query: document,
      variables: lastVariables as TVariables,
      notifyOnNetworkStatusChange: true,
      skipPollAttempt() {
        return skip.value;
      },
    } satisfies ApolloClient.WatchQueryOptions<TData, TVariables>),
    (i) => i.stopPolling(),
  );
  const result = shallowRef<ObservableQuery.Result<TData>>();
  // 变量修改
  const stableVariables = stableComputed(() => toValue(variables));
  stack.defer(
    watch(
      stableVariables,
      (vars) => {
        if (vars && !isEqual(vars, lastVariables)) {
          lastVariables = vars;
          void query.setVariables(vars);
        }
      },
      { immediate: true },
    ),
  );
  // 中止
  stack.defer(
    watch(
      skip,
      (shouldSkip, _, onCleanup) => {
        if (shouldSkip) {
          result.value = undefined;
          return;
        }
        let cancelled = false;
        onCleanup(() => {
          cancelled = true;
        });
        result.value = query.getCurrentResult();
        const sub = query.subscribe((data) => {
          if (cancelled) {
            return;
          }
          result.value = data;
        });
        onCleanup(() => sub.unsubscribe());
      },
      { immediate: true },
    ),
  );
  // 加载状态
  if (loadingCount) {
    stack.defer(
      watch(
        () => !skip.value && isLoading(result.value),
        (loading, _, onCleanup) => {
          if (loading) {
            loadingCount.value += 1;
            onCleanup(() => {
              loadingCount.value -= 1;
            });
          }
        },
        { immediate: true },
      ),
    );
  }
  // 刷新
  const flight = new SingleFlightGroup();
  let lastRefreshDurationMs = 0;
  async function refresh() {
    await flight.do("refresh", async () => {
      const startAt = performance.now();
      const vars = toValue(variables);
      if (variables == null || !!vars) {
        await query.refetch(vars);
        lastRefreshDurationMs = performance.now() - startAt;
        if (lastRefreshDurationMs > 1e3) {
          console.warn("slow refresh", {
            operationName: getOperationName(document),
            lastRefreshDurationMs,
          });
        }
      }
    });
  }
  return {
    data: stableComputed(() => {
        if (result.value?.dataState === "complete") {
          return result.value.data;
        }
        return undefined;
      }),
    query,
    refresh,
    [Symbol.dispose]: () => stack.dispose(),
  };
}
