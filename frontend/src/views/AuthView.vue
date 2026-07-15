<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useRouter } from "vue-router";
import { browserSupportsWebAuthn } from "@simplewebauthn/browser";
import useQuery from "@/graphql/utils/useQuery";
import {
  AuthStatusDocument,
  MetaDocument,
  PairingRequestUpdatedDocument,
} from "@/graphql/generated";
import useSubscription from "@/graphql/utils/useSubscription";
import { useAuthenticate } from "@/composables/useAuthenticate";

const router = useRouter();

const authLoadingCount = ref(0);
const { data: authResult, refresh: refreshAuth } = useQuery(AuthStatusDocument, {
  loadingCount: authLoadingCount,
  fetchPolicy: "network-only",
});

const { data: metaResult } = useQuery(MetaDocument, {
  fetchPolicy: "cache-first",
});

const authStatus = computed(() => authResult.value?.authStatus);
const meta = computed(() => metaResult.value?.meta);

const isTrustedDevice = computed(() => authStatus.value?.isTrustedDevice ?? false);

const isTrustedIP = computed(() => authStatus.value?.isTrustedIP ?? false);

const supportsWebAuthn = browserSupportsWebAuthn();

function enterApp() {
  let redirect = router.currentRoute.value.query.redirect as string;
  if (redirect) {
    try {
      const u = new URL(redirect, window.location.origin);
      if (u.searchParams.has("setup_token")) {
        u.searchParams.delete("setup_token");
        redirect = u.pathname + u.search;
      }
    } catch {
      // ignore parsing error
    }
    router.push(redirect);
  } else {
    router.push("/");
  }
}

watch(
  isTrustedDevice,
  (val) => {
    if (val) {
      enterApp();
    }
  },
  { immediate: true },
);

const baseURL = computed(() => meta.value?.baseURL);

const { authenticate: rawAuthenticate, errorMsg, pairingCode, isActionLoading } = useAuthenticate();

async function authenticate() {
  await rawAuthenticate(async () => {
    await refreshAuth();
    enterApp();
  });
}

useSubscription(PairingRequestUpdatedDocument, {
  variables: computed(() => (pairingCode.value ? { code: pairingCode.value } : undefined)),
  onNext(result) {
    const update = result.data?.pairingRequestUpdated;
    if (update) {
      pairingCode.value = "";
      if (update.status === "APPROVED") {
        void authenticate();
      } else if (update.status === "REJECTED") {
        errorMsg.value = "配对请求已被拒绝，请重新申请。";
      }
    }
  },
});
</script>

