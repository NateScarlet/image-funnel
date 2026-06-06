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
const { data: authResult, refresh: refreshAuth } = useQuery(
  AuthStatusDocument,
  {
    loadingCount: authLoadingCount,
    fetchPolicy: "network-only",
  },
);

const { data: metaResult } = useQuery(MetaDocument, {
  fetchPolicy: "cache-first",
});

const authStatus = computed(() => authResult.value?.authStatus);
const meta = computed(() => metaResult.value?.meta);

const isTrustedDevice = computed(
  () => authStatus.value?.isTrustedDevice ?? false,
);

const supportsWebAuthn = browserSupportsWebAuthn();

watch(
  isTrustedDevice,
  (val) => {
    if (val) {
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
  },
  { immediate: true },
);

const baseURL = computed(() => meta.value?.baseURL);

const {
  authenticate: rawAuthenticate,
  errorMsg,
  pairingCode,
  isActionLoading,
} = useAuthenticate();

async function authenticate() {
  await rawAuthenticate(async () => {
    await refreshAuth();
    const redirect = router.currentRoute.value.query.redirect as string;
    router.push(redirect || "/");
  });
}

useSubscription(PairingRequestUpdatedDocument, {
  variables: computed(() =>
    pairingCode.value ? { code: pairingCode.value } : undefined,
  ),
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
  <div
    class="flex h-screen w-full items-center justify-center bg-primary-900 p-4"
  >
    <div
      class="w-full max-w-md rounded-2xl bg-primary-800 p-8 shadow-xl border border-primary-700/50"
    >
      <h1 class="mb-6 text-center text-3xl font-bold text-primary-100">
        Image Funnel
      </h1>

      <div v-if="authLoadingCount > 0" class="flex justify-center py-8">
        <div
          class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <div
        v-else-if="!supportsWebAuthn && !isTrustedDevice"
        class="space-y-6 text-center"
      >
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
          <h2 class="mb-2 text-xl font-semibold text-secondary-100">
            无法进行安全认证
          </h2>
          <p class="text-sm">
            您的当前环境不支持 WebAuthn，无法进行设备认证（可能是因为您未使用
            HTTPS 或者未通过 localhost 访问）。
          </p>
        </div>

        <div class="space-y-4 text-left">
          <p class="font-medium text-primary-200">
            您可以尝试以下方法继续访问：
          </p>
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
              <code
                class="bg-primary-950 px-1 py-0.5 rounded text-secondary-300 font-mono text-xs"
                >IMAGE_FUNNEL_TRUSTED_IP</code
              >
              环境变量中。
            </li>
            <li>
              如果您在使用反向代理，请确保其支持 HTTPS，或配置
              <code
                class="bg-primary-950 px-1 py-0.5 rounded text-secondary-300 font-mono text-xs"
                >IMAGE_FUNNEL_TRUSTED_PROXY</code
              >
              以传递真实 IP。
            </li>
          </ul>
        </div>
      </div>

      <div v-else class="text-center">
        <div v-if="pairingCode" class="space-y-6">
          <h2 class="text-xl font-semibold text-primary-100">等待设备配对</h2>
          <p class="text-primary-300 text-sm">
            请在已登录的设备上接受以下配对请求：
          </p>
          <div
            class="rounded-lg bg-primary-950 border border-primary-700/50 p-4 text-center"
          >
            <span
              class="font-mono text-3xl font-bold tracking-widest text-secondary-500"
              >{{ pairingCode }}</span
            >
          </div>
          <button
            class="mt-4 text-sm text-primary-400 hover:text-primary-200 transition-colors"
            @click="pairingCode = ''"
          >
            取消配对
          </button>
        </div>

        <div v-else class="space-y-4">
          <p class="mb-4 text-sm text-primary-300">
            当前设备需要通过安全认证才能继续访问。
          </p>

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
            {{ isActionLoading ? "处理中..." : "登录" }}
          </button>
          <p class="mt-2 text-left text-xs text-primary-400">
            提示：注册新设备时，浏览器可能会弹出 1–3
            次系统确认请求，请按照提示操作直到全部完成。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
