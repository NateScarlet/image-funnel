import { ApolloClient, ApolloLink } from "@apollo/client/core";
import { CombinedGraphQLErrors } from "@apollo/client/errors";
import { BatchHttpLink } from "@apollo/client/link/batch-http";
import { ErrorLink } from "@apollo/client/link/error";
import { PersistedQueryLink } from "@apollo/client/link/persisted-queries";
import type { GraphQLFormattedError } from "graphql";
import { createClient } from "graphql-ws";
import { GraphQLWsLink } from "@apollo/client/link/subscriptions";
import { getMainDefinition } from "@apollo/client/utilities";

import { PersistentCache } from "./cache-persistence";
import useNotification from "../composables/useNotification";
import { HttpLink } from "@apollo/client";
import sha256Hash from "@/utils/sha256Hash";
import getGraphqlErrorMessage from "@/utils/getGraphqlErrorMessage";

export interface OperationContext {
  anonymous?: boolean;
  fetchOptions?: RequestInit;
  transport?: "http" | "batch-http" | "ws";
  suppressError?:
    | boolean
    | ((ctx: { graphQLErrors?: readonly GraphQLFormattedError[] }) => boolean);

  // https://github.com/apollographql/apollo-client/blob/770cb7293d421ccad0abc1a43797c1f761d9aecf/src/link/persisted-queries/index.ts#L238
  http?: {
    includeQuery?: boolean;
    includeExtensions?: boolean;
  };
}

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
});

const batchHttpLink = new BatchHttpLink({
  uri: "graphql",
  batchMax: 1024,
  batchInterval: 10,
  batchDebounce: true,
});

const persistedQueryLink = new PersistedQueryLink({
  sha256: sha256Hash,
});

const wsUrl = new URL("graphql", document.baseURI);
wsUrl.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

const wsLink = new GraphQLWsLink(
  createClient({
    url: wsUrl.toString(),
  }),
);

const httpOrBatchLink = ApolloLink.split(
  ({ variables, getContext }) => {
    return (
      (getContext() as OperationContext).transport === "http" ||
      containsUpload(variables)
    );
  },
  persistedQueryLink.concat(httpLink),
  persistedQueryLink.concat(batchHttpLink),
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

const errorLink = new ErrorLink(({ error, operation }) => {
  const knownMessages = new Set();
  const errorOnce = (msg: string) => {
    if (knownMessages.has(msg)) {
      return;
    }
    const { showError } = useNotification();
    showError(`${operation.operationName}: ${msg}`);
    knownMessages.add(msg);
  };

  const context = operation.getContext() as OperationContext;
  const suppressError = context.suppressError;

  let graphQLErrors: readonly GraphQLFormattedError[] | undefined;
  let networkError: Error | undefined;

  if (CombinedGraphQLErrors.is(error)) {
    graphQLErrors = error.errors;
  } else {
    networkError = error;
  }

  if (graphQLErrors) {
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
        errorOnce(getGraphqlErrorMessage(i));
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

    if (!shouldSuppress) {
      errorOnce(
        `网络错误: ${networkError instanceof Error ? networkError.message : "Unknown error"}`,
      );
    }
  }
});

export const apolloClient = new ApolloClient({
  link: ApolloLink.from([errorLink, link]),
  cache: new PersistentCache(
    "apollo-cache-persist",
    1024 * 1024, // 1MB
    1000, // 1秒防抖
  ),
  defaultOptions: {
    watchQuery: {
      fetchPolicy: "cache-and-network",
    },
  },
});
