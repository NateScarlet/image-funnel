import type {
  FetchPolicy,
  ErrorPolicy,
  OperationVariables,
  TypedDocumentNode,
  ApolloQueryResult,
} from "@apollo/client/core";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";

export default function query<TData, TVariables extends OperationVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  options: {
    variables?: TVariables | undefined;
    context?: OperationContext;
    fetchPolicy?: FetchPolicy;
    errorPolicy?: ErrorPolicy;
  } = {}
): Promise<ApolloQueryResult<TData>> {
  return apolloClient.query({
    ...options,
    query: document,
  });
}
