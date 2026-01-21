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
  options: {
    variables?: TVariables | undefined;
    context?: OperationContext;
    fetchPolicy?: MutationFetchPolicy;
    errorPolicy?: ErrorPolicy;
  } = {},
): Promise<ApolloClient.MutateResult<TData>> {
  return apolloClient.mutate<TData, TVariables>({
    ...options,
    mutation: document,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } as any);
}
