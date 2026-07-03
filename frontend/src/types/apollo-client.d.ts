import type OperationContext from "@/graphql/OperationContext";
import type { HttpLink } from "@apollo/client";
import type { BatchHttpLink } from "@apollo/client/link/batch-http";

declare module "@apollo/client" {
  interface DefaultContext
    extends HttpLink.ContextOptions, BatchHttpLink.ContextOptions, OperationContext {
    // https://github.com/apollographql/apollo-client/blob/770cb7293d421ccad0abc1a43797c1f761d9aecf/src/link/persisted-queries/index.ts#L238
    http?: {
      includeQuery?: boolean;
      includeExtensions?: boolean;
    };
  }
}
