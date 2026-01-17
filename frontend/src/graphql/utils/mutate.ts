import type {
  MutationFetchPolicy,
  OperationVariables,
  TypedDocumentNode,
  FetchResult,
  ErrorPolicy,
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
  } = {}
): Promise<FetchResult<TData>> {
  return apolloClient.mutate({
    ...options,
    mutation: document,
  });
}
