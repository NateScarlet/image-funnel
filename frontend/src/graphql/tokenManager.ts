import useStorage from "@/composables/useStorage";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import withWebLock from "@/utils/withWebLock";
import { RefreshTokenDocument } from "./generated";
import client from "./client";

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

async function refreshToken() {
  const refreshToken = tokenStore.value?.refreshToken;
  if (!refreshToken) return;

  await flight.do(refreshToken, async () => {
    await withWebLock(lockKey, async () => {
      if (tokenStore.value?.refreshToken != refreshToken) {
        // 等待期间已经刷新了
        return;
      }
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
    });
  });
}
