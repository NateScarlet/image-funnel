import useStorage from "@/composables/useStorage";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import withWebLock from "@/utils/withWebLock";
import { RefreshTokenDocument } from "./generated";
import client from "./client";
import { CombinedGraphQLErrors } from "@apollo/client/errors";

interface TokenStore {
  accessToken: string;
  accessTokenExpiresAt: string;
  refreshToken: string;
  refreshTokenExpiresAt: string;
}

export const { model: tokenStore } = useStorage<TokenStore>(
  localStorage,
  "token_store_8b54ed",
);

const flight = new SingleFlightGroup<void>();
const lockKey = "refresh_token_lock_8b54ed";

export async function getValidToken(): Promise<string | undefined> {
  if (!tokenStore.value) {
    return;
  }

  const expiresAt = new Date(tokenStore.value.accessTokenExpiresAt);
  const now = new Date();
  // 若访问令牌快过期（少于 2 分钟），提前刷新
  if (expiresAt.getTime() - now.getTime() < 2 * 60 * 1000) {
    await refreshToken();
  }
  return tokenStore.value.accessToken;
}

export async function refreshToken() {
  const refreshToken = tokenStore.value?.refreshToken;
  if (!refreshToken) return;

  await flight.do(refreshToken, async () => {
    await withWebLock(lockKey, async () => {
      if (tokenStore.value?.refreshToken != refreshToken) {
        // 等待期间已经刷新了
        return;
      }
      try {
        const res = await client.mutate({
          mutation: RefreshTokenDocument,
          variables: {
            input: {
              refreshToken,
            },
          },
          context: {
            suppressError: true,
            anonymous: true,
          },
        });
        const error = res.error;
        if (error && CombinedGraphQLErrors.is(error)) {
          const hasAuthError = error.errors.some((e) => {
            const code = e.extensions?.code;
            return code === "INVALID_TOKEN" || code === "UNAUTHORIZED";
          });
          if (hasAuthError) {
            // 当遭遇无效的或未授权的刷新令牌时，清除令牌数据并自动重定向到登录页面以防止重复发生死循环式请求
            tokenStore.value = undefined;
            if (window.location.pathname !== "/auth") {
              window.location.href = `/auth?redirect=${encodeURIComponent(
                window.location.pathname + window.location.search,
              )}`;
            }
            throw res.error;
          }
        }
        if (res.data?.refreshToken) {
          const {
            accessToken: newAccessToken,
            accessTokenExpiresIn: newAccessExpiresIn,
            refreshToken: newRefreshToken,
            refreshTokenExpiresIn: newRefreshExpiresIn,
          } = res.data.refreshToken;
          tokenStore.value = {
            accessToken: newAccessToken,
            accessTokenExpiresAt: new Date(
              Date.now() + newAccessExpiresIn * 1000,
            ).toISOString(),
            refreshToken: newRefreshToken,
            refreshTokenExpiresAt: new Date(
              Date.now() + newRefreshExpiresIn * 1000,
            ).toISOString(),
          };
        }
      } catch (err) {
        if (err && CombinedGraphQLErrors.is(err)) {
          const hasAuthError = err.errors.some((e) => {
            const code = e.extensions?.code;
            return code === "INVALID_TOKEN" || code === "UNAUTHORIZED";
          });
          if (hasAuthError) {
            // 当遭遇无效的或未授权的刷新令牌时，清除令牌数据并自动重定向到登录页面以防止重复发生死循环式请求
            tokenStore.value = undefined;
            if (window.location.pathname !== "/auth") {
              window.location.href = `/auth?redirect=${encodeURIComponent(
                window.location.pathname + window.location.search,
              )}`;
            }
          }
        }
        throw err;
      }
    });
  });
}
