import {
  ApolloClient,
  InMemoryCache,
  createHttpLink,
} from "@apollo/client/core";
import { onError } from "@apollo/client/link/error";
import type { GraphQLFormattedError } from "graphql";
import useNotification from "../composables/useNotification";

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

const httpLink = createHttpLink({
  uri: "graphql",
});

const errorLink = onError(({ graphQLErrors, networkError, operation }) => {
  const context = operation.getContext() as OperationContext;
  const suppressError = context.suppressError;

  if (graphQLErrors) {
    let shouldSuppress = false;
    if (typeof suppressError === "function") {
      shouldSuppress = suppressError({ graphQLErrors });
    } else if (suppressError === true) {
      shouldSuppress = true;
    }

    if (!shouldSuppress) {
      const { showError } = useNotification();
      const errorMessages = graphQLErrors
        .map((err: GraphQLFormattedError) => err.message)
        .join("; ");
      showError(`GraphQL 错误: ${errorMessages}`);
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
      const { showError } = useNotification();
      showError(
        `网络错误: ${networkError instanceof Error ? networkError.message : "Unknown error"}`,
      );
    }
  }
});

export const apolloClient = new ApolloClient({
  link: errorLink.concat(httpLink),
  cache: new InMemoryCache(),
  defaultOptions: {
    watchQuery: {
      fetchPolicy: "cache-and-network",
    },
  },
});
