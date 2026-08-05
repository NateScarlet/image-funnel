<template>
  <div class="min-h-screen bg-primary-900 text-primary-100 p-4 md:p-8">
    <div class="max-w-4xl mx-auto">
      <!-- 顶部工具栏 -->
      <div class="flex justify-end gap-2 mb-4">
        <NotificationCenterButton
          button-class="relative flex items-center gap-2 rounded-lg border border-primary-700 bg-primary-800/80 px-4 py-2 text-sm font-medium text-primary-300 transition-all hover:border-primary-600 hover:bg-primary-700 hover:text-white active:scale-95 cursor-pointer select-none"
        >
          <span class="hidden sm:inline">通知</span>
        </NotificationCenterButton>
        <DeviceManagerButton />
      </div>

      <header class="mb-8">
        <h1 class="text-3xl md:text-4xl font-bold text-center mb-2">
          ImageFunnel
          <span v-if="version" class="text-lg md:text-xl text-primary-400 font-normal ml-2">
            {{ version }}
          </span>
        </h1>
        <p class="text-primary-400 text-center">图片筛选工具</p>
      </header>

      <!-- 目录选择与操作按钮区 -->
      <div class="bg-primary-800 rounded-lg p-6 flex flex-col gap-6">
        <DirectorySelector v-model="selectedDirectoryId" :root-path="rootPath" />

        <!-- 底部按钮区 -->
        <div v-if="selectedDirectoryId" class="flex flex-col sm:flex-row gap-4">
          <!-- 浏览按钮 -->
          <RouterLink
            :to="{
              path: '/browse',
              query: { dir: selectedDirectoryId },
            }"
            title="浏览该目录下的图片"
            class="py-3 px-5 bg-primary-700 hover:bg-primary-600 rounded-lg transition-colors flex items-center justify-center gap-2 border border-primary-600 text-primary-300 hover:text-white no-underline"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiFolder" fill="currentColor" />
            </svg>
            <span>浏览</span>
          </RouterLink>

          <!-- 继续筛选：仅当目录存在服务端仍在的会话时显示 -->
          <RouterLink
            v-if="lastSession"
            :to="{
              name: 'session',
              params: { id: lastSession.id },
            }"
            title="恢复上一次筛选会话"
            class="flex-1 py-3 px-6 bg-secondary-600 hover:bg-secondary-700 rounded-lg font-medium transition-colors flex items-center justify-center gap-2 no-underline text-white"
          >
            <span>继续筛选</span>
          </RouterLink>

          <!-- 开始新筛选 -->
          <button
            :disabled="creatingSession"
            class="flex-1 py-3 px-6 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
            @click="handleStartNew"
          >
            <svg v-if="creatingSession" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <span>{{ creatingSession ? "创建中…" : "开始新筛选" }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 配置弹窗：首次创建目录时弹出 -->
    <configModal.component container-class="sm:max-w-lg">
      <div class="p-6">
        <h2 class="text-xl font-semibold mb-4 text-white">配置筛选会话</h2>
        <CreateSessionForm ref="createFormRef" />
        <div class="flex gap-4 mt-6">
          <button
            class="flex-1 py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-colors"
            @click="configModal.close()"
          >
            取消
          </button>
          <button
            :disabled="creatingSession || !createFormRef?.canCreate"
            class="flex-1 py-3 px-6 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
            @click="handleCreateWithConfig"
          >
            <svg v-if="creatingSession" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <span>{{ creatingSession ? "创建中…" : "开始筛选" }}</span>
          </button>
        </div>
      </div>
    </configModal.component>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import { mdiLoading, mdiFolder } from "@mdi/js";
import useQuery from "../graphql/utils/useQuery";
import { MetaDocument, RootDirectoryDocument } from "../graphql/generated";
import CreateSessionForm from "../components/CreateSessionForm.vue";
import DirectorySelector from "../components/DirectorySelector.vue";
import DeviceManagerButton from "../components/DeviceManagerButton.vue";
import NotificationCenterButton from "../components/NotificationCenterButton.vue";
import useRouteQuery from "../composables/useRouteQuery";
import useSession from "../composables/domain/useSession";
import { useDirectoryState } from "../composables/useDirectoryState";
import { useAutoCreateSession } from "../composables/useAutoCreateSession";
import useModalDialog from "../composables/useModalDialog";
import optionalArray from "../utils/optionalArray";

const router = useRouter();
const loadingCount = ref(0);

const { data: metaData } = useQuery(MetaDocument, {
  loadingCount,
});

const { data: rootData } = useQuery(RootDirectoryDocument, {
  loadingCount,
});

const version = computed(() => metaData.value?.meta?.version || "");
const rootPath = computed(() => metaData.value?.meta?.rootAbsPath || "");

// #region 目录选择与状态
const dirQuery = useRouteQuery("dir");

const selectedDirectoryId = computed({
  get() {
    return dirQuery.value[0] ?? rootData.value?.rootDirectory.id ?? "";
  },
  set(v) {
    if (v === selectedDirectoryId.value) return;
    dirQuery.value = v ? [v] : [];
  },
});

// 查询当前所选目录的会话状态
const { lastSession, lastSessionState } = useDirectoryState(() => selectedDirectoryId.value);

// 自动创建会话（基于目录上次配置）
const { autoCreateSession } = useAutoCreateSession(
  () => selectedDirectoryId.value,
  () => ({}),
  () => 0,
);

// 手动创建会话（首次配置弹窗使用）
const { createSession } = useSession("");
// #endregion

// #region 配置弹窗
const configModal = useModalDialog();
const createFormRef = useTemplateRef<InstanceType<typeof CreateSessionForm>>("createFormRef");
const creatingSession = ref(false);

// 处理"开始新筛选"按钮点击
async function handleStartNew() {
  // 目录有活跃会话或上次配置快照：不弹窗，自动用上次配置创建会话
  if (lastSession.value || lastSessionState.value) {
    creatingSession.value = true;
    try {
      const session = await autoCreateSession();
      if (session) {
        await router.push({ name: "session", params: { id: session.id } });
      }
    } finally {
      creatingSession.value = false;
    }
    return;
  }

  // 首次创建：弹出配置弹窗
  configModal.open();
}

// 处理配置弹窗中的"开始筛选"按钮
async function handleCreateWithConfig() {
  const form = createFormRef.value;
  if (!form) return;

  creatingSession.value = true;
  try {
    const filterRating = optionalArray(form.filterRating?.slice());
    const session = await createSession({
      directoryId: selectedDirectoryId.value,
      filter: {
        rating: filterRating,
      },
      targetKeep: form.targetKeep,
      createActions: form.selectedPreset?.writeActions,
    });

    if (session) {
      configModal.close();
      await router.push({ name: "session", params: { id: session.id } });
    }
  } finally {
    creatingSession.value = false;
  }
}
// #endregion
</script>
