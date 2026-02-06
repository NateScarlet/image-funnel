import "core-js/actual/disposable-stack";

import type {
  OperationVariables,
  TypedDocumentNode,
  FetchPolicy,
  ErrorPolicy,
  ApolloLink,
  ApolloClient,
} from "@apollo/client/core";
import { computed, onScopeDispose, watch, type WatchSource } from "vue";
import { debounce } from "es-toolkit/compat";
import toStableValue from "@/utils/toStableValue";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";

export default function useSubscription<
  TData,
  TVariables extends OperationVariables,
>(
  document: TypedDocumentNode<TData, TVariables>,
  options: {
    onNext?: (v: ApolloLink.Result<TData>) => void;
    onError?: (err: unknown) => void;
    variables?: WatchSource<TVariables | undefined>;
    context?: OperationContext;
    fetchPolicy?: FetchPolicy;
    errorPolicy?: ErrorPolicy;
  } = {},
): Disposable {
  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose(), true);
  import.meta.hot?.dispose(() => stack.dispose());

  function run(stack: DisposableStack, variables?: TVariables) {
    const ob = apolloClient.subscribe({
      ...options,
      query: document,
      variables: variables as TVariables,
    } satisfies ApolloClient.SubscribeOptions<TData, TVariables>);
    stack.adopt(
      ob.subscribe({
        next(v) {
          if (stack.disposed) {
            return;
          }
          options.onNext?.(v);
        },
        error(err) {
          if (stack.disposed) {
            return;
          }
          options.onError?.(err);
          stack.dispose();
        },
      }),
      (i) => i.unsubscribe(),
    );
  }
  if (options.variables) {
    let current: DisposableStack | undefined;
    stack.defer(() => current?.dispose());
    const waitMs = 100;
    const setVariables = stack.adopt(
      debounce(
        (v: TVariables) => {
          const previous = current;
          current = new DisposableStack();
          run(current, v);
          if (previous) {
            // 启动后再停止之前的订阅，确保无缝衔接
            setTimeout(() => {
              previous?.dispose();
            }, waitMs);
          }
        },
        waitMs,
        { leading: true, trailing: true, maxWait: 1e3 },
      ),
      (i) => i.cancel(),
    );
    const stableVariables = computed<TVariables | undefined>((oldValue) =>
      toStableValue(options.variables, oldValue),
    );
    stack.defer(
      watch(
        stableVariables,
        (v) => {
          if (v == null) {
            current?.dispose();
            current = undefined;
            return;
          }
          setVariables(v);
        },
        { immediate: true },
      ),
    );
  } else {
    run(stack);
  }

  return stack;
}
