import "core-js/actual/disposable-stack";

import type {
  OperationVariables,
  TypedDocumentNode,
  FetchPolicy,
  ErrorPolicy,
} from "@apollo/client/core";
import { getCurrentInstance, onUnmounted, watch, type WatchSource } from "vue";
import { apolloClient, OperationContext } from "../client";
import { ApolloLink } from "@apollo/client";
import { ApolloClient } from "@apollo/client";
import { debounce, isEqual } from "es-toolkit/compat";

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
  if (getCurrentInstance()) {
    onUnmounted(() => {
      stack.dispose();
    });
  }
  import.meta.hot?.dispose(() => stack.dispose());

  function run(variables?: TVariables) {
    const stack = new DisposableStack();
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
    return stack;
  }
  if (options.variables) {
    let current: DisposableStack | undefined;
    stack.defer(() => current?.dispose());
    const waitMs = 100;
    const setVariables = stack.adopt(
      debounce(
        (v: TVariables) => {
          const previous = current;
          current = run(v);
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
    stack.defer(
      watch(
        options.variables,
        (v, oldValue) => {
          if (v == null) {
            current?.dispose();
            current = undefined;
            return;
          }
          if (isEqual(v, oldValue)) {
            return;
          }
          setVariables(v);
        },
        { flush: "post", immediate: true },
      ),
    );
  } else {
    stack.use(run());
  }

  return stack;
}
