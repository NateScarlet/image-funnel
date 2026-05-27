import type { TypedDocumentNode } from '@apollo/client';
import { Kind } from 'graphql';

export default function getOperationName<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>
): string {
  for (const definition of document.definitions) {
    if (definition.kind === Kind.OPERATION_DEFINITION) {
      return definition.name?.value ?? '';
    }
  }
  return '';
}
