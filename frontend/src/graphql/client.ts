import { ApolloClient, ApolloLink, Observable } from "@apollo/client/core";
import { CombinedGraphQLErrors } from "@apollo/client/errors";
import { BatchHttpLink } from "@apollo/client/link/batch-http";
import { ErrorLink } from "@apollo/client/link/error";
import { PersistedQueryLink } from "@apollo/client/link/persisted-queries";
import type { GraphQLFormattedError } from "graphql";
import { Kind, OperationTypeNode } from "graphql";
import { getMainDefinition } from "@apollo/client/utilities";
import type { ClientOptions } from "graphql-ws";

import { PersistentCache } from "./cache-persistence";
import useNotification from "../composables/useNotification";
import { HttpLink } from "@apollo/client";
import sha256Hash from "@/utils/sha256Hash";
import getGraphqlErrorMessage from "@/utils/getGraphqlErrorMessage";
import isAbortError from "@/utils/isAbortError";
import type OperationContext from "./OperationContext";
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

// #region 常驻连接重试策略（导出供测试锁定行为契约）
export const wsClientOptions = {
  // 常驻连接：无订阅的视图也维持连接，使版本失配检测不依赖当前视图是否活跃
  lazy: false,
  // 无限重试：默认重试 5 次后彻底放弃，服务器停机稍长旧页面将永远不再重连，
  // "正在重连…"通知永久挂起且版本检测失效。指数退避封顶 30 秒保证恢复后及时重连
  retryAttempts: Number.MAX_SAFE_INTEGER,
  // 握手失败（如停机期间网关 502）以裸 error 事件形态到达、不带 close code，
  // 库默认只放行 close 事件形态的失败，会导致常驻循环首次握手失败即永久放弃；
  // 显式放行一切失败，节奏由 retryAttempts/retryWait 全权控制
  shouldRetry: () => true,
  retryWait: async (retries: number) => {
    const ms = Math.min(1000 * 2 ** retries, 30_000);
    await new Promise((resolve) => setTimeout(resolve, ms));
  },
} satisfies Partial<ClientOptions>;
// #endregion

export const wsLink = new WebSocketLink({
  url: wsUrl.toString(),
  ...wsClientOptions,
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
      definition.kind === Kind.OPERATION_DEFINITION &&
      definition.operation === OperationTypeNode.MUTATION;
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
      definition.kind === Kind.OPERATION_DEFINITION &&
      definition.operation === OperationTypeNode.SUBSCRIPTION
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
      (i) => i.extensions?.code === "UNAUTHORIZED" || i.extensions?.code === "INVALID_TOKEN",
    );

    const hasRefreshToken = !!tokenStore.value?.refreshToken;

    if (hasAuthError && hasRefreshToken && !context.anonymous && !context.tokenRefreshed) {
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

  return undefined;
});

const persistentCache = new PersistentCache(
  "apollo-cache-persist",
  1000, // 1秒防抖
  (stats) => {
    const { showInfo } = useNotification();
    const parts = Object.entries(stats.entityCounts)
      .toSorted(([a], [b]) => a.localeCompare(b))
      .map(([type, count]) => `${type}=${count}`);
    showInfo(`缓存恢复完成 (${Math.round(stats.elapsedMs)}ms): ${parts.join(", ")}`, 0);
  },
);

try {
  await persistentCache.load();
} catch (error) {
  const { showError } = useNotification();
  showError(`缓存恢复失败: ${error instanceof Error ? error.message : String(error)}`);
}

const authLink = new ApolloLink((operation, forward) => {
  return new Observable((observer) => {
    let handle: { unsubscribe: () => void } | undefined;
    Promise.resolve()
      .then(async () => {
        const ctx = operation.getContext() as OperationContext & {
          anonymous?: boolean;
        };
        if (ctx.anonymous) {
          return undefined;
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
