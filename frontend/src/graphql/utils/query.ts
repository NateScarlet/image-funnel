import type {
  FetchPolicy,
  ErrorPolicy,
  OperationVariables,
  TypedDocumentNode,
  ApolloClient,
} from "@apollo/client/core";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";

export default function query<TData, TVariables extends OperationVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  {
    variables,
    ...options
  }: {
    variables?: TVariables | undefined;
    context?: OperationContext;
    fetchPolicy?: FetchPolicy;
    errorPolicy?: ErrorPolicy;
  } = {},
): Promise<ApolloClient.QueryResult<TData>> {
  return apolloClient.query<TData, TVariables>({
    ...options,
    query: document,
    variables: variables as TVariables,
  });
}
