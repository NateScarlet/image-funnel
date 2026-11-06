<script setup lang="ts">
import { computed, watch } from "vue";
import { formatDistanceToNow } from "date-fns";
import { zhCN } from "date-fns/locale";
import mutate from "@/graphql/utils/mutate";
import {
  ApprovePairingRequestDocument,
  RejectPairingRequestDocument,
  DeleteDeviceDocument,
} from "@/graphql/generated";
import type { DeviceFragment } from "@/graphql/generated";
import { forgetTokens } from "@/graphql/tokenManager";
import useModalDrawer from "@/composables/useModalDrawer";
import { useDevices } from "@/composables/useDevices";
import { useAuthenticate } from "@/composables/useAuthenticate";

const {
  visible: globalVisible,
  devices,
  devicesLoaded,
  pairingRequests,
  isTrustedDevice,
  refreshDevices,
  refreshAuthStatus,
  close: closeGlobal,
} = useDevices();

const drawer = useModalDrawer({
  onDidClose() {
    closeGlobal();
  },
});

// 当前设备是否存在，取决于设备列表中是否有条目被标记为当前设备。
// 不能用 isTrustedDevice 判断：删除当前设备后浏览器仍持有未过期的访问令牌，
// 此时请求仍被当作可信设备访问，但设备记录已经不存在，会导致注册入口被永久隐藏。
const currentDevice = computed(() => devices.value.find((device) => device.isCurrent));

// 设备列表加载完成前无法断言当前设备未注册，否则每次打开抽屉都会先闪一下注册入口
const showRegisterPanel = computed(() => devicesLoaded.value && !currentDevice.value);

// 未注册的两种成因：当前根本不可信（依赖信任 IP 访问），或访问令牌对应的设备已被删除
const registerHint = computed(() => {
  if (isTrustedDevice.value === true) {
    return "当前会话仍被识别为可信设备访问，但访问令牌对应的设备记录已被删除，因此本机不会出现在已配对设备中。重新注册当前设备即可恢复受信任状态。";
  }
  return "您目前通过信任 IP 访问，但当前设备尚未注册凭证。注册后可以在非信任网络下安全访问。";
});

// 监听全局可见性状态以控制抽屉的弹出/关闭
watch(globalVisible, (val) => {
  if (val && !drawer.visible.value) {
    drawer.open();
  } else if (!val && drawer.visible.value) {
    drawer.close();
  }
});

async function approveRequest(code: string) {
  await mutate(ApprovePairingRequestDocument, {
    variables: { input: { code } },
  });
  pairingRequests.value = pairingRequests.value.filter((req) => req.code !== code);
  refreshDevices();
}

async function rejectRequest(code: string) {
  await mutate(RejectPairingRequestDocument, {
    variables: { input: { code } },
  });
  pairingRequests.value = pairingRequests.value.filter((req) => req.code !== code);
}

async function deleteDevice(device: DeviceFragment) {
  if (!confirm("确定要删除此设备吗？删除后该设备将被强制登出。")) return;
  await mutate(DeleteDeviceDocument, { variables: { input: { id: device.id } } });
  if (device.isCurrent) {
    // 删除的是当前设备：本机令牌对应的设备已不存在，继续持有令牌只会被服务端当作可信设备，
    // 反而让本机无法重新注册，因此立即清除本机令牌（强制登出）
    forgetTokens();
  }
  refreshDevices();
  refreshAuthStatus();
}

function formatDate(isoStr: string) {
  return formatDistanceToNow(new Date(isoStr), {
    addSuffix: true,
    locale: zhCN,
  });
}

const {
  authenticate,
  errorMsg: authError,
  pairingCode,
  isActionLoading: authLoading,
} = useAuthenticate();

async function handleRegister() {
  await authenticate(async () => {
    refreshDevices();
    refreshAuthStatus();
  }, devices.value);
}
</script>

