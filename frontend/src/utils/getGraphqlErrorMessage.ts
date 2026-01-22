import type { GraphQLFormattedError } from "graphql";
import getUILanguage from "./getUILanguage";

export default function getGraphqlErrorMessage(
  e: Pick<GraphQLFormattedError, "extensions" | "message"> | Error,
): string {
  if ("extensions" in e) {
    return (
      ((e.extensions?.locales as Record<string, unknown> | undefined)?.[
        getUILanguage()
      ] as string | undefined) ?? e.message
    );
  }
  return e.message;
}
