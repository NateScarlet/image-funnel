import { describe, test, expect, vi, beforeEach } from "vitest";

const { mutateMock } = vi.hoisted(() => ({ mutateMock: vi.fn() }));

vi.mock("./client", () => ({
  default: { mutate: mutateMock },
}));

import { getValidToken, tokenStore } from "./tokenManager";
import { CombinedGraphQLErrors } from "@apollo/client/errors";

// 令牌临期（少于 2 分钟）时 getValidToken 会触发刷新
function seedExpiringTokens() {
  tokenStore.value = {
    accessToken: "old-access",
    accessTokenExpiresAt: new Date(Date.now() + 60 * 1000).toISOString(),
    refreshToken: "refresh-1",
    refreshTokenExpiresAt: new Date(Date.now() + 7 * 86400 * 1000).toISOString(),
  };
}

describe("getValidToken", () => {
  let redirectedUrl = "";

  beforeEach(() => {
    mutateMock.mockReset();
    redirectedUrl = "";
    Object.defineProperty(window, "location", {
      writable: true,
      value: {
        pathname: "/",
        search: "",
        get href() {
          return redirectedUrl || "http://localhost/";
        },
        set href(val: string) {
          redirectedUrl = val;
        },
      },
    });
    seedExpiringTokens();
  });

  test("刷新成功时返回新访问令牌", async () => {
    mutateMock.mockResolvedValue({
      data: {
        refreshToken: {
          accessToken: "new-access",
          accessTokenExpiresIn: 900,
          refreshToken: "refresh-2",
          refreshTokenExpiresIn: 86400,
        },
      },
    });

    await expect(getValidToken()).resolves.toBe("new-access");
  });

  test("刷新遇网络错误时按契约抛出，不降级返回可能过期的令牌", async () => {
    // 服务器停机窗口内刷新请求必然失败；getValidToken 的契约是
    // "返回有效令牌或抛出"，消化错误是调用边界的职责
    mutateMock.mockRejectedValue(new TypeError("fetch failed"));

    await expect(getValidToken()).rejects.toThrow("fetch failed");
  });

  test("刷新令牌无效时清除令牌并跳转登录页后抛出", async () => {
    // 构造器入参是响应结果对象，errors 数组放在 result.errors 上
    mutateMock.mockRejectedValue(
      new CombinedGraphQLErrors({
        errors: [{ message: "invalid refresh token", extensions: { code: "INVALID_TOKEN" } }],
      }),
    );

    await expect(getValidToken()).rejects.toBeTruthy();
    expect(tokenStore.value).toBeUndefined();
    expect(redirectedUrl).toBe("/auth?redirect=%2F");
  });
});
