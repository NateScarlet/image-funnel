import {
  ApolloClient,
  InMemoryCache,
  createHttpLink,
} from "@apollo/client/core";
import type { GraphQLFormattedError } from "graphql";

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

export const apolloClient = new ApolloClient({
  link: httpLink,
  cache: new InMemoryCache(),
  defaultOptions: {
    watchQuery: {
      fetchPolicy: "cache-and-network",
    },
  },
});
