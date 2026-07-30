import { describe, test, expect, beforeEach } from "vitest";

function handleErrors({
  graphQLErrors,
}: {
  graphQLErrors?: Array<{ extensions?: { code?: string } }>;
}) {
  if (graphQLErrors) {
    graphQLErrors.forEach((i) => {
      const code = i.extensions?.code;
      if (code === "UNAUTHORIZED" || code === "INVALID_TOKEN") {
        if (window.location.pathname !== "/auth") {
          window.location.href = `/auth?redirect=${encodeURIComponent(
            window.location.pathname + window.location.search,
          )}`;
        }
      }
    });
  }
}

describe("ErrorLink unauthorized redirect", () => {
  let redirectedUrl = "";

  beforeEach(() => {
    redirectedUrl = "";
    Object.defineProperty(window, "location", {
      writable: true,
      value: {
        pathname: "/",
        search: "?foo=bar",
        get href() {
          return redirectedUrl || "http://localhost/?foo=bar";
        },
        set href(val: string) {
          redirectedUrl = val;
        },
      },
    });
  });

  test("redirects to /auth on UNAUTHORIZED graphql error", () => {
    handleErrors({
      graphQLErrors: [
        {
          extensions: { code: "UNAUTHORIZED" },
        },
      ],
    });

    expect(redirectedUrl).toBe("/auth?redirect=%2F%3Ffoo%3Dbar");
  });
});
