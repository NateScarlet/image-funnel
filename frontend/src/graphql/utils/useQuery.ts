import type {
  ApolloQueryResult,
  ObservableQuery,
  OperationVariables,
  TypedDocumentNode,
  WatchQueryOptions,
} from "@apollo/client/core";
import { NetworkStatus } from "@apollo/client/core";
import type { Ref } from "vue";
import { computed, shallowRef, watch } from "vue";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";
import { isEqual } from "es-toolkit";

function isLoading(v: ApolloQueryResult<unknown> | undefined): boolean {
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
  options: {
    variables?: () => TVariables | undefined;
    context?: OperationContext;
    loadingCount?: Ref<number>;
  } & Pick<
    WatchQueryOptions<TVariables>,
    "fetchPolicy" | "nextFetchPolicy" | "errorPolicy" | "pollInterval"
  > = {}
): {
  data: Ref<TData | undefined>;
  query: ObservableQuery<TData, TVariables>;
} & Disposable {
  const { loadingCount, variables } = options;
  const stack = new DisposableStack();
  import.meta.hot?.dispose(() => stack.dispose());
  const query = stack.adopt(
    apolloClient.watchQuery({
      ...options,
      query: document,
      variables: options.variables?.(),
      notifyOnNetworkStatusChange: true,
    }),
    (i) => i.stopPolling()
  );
  const resultBuffer = shallowRef<{ v?: ApolloQueryResult<TData> }>();
  const resultModel = computed({
    get() {
      return resultBuffer.value?.v;
    },
    set(v) {
      resultBuffer.value = { v };
    },
  });
  if (loadingCount) {
    stack.defer(
      watch(
        () => {
          if (variables && variables() == null) {
            // skipping
            return false;
          }
          return resultBuffer.value && isLoading(resultBuffer.value.v);
        },
        (value, _, cleanup) => {
          if (value) {
            loadingCount.value += 1;
            cleanup(() => {
              loadingCount.value -= 1;
            });
          }
        },
        { immediate: true }
      )
    );
  }
  function read() {
    resultModel.value = query.getCurrentResult(true);
  }
  function run(variables?: TVariables) {
    const stack = new DisposableStack();
    (async () => {
      read();
      if (variables) {
        await query.setVariables(variables);
      }
      if (stack.disposed) {
        return;
      }
      stack.adopt(
        query.subscribe({
          start: read,
          next: read,
          error: () => undefined,
        }),
        (i) => i.unsubscribe()
      );
    })();
    return stack;
  }
  if (variables) {
    stack.defer(
      watch(
        variables,
        (n, o, onCleanup) => {
          if (isEqual(n, o)) {
            // not recreate query if variable not changed
            return;
          }
          if (n == null) {
            resultModel.value = undefined;
            return;
          }
          const stack = run(n);
          onCleanup(() => stack.dispose());
        },
        { immediate: true }
      )
    );
  } else {
    stack.use(run());
  }
  return {
    data: computed(() => resultModel.value?.data),
    query,
    [Symbol.dispose]: () => stack.dispose(),
  };
}
