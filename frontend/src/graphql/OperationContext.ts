import { GraphQLFormattedError } from "graphql";

export default interface OperationContext {
  fetchOptions?: RequestInit;
  transport?: "http" | "batch-http" | `batch-http:${string}` | "ws";
  suppressError?:
    | boolean
    | ((ctx: { graphQLErrors?: readonly GraphQLFormattedError[] }) => boolean);

  // https://github.com/apollographql/apollo-client/blob/770cb7293d421ccad0abc1a43797c1f761d9aecf/src/link/persisted-queries/index.ts#L238
  http?: {
    includeQuery?: boolean;
    includeExtensions?: boolean;
  };
}
