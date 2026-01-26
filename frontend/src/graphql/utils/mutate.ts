import type {
  MutationFetchPolicy,
  OperationVariables,
  TypedDocumentNode,
  ErrorPolicy,
  ApolloClient,
} from "@apollo/client/core";
import type { OperationContext } from "../client";
import { apolloClient } from "../client";

export default function mutate<TData, TVariables extends OperationVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  {
    variables,
    ...options
  }: {
    variables?: TVariables | undefined;
    context?: OperationContext;
    fetchPolicy?: MutationFetchPolicy;
    errorPolicy?: ErrorPolicy;
  } = {},
): Promise<ApolloClient.MutateResult<TData>> {
  return apolloClient.mutate<TData, TVariables>({
    ...options,
    mutation: document,
    variables: variables as TVariables,
  });
}