<template>
  <div class="flex h-screen w-full items-center justify-center bg-primary-900 p-4">
    <div
      class="w-full max-w-md rounded-2xl bg-primary-800 p-8 shadow-xl border border-primary-700/50"
    >
      <h1 class="mb-6 text-center text-3xl font-bold text-primary-100">Image Funnel</h1>

      <div v-if="authLoadingCount > 0" class="flex justify-center py-8">
        <div
          class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- 通过可信 IP 访问且未绑定设备 -->
      <div v-else-if="isTrustedIP && !isTrustedDevice" class="space-y-6 text-center">
        <div
          class="rounded-xl bg-success-950/40 border border-success-800/60 p-6 text-success-200 shadow-inner"
        >
          <svg
            class="mx-auto mb-4 h-12 w-12 text-success-500 animate-pulse"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
            />
          </svg>
          <h2 class="mb-2 text-xl font-semibold text-success-100">可信 IP 访问已启用</h2>
          <p class="text-sm">您当前正通过可信 IP 访问，无需登录即可使用应用。</p>
        </div>

        <button
          class="w-full rounded-lg bg-success-600 px-4 py-3 font-semibold text-white transition-all duration-200 hover:bg-success-700 active:bg-success-800 cursor-pointer shadow-lg shadow-success-950/20"
          @click="enterApp"
        >
          直接进入应用
        </button>

        <div class="border-t border-primary-700/50 my-6"></div>

        <div class="space-y-4 text-left">
          <p class="text-sm font-medium text-primary-200">说明：</p>
          <p class="text-xs text-primary-300 leading-relaxed">
            不登录也可使用，但仅限于可信
            IP。若要在其他网络环境（如移动蜂窝网络、外网等）下访问，可以通过以下方式访问：
          </p>
          <ul class="list-disc space-y-2 pl-5 text-primary-300 text-xs leading-relaxed">
            <li>
              将对应环境的 IP 地址配置到服务端的
              <code class="bg-primary-950 px-1 py-0.5 rounded text-secondary-300 font-mono text-xs">
                IMAGE_FUNNEL_TRUSTED_IP
              </code>
              环境变量中。
            </li>
            <li v-if="supportsWebAuthn">
              在此设备上通过下方登录流程注册安全密钥，绑定为受信任设备。绑定后，该设备在任何网络环境下均可直接访问。
            </li>
            <li v-else class="text-primary-400">
              当前浏览器不支持设备认证（WebAuthn）。若想绑定设备，请通过 HTTPS 或使用支持 WebAuthn
              的现代浏览器访问。
            </li>
          </ul>
        </div>

        <!-- 仅在支持 WebAuthn 时提供设备绑定登录入口 -->
        <div v-if="supportsWebAuthn" class="space-y-4 pt-4 border-t border-primary-700/30">
          <div v-if="pairingCode" class="space-y-4 text-center">
            <p class="text-primary-300 text-sm">请在已登录的设备上接受以下配对请求：</p>
            <div class="rounded-lg bg-primary-950 border border-primary-700/50 p-4 text-center">
              <span class="font-mono text-3xl font-bold tracking-widest text-secondary-500">{{
                pairingCode
              }}</span>
            </div>
            <button
              class="text-sm text-primary-400 hover:text-primary-200 transition-colors"
              @click="pairingCode = ''"
            >
              取消配对
            </button>
          </div>
          <div v-else class="space-y-4">
            <div
              v-if="errorMsg"
              class="rounded-lg bg-error-950/40 border border-error-800/50 p-3 text-sm text-error-200"
            >
              {{ errorMsg }}
            </div>
            <button
              :disabled="isActionLoading"
              class="w-full rounded-lg bg-primary-700 px-4 py-2 text-sm font-semibold text-primary-200 transition-colors hover:bg-primary-600 disabled:opacity-50 cursor-pointer"
              @click="authenticate"
            >
              {{ isActionLoading ? "处理中…" : "在此设备上登录以绑定" }}
            </button>
          </div>
        </div>
      </div>

      <!-- 不支持 WebAuthn 且未绑定设备，且不在可信 IP 下访问的视图 -->
      <div v-else-if="!supportsWebAuthn && !isTrustedDevice" class="space-y-6 text-center">
        <div
          class="rounded-xl bg-secondary-950/40 border border-secondary-800/60 p-6 text-secondary-200 shadow-inner"
        >
          <svg
            class="mx-auto mb-4 h-12 w-12 text-secondary-500"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <h2 class="mb-2 text-xl font-semibold text-secondary-100">无法进行安全认证</h2>
          <p class="text-sm">
            您的当前环境不支持 WebAuthn，无法进行设备认证（可能是因为您未使用 HTTPS 或者未通过
            localhost 访问）。
          </p>
        </div>

        <div class="space-y-4 text-left">
          <p class="font-medium text-primary-200">您可以尝试以下方法继续访问：</p>
          <ul class="list-disc space-y-2 pl-5 text-primary-300 text-sm">
            <li v-if="baseURL">
              通过配置的主域名访问：
              <a
                :href="baseURL"
                class="text-secondary-400 hover:text-secondary-300 hover:underline break-all"
                >{{ baseURL }}</a
              >
            </li>
            <li v-else>管理员尚未配置 IMAGE_FUNNEL_BASE_URL。</li>
            <li>
              将您的当前 IP 地址加入到服务端的
              <code class="bg-primary-950 px-1 py-0.5 rounded text-secondary-300 font-mono text-xs"
                >IMAGE_FUNNEL_TRUSTED_IP</code
              >
              环境变量中。
            </li>
            <li>
              如果您在使用反向代理，请确保其支持 HTTPS，或配置
              <code class="bg-primary-950 px-1 py-0.5 rounded text-secondary-300 font-mono text-xs"
                >IMAGE_FUNNEL_TRUSTED_PROXY</code
              >
              以传递真实 IP。
            </li>
          </ul>
        </div>
      </div>

      <!-- 支持 WebAuthn 或已是可信设备 -->
      <div v-else class="text-center">
        <div v-if="pairingCode" class="space-y-6">
          <h2 class="text-xl font-semibold text-primary-100">等待设备配对</h2>
          <p class="text-primary-300 text-sm">请在已登录的设备上接受以下配对请求：</p>
          <div class="rounded-lg bg-primary-950 border border-primary-700/50 p-4 text-center">
            <span class="font-mono text-3xl font-bold tracking-widest text-secondary-500">{{
              pairingCode
            }}</span>
          </div>
          <button
            class="mt-4 text-sm text-primary-400 hover:text-primary-200 transition-colors"
            @click="pairingCode = ''"
          >
            取消配对
          </button>
        </div>

        <div v-else class="space-y-4">
          <p class="mb-4 text-sm text-primary-300">当前设备需要通过安全认证才能继续访问。</p>

          <div
            v-if="errorMsg"
            class="mb-4 rounded-lg bg-error-950/40 border border-error-800/50 p-3 text-sm text-error-200"
          >
            {{ errorMsg }}
          </div>

          <button
            :disabled="isActionLoading"
            class="w-full rounded-lg bg-secondary-600 px-4 py-3 font-semibold text-white transition-colors hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer"
            @click="authenticate"
          >
            {{ isActionLoading ? "处理中…" : "登录" }}
          </button>
          <p class="mt-2 text-left text-xs text-primary-400">
            提示：注册新设备时，浏览器可能会弹出 1–3 次系统确认请求，请按照提示操作直到全部完成。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
