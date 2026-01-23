import type {
  ObservableQuery,
  OperationVariables,
  TypedDocumentNode,
} from "@apollo/client/core";
import { NetworkStatus } from "@apollo/client/core";
import type { MaybeRefOrGetter, Ref } from "vue";
import { computed, shallowRef, toValue, watch } from "vue";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";
import { isEqual } from "es-toolkit";
import type { ApolloClient } from "@apollo/client";

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
  query: ObservableQuery<TData, TVariables>;
} & Disposable {
  const stack = new DisposableStack();
  import.meta.hot?.dispose(() => stack.dispose());
  const query = stack.adopt(
    apolloClient.watchQuery({
      ...options,
      query: document,
      variables: toValue(variables) as TVariables,
      notifyOnNetworkStatusChange: true,
    }),
    (i) => i.stopPolling(),
  );
  const resultBuffer = shallowRef<{ v?: ObservableQuery.Result<TData> }>();
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
          if (variables != null && toValue(variables) == null) {
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
        { immediate: true },
      ),
    );
  }
  async function run(stack: DisposableStack, variables?: TVariables) {
    resultModel.value = query.getCurrentResult();
    if (variables) {
      await query.setVariables(variables);
    }
    if (stack.disposed) {
      return;
    }
    resultModel.value = query.getCurrentResult();
    stack.adopt(
      query.subscribe({
        next: (data) => {
          if (!stack.disposed) {
            resultModel.value = data;
          }
        },
      }),
      (i) => i.unsubscribe(),
    );
  }
  if (variables) {
    let queryStack: DisposableStack | undefined;
    stack.defer(() => queryStack?.dispose());
    stack.defer(
      watch(
        () => toValue(variables),
        (n, o) => {
          if (isEqual(n, o)) {
            // not recreate query if variable not changed
            return;
          }
          if (n == null) {
            resultModel.value = undefined;
            return;
          }
          queryStack?.dispose();
          queryStack = new DisposableStack();
          run(queryStack, n);
        },
        { immediate: true },
      ),
    );
  } else {
    run(stack);
  }
  return {
    data: computed(() => resultModel.value?.data) as Ref<TData | undefined>,
    query,
    [Symbol.dispose]: () => stack.dispose(),
  };
}
