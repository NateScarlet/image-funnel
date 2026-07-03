import { ref } from "vue";
import { startRegistration, startAuthentication } from "@simplewebauthn/browser";
import mutate from "@/graphql/utils/mutate";
import { tokenStore } from "@/graphql/client";
import {
  BeginWebAuthnRegistrationDocument,
  FinishWebAuthnRegistrationDocument,
  BeginWebAuthnLoginDocument,
  FinishWebAuthnLoginDocument,
} from "@/graphql/generated";
import useStorage from "@/composables/useStorage";

// 记录已注册设备的 ID，用于在设备列表中判断设备是否仍存在
// 值为 "unknown" 表示配对请求已创建但尚未审批通过，此时设备 ID 未知
const registeredDeviceId = useStorage<string | null>(
  localStorage,
  "registered_device_id_a1c304",
  null,
);

// 从 URL 中提取 setupToken
function extractSetupToken(): string | null {
  const params = new URL(window.location.href).searchParams;
  let setupToken = params.get("setup_token");
  if (!setupToken) {
    const redirectUrl = params.get("redirect");
    if (redirectUrl) {
      try {
        const u = new URL(redirectUrl, window.location.origin);
        setupToken = u.searchParams.get("setup_token");
      } catch {
        // ignore parsing error
      }
    }
  }
  return setupToken;
}

// 设置 token 并记录设备 ID
function setTokens(
  accessToken: string,
  accessTokenExpiresIn: number | null,
  refreshToken: string,
  refreshTokenExpiresIn: number | null,
  deviceId: string,
) {
  registeredDeviceId.model.value = deviceId;
  tokenStore.value = {
    accessToken,
    accessTokenExpiresAt: accessTokenExpiresIn
      ? new Date(Date.now() + accessTokenExpiresIn * 1000).toISOString()
      : "",
    refreshToken: refreshToken ?? "",
    refreshTokenExpiresAt: refreshTokenExpiresIn
      ? new Date(Date.now() + refreshTokenExpiresIn * 1000).toISOString()
      : "",
  };
}

// 纯登录流程（验证已有凭证），不回退
// 返回 true 表示登录成功，false 表示失败
async function login(): Promise<boolean> {
  const beginRes = await mutate(BeginWebAuthnLoginDocument, {
    variables: { input: {} },
  });
  if (!beginRes.data) return false;
  const { options, sessionKey } = beginRes.data.beginWebAuthnLogin;

  const asseResp = await startAuthentication({
    optionsJSON: options as Parameters<typeof startAuthentication>[0]["optionsJSON"],
  });

  const finishRes = await mutate(FinishWebAuthnLoginDocument, {
    variables: {
      input: {
        sessionKey,
        response: JSON.stringify(asseResp),
      },
    },
  });
  if (!finishRes.data) return false;
  const {
    accessToken,
    accessTokenExpiresIn,
    refreshToken: newRefreshToken,
    refreshTokenExpiresIn,
    device: loginDevice,
  } = finishRes.data.finishWebAuthnLogin;
  setTokens(
    accessToken,
    accessTokenExpiresIn,
    newRefreshToken,
    refreshTokenExpiresIn,
    loginDevice.id,
  );
  return true;
}

// 注册流程（创建新凭证）
// 返回 true 表示注册成功（设置了 token），false 表示需要等待配对审批
async function register(
  setupToken: string | null,
  pairingCode: ReturnType<typeof ref<string>>,
): Promise<boolean> {
  const beginRes = await mutate(BeginWebAuthnRegistrationDocument, {
    variables: { input: { setupToken } },
  });
  if (!beginRes.data) throw new Error("无效的响应");
  const { options, sessionKey } = beginRes.data.beginWebAuthnRegistration;

  let attResp;
  try {
    attResp = await startRegistration({
      optionsJSON: options as Parameters<typeof startRegistration>[0]["optionsJSON"],
    });
  } catch (err) {
    console.error("regisration failed, try login", err);
    return login();
  }

  const finishRes = await mutate(FinishWebAuthnRegistrationDocument, {
    variables: {
      input: {
        sessionKey,
        response: JSON.stringify(attResp),
        setupToken,
      },
    },
  });
  if (!finishRes.data) throw new Error("无效的响应");
  const {
    accessToken,
    accessTokenExpiresIn,
    refreshToken: newRefreshToken,
    refreshTokenExpiresIn,
    device,
  } = finishRes.data.finishWebAuthnRegistration;
  if (accessToken && device) {
    setTokens(
      accessToken,
      accessTokenExpiresIn,
      newRefreshToken ?? "",
      refreshTokenExpiresIn,
      device.id,
    );
    return true;
  }

  const pairingReq = finishRes.data.finishWebAuthnRegistration.pairingRequest;
  if (pairingReq) {
    registeredDeviceId.model.value = "unknown";
    pairingCode.value = pairingReq.code;
    return false;
  }

  return false;
}

// 登录流程，失败时回退到注册
// 返回 true 表示成功（登录或注册成功），false 表示需要等待配对审批
async function tryLogin(
  setupToken: string | null,
  pairingCode: ReturnType<typeof ref<string>>,
): Promise<boolean> {
  try {
    return await login();
  } catch {
    // 登录失败，回退到注册
    return register(setupToken, pairingCode);
  }
}

export function useAuthenticate() {
  const errorMsg = ref("");
  const pairingCode = ref("");
  const isActionLoading = ref(false);

  async function authenticate(
    onSuccess?: () => void | Promise<void>,
    deviceList?: { id: string; isCurrent?: boolean | null }[],
  ) {
    errorMsg.value = "";
    isActionLoading.value = true;
    try {
      const setupToken = extractSetupToken();
      const storedId = registeredDeviceId.model.value;

      // 如果提供了设备列表，仅当存储的 ID 仍在列表中时才尝试登录；否则只要注册过就尝试登录
      const shouldTryLogin = storedId && (!deviceList || deviceList.some((d) => d.id === storedId));

      const success = shouldTryLogin
        ? await tryLogin(setupToken, pairingCode)
        : await register(setupToken, pairingCode);

      if (success && onSuccess) {
        await onSuccess();
      }
    } catch (err: unknown) {
      errorMsg.value = err instanceof Error ? err.message : "认证失败";
      console.error(err);
    } finally {
      isActionLoading.value = false;
    }
  }

  return {
    authenticate,
    errorMsg,
    pairingCode,
    isActionLoading,
    registeredDeviceId,
  };
}
