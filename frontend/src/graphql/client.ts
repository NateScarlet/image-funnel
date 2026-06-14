import { ApolloClient, ApolloLink, Observable } from "@apollo/client/core";
import { CombinedGraphQLErrors } from "@apollo/client/errors";
import { BatchHttpLink } from "@apollo/client/link/batch-http";
import { ErrorLink } from "@apollo/client/link/error";
import { PersistedQueryLink } from "@apollo/client/link/persisted-queries";
import type { GraphQLFormattedError } from "graphql";
import { getMainDefinition } from "@apollo/client/utilities";

import { PersistentCache } from "./cache-persistence";
import useNotification from "../composables/useNotification";
import { HttpLink } from "@apollo/client";
import sha256Hash from "@/utils/sha256Hash";
import getGraphqlErrorMessage from "@/utils/getGraphqlErrorMessage";
import isAbortError from "@/utils/isAbortError";
import OperationContext from "./OperationContext";
export type { OperationContext };
import WebSocketLink from "./WebsocketLink";
import { getValidToken, refreshToken, tokenStore } from "./tokenManager";
export { tokenStore };

function containsUpload(v: unknown): boolean {
  if (v == null) {
    return false;
  }
  if (v instanceof File || v instanceof Blob) {
    return true;
  }
  if (typeof v === "object") {
    return Object.values(v).some(containsUpload);
  }
  return false;
}

const httpLink = new HttpLink({
  uri: "graphql",
  credentials: "include",
});

const batchHttpLink = new BatchHttpLink({
  uri: "graphql",
  batchMax: 64,
  batchInterval: 10,
  batchKey: (operation) => {
    const ctx = operation.getContext() as OperationContext;
    return ctx.transport || "batch-http";
  },
  credentials: "include",
});

const persistedQueryLink = new PersistedQueryLink({
  sha256: sha256Hash,
});

const wsUrl = new URL("graphql", document.baseURI);
wsUrl.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

const wsLink = new WebSocketLink({
  url: wsUrl.toString(),
  connectionParams: async () => {
    const token = await getValidToken();
    if (token) {
      return {
        Authorization: `Bearer ${token}`,
      };
    }
    return {};
  },
});

const httpOrBatchLink = ApolloLink.split(
  ({ query, variables, getContext }) => {
    const definition = getMainDefinition(query);
    const isMutation =
      definition.kind === "OperationDefinition" &&
      definition.operation === "mutation";
    return (
      (getContext() as OperationContext).transport === "http" ||
      isMutation ||
      containsUpload(variables)
    );
  },
  httpLink,
  batchHttpLink,
);

const link = ApolloLink.split(
  ({ query, getContext }) => {
    if ((getContext() as OperationContext).transport === "ws") {
      return true;
    }
    const definition = getMainDefinition(query);
    return (
      definition.kind === "OperationDefinition" &&
      definition.operation === "subscription"
    );
  },
  wsLink,
  httpOrBatchLink,
);

const errorLink = new ErrorLink(({ error, operation, forward }) => {
  const knownMessages = new Set();
  const errorOnce = (msg: string) => {
    if (knownMessages.has(msg)) {
      return;
    }
    const { showError } = useNotification();
    showError(`${operation.operationName}: ${msg}`);
    knownMessages.add(msg);
  };

  const context = operation.getContext() as OperationContext & {
    tokenRefreshed?: boolean;
    anonymous?: boolean;
  };
  const suppressError = context.suppressError;

  let graphQLErrors: readonly GraphQLFormattedError[] | undefined;
  let networkError: Error | undefined;

  if (CombinedGraphQLErrors.is(error)) {
    graphQLErrors = error.errors;
  } else {
    networkError = error;
  }

  if (graphQLErrors) {
    const hasAuthError = graphQLErrors.some(
      (i) =>
        i.extensions?.code === "UNAUTHORIZED" ||
        i.extensions?.code === "INVALID_TOKEN",
    );

    if (hasAuthError && !context.anonymous && !context.tokenRefreshed) {
      operation.setContext({ tokenRefreshed: true });
      return new Observable((observer) => {
        let handle: { unsubscribe: () => void } | undefined;
        refreshToken()
          .then(() => {
            const token = tokenStore.value?.accessToken;
            if (token) {
              operation.setContext(({ headers = {} }) => ({
                headers: {
                  ...headers,
                  Authorization: `Bearer ${token}`,
                },
              }));
            }
            handle = forward(operation).subscribe({
              next: observer.next.bind(observer),
              error: observer.error.bind(observer),
              complete: observer.complete.bind(observer),
            });
          })
          .catch((err) => {
            // 不在此处盲目重定向至登录页。对于因网络波动引发的刷新失败，此处仅传播错误，
            // 确保用户不会因为网络抖动被强制退出。真正的认证失效（如 INVALID_TOKEN）已由 tokenManager.ts 的错误拦截处理。
            observer.error(err);
          });

        return () => {
          if (handle) {
            handle.unsubscribe();
          }
        };
      });
    }

    let shouldSuppress = false;
    if (typeof suppressError === "function") {
      shouldSuppress = suppressError({ graphQLErrors });
    } else if (suppressError === true) {
      shouldSuppress = true;
    }

    if (!shouldSuppress) {
      if (import.meta.env.DEV) {
        console.error({
          operation,
          graphQLErrors,
        });
      }

      graphQLErrors.forEach((i) => {
        const code = i.extensions?.code;
        if (code === "UNAUTHORIZED" || code === "INVALID_TOKEN") {
          if (window.location.pathname !== "/auth") {
            window.location.href = `/auth?redirect=${encodeURIComponent(window.location.pathname + window.location.search)}`;
          }
        } else {
          errorOnce(getGraphqlErrorMessage(i));
        }
      });
    }
  }

  if (networkError) {
    let shouldSuppress = false;
    if (typeof suppressError === "function") {
      shouldSuppress = suppressError({ graphQLErrors: undefined });
    } else if (suppressError === true) {
      shouldSuppress = true;
    }

    if (!shouldSuppress && !isAbortError(networkError)) {
      errorOnce(
        `网络错误: ${networkError instanceof Error ? networkError.message : "Unknown error"}`,
      );
    }
  }
});

const persistentCache = new PersistentCache(
  "apollo-cache-persist",
  1000, // 1秒防抖
);

await persistentCache.load();

const authLink = new ApolloLink((operation, forward) => {
  return new Observable((observer) => {
    let handle: { unsubscribe: () => void } | undefined;
    Promise.resolve()
      .then(async () => {
        const ctx = operation.getContext() as OperationContext & {
          anonymous?: boolean;
        };
        if (ctx.anonymous) {
          return;
        }
        return getValidToken();
      })
      .then((token) => {
        if (token) {
          operation.setContext(({ headers = {} }) => ({
            headers: {
              ...headers,
              Authorization: `Bearer ${token}`,
            },
          }));
        }
        handle = forward(operation).subscribe({
          next: observer.next.bind(observer),
          error: observer.error.bind(observer),
          complete: observer.complete.bind(observer),
        });
      })
      .catch(observer.error.bind(observer));

    return () => {
      if (handle) {
        handle.unsubscribe();
      }
    };
  });
});

export const client = new ApolloClient({
  link: ApolloLink.from([errorLink, persistedQueryLink, authLink, link]),
  cache: persistentCache,
  assumeImmutableResults: true,
  defaultOptions: {
    watchQuery: {
      fetchPolicy: "cache-and-network",
    },
  },
});

export default client;