<template>
  <drawer.component
    container-class="w-full max-w-md md:max-w-2xl bg-primary-800 border-l border-primary-700 p-6 overflow-y-auto overflow-x-hidden shadow-2xl flex flex-col h-full text-left"
  >
    <div class="p-6">
      <div class="mb-6">
        <h2 class="text-xl font-bold text-primary-100">设备管理</h2>
      </div>

      <div v-if="showRegisterPanel" class="mb-6">
        <div class="rounded-xl border border-primary-700/60 bg-primary-900/30 p-4">
          <h3 class="mb-2 text-sm font-semibold text-primary-100">
            {{ isTrustedDevice === true ? "当前设备不在已配对设备中" : "当前设备未注册" }}
          </h3>
          <p class="mb-4 text-xs text-primary-400">{{ registerHint }}</p>
          <div
            v-if="pairingCode"
            class="rounded-lg bg-primary-950 border border-primary-700/50 p-3"
          >
            <p class="mb-2 text-xs text-primary-300">
              已创建配对请求，请在其他已登录的设备上接受以下配对码：
            </p>
            <span class="font-mono text-2xl font-bold tracking-widest text-secondary-500">{{
              pairingCode
            }}</span>
            <button
              class="mt-2 block text-xs text-primary-400 hover:text-primary-200 transition-colors cursor-pointer"
              @click="pairingCode = ''"
            >
              取消配对
            </button>
          </div>
          <template v-else>
            <div
              v-if="authError"
              class="mb-3 rounded-lg bg-error-950/40 border border-error-800/50 p-2 text-xs text-error-200"
            >
              {{ authError }}
            </div>
            <button
              :disabled="authLoading"
              class="inline-block rounded-xl bg-primary-700 hover:bg-primary-600 px-4 py-2 text-sm font-medium text-primary-100 shadow-md transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
              @click="handleRegister"
            >
              {{ authLoading ? "处理中…" : "注册当前设备" }}
            </button>
            <p class="mt-2 text-xs text-primary-400">
              提示：注册新设备时，浏览器可能会弹出 1–3 次系统确认请求，请按照提示操作直到全部完成。
            </p>
          </template>
        </div>
      </div>

      <div v-if="pairingRequests.length > 0" class="mb-8">
        <h3 class="mb-4 text-xs font-semibold uppercase tracking-wider text-primary-400">
          配对请求
        </h3>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div
            v-for="req in pairingRequests"
            :key="req.code"
            class="rounded-xl border border-yellow-500/20 bg-yellow-500/5 p-4 shadow-md transition-all hover:border-yellow-500/40"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="font-mono text-lg font-bold tracking-widest text-yellow-400">{{
                req.code
              }}</span>
              <span class="text-xs text-yellow-500/80">{{ formatDate(req.createdAt) }}</span>
            </div>
            <p class="mb-4 text-xs text-yellow-200/80">
              新设备正在请求配对，请核对设备上的配对码。
            </p>
            <div class="flex gap-2">
              <button
                class="flex-1 rounded-xl bg-yellow-600 py-2 text-xs font-semibold text-white transition-all hover:bg-yellow-500 hover:shadow-lg active:scale-95 cursor-pointer"
                @click="approveRequest(req.code)"
              >
                允许
              </button>
              <button
                class="flex-1 rounded-xl bg-primary-800/80 border border-primary-700 py-2 text-xs font-semibold text-primary-200 transition-all hover:bg-primary-700 hover:text-white active:scale-95 cursor-pointer"
                @click="rejectRequest(req.code)"
              >
                拒绝
              </button>
            </div>
          </div>
        </div>
      </div>

      <div>
        <h3 class="mb-4 text-xs font-semibold uppercase tracking-wider text-primary-400">
          已配对设备
        </h3>
        <div
          v-if="devices.length === 0"
          class="text-center text-sm text-primary-400 py-6 border border-dashed border-primary-700/60 rounded-xl bg-primary-900/10"
        >
          暂无设备信息
        </div>
        <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div
            v-for="device in devices"
            :key="device.id"
            :class="[
              'flex items-center justify-between rounded-xl border p-4 shadow-md transition-all',
              device.isCurrent
                ? 'border-emerald-500 bg-emerald-950/20 shadow-emerald-500/5'
                : 'border-primary-700/60 bg-primary-900/30 hover:border-primary-600 hover:bg-primary-900/50',
            ]"
          >
            <div class="min-w-0 flex-1 pr-2 text-left">
              <div
                class="font-semibold text-primary-100 truncate flex items-center gap-2"
                :title="device.name"
              >
                <span class="truncate">{{ device.name }}</span>
                <span
                  v-if="device.isCurrent"
                  class="shrink-0 rounded bg-emerald-500/10 px-2 py-0 text-xs font-semibold text-emerald-400 border border-emerald-500/20"
                >
                  当前设备
                </span>
              </div>
              <div class="text-xs text-primary-400 mt-1 space-y-0.5">
                <div>最后登录: {{ formatDate(device.lastLoginAt) }}</div>
                <div class="flex items-center gap-1.5 truncate">
                  <span>IP:</span>
                  <span class="font-mono text-primary-300">{{ device.lastLoginIp || "未知" }}</span>
                </div>
              </div>
            </div>
            <button
              class="rounded-xl p-2 text-primary-400 transition-all hover:bg-red-500/10 hover:text-red-400 active:scale-90 cursor-pointer shrink-0"
              title="删除设备"
              @click="deleteDevice(device)"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </drawer.component>
</template>
