<template>
  <div
    class="h-screen bg-slate-900 text-slate-100 flex flex-col overflow-hidden"
  >
    <header
      class="bg-slate-800 border-b border-slate-700 p-2 md:p-4 flex-shrink-0"
    >
      <div class="max-w-7xl mx-auto flex items-center justify-between">
        <div class="flex-1 min-w-0 mr-4">
          <div class="text-xs md:text-sm text-slate-400 truncate">
            {{ session?.directory || "加载中..." }}
          </div>
          <div class="text-sm md:text-lg font-semibold truncate">
            {{ stats?.processed || 0 }} / {{ stats?.total || 0 }}
            <span class="text-green-400 ml-2"
              >保留: {{ stats?.kept || 0 }} /
              {{ session?.targetKeep || 0 }}</span
            >
          </div>
        </div>

        <button
          class="md:hidden p-2 rounded-lg hover:bg-slate-700 transition-colors"
          @click="showMenu = true"
        >
          <svg
            class="w-6 h-6"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path :d="mdiMenu" fill="currentColor" />
          </svg>
        </button>

        <div class="hidden md:flex items-center gap-4">
          <button
            :disabled="!session?.canUndo || undoing"
            class="px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
            @click="undo"
          >
            <svg
              v-if="undoing"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiUndo" fill="currentColor" />
            </svg>
            <span>{{ undoing ? "撤销中..." : "撤销" }}</span>
          </button>

          <button
            class="px-4 py-2 bg-slate-700 hover:bg-slate-600 rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
            @click="showUpdatePresetModal = true"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCogOutline" fill="currentColor" />
            </svg>
            修改预设
          </button>

          <button
            class="px-4 py-2 rounded-lg font-medium transition-colors bg-red-600 hover:bg-red-700 flex items-center gap-2 whitespace-nowrap"
            @click="confirmAbandon"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCloseCircleOutline" fill="currentColor" />
            </svg>
            放弃
          </button>

          <button
            :disabled="!session?.canCommit"
            class="px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
            @click="showCommitModal = true"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheck" fill="currentColor" />
            </svg>
            提交
          </button>
        </div>
      </div>
    </header>

    <main
      class="flex-1 flex items-center justify-center p-2 md:p-4 overflow-hidden"
    >
      <div v-if="loading" class="text-center text-slate-400">加载中...</div>

      <div v-else-if="!session" class="text-center">
        <div class="text-4xl mb-4">🔍</div>
        <h2 class="text-2xl font-bold mb-2">会话不存在</h2>
        <p class="text-slate-400 mb-4">找不到指定的筛选会话</p>
        <button
          class="px-6 py-3 bg-secondary-600 hover:bg-secondary-700 rounded-lg font-medium flex items-center gap-2 whitespace-nowrap"
          @click="router.push('/')"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiHome" fill="currentColor" />
          </svg>
          返回主页
        </button>
      </div>

      <div v-else-if="isCompleted" class="text-center">
        <div class="text-4xl mb-4">🎉</div>
        <h2 class="text-2xl font-bold mb-2">筛选完成！</h2>
        <p class="text-slate-400 mb-4">保留了 {{ stats?.kept }} 张图片</p>
        <button
          class="px-6 py-3 bg-secondary-600 hover:bg-secondary-700 rounded-lg font-medium flex items-center gap-2 whitespace-nowrap"
          @click="showCommitModal = true"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiCheck" fill="currentColor" />
          </svg>
          提交更改
        </button>
      </div>

      <div v-else-if="!currentImage" class="text-center text-slate-400">
        没有更多图片
      </div>

      <div v-else class="w-full flex flex-col items-center h-full min-h-0">
        <div
          class="relative w-full flex-1 bg-slate-800 rounded-lg overflow-hidden mb-2 md:mb-4 min-h-0"
        >
          <ImageViewer
            v-if="currentImage"
            :src="currentImage.url"
            :alt="currentImage.filename"
          >
            <template #info="{ isFullscreen }">
              <span class="lg:min-w-24 hidden md:block">
                {{ formatDate(currentImage.modTime) }}
              </span>
              <template v-if="isFullscreen">
                <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
                <span class="lg:min-w-24">
                  {{ stats?.processed || 0 }} / {{ stats?.total || 0 }}
                </span>
                <div class="w-px h-4 bg-white/30 mx-1"></div>
                <span class="lg:min-w-24 text-green-400">
                  保留: {{ stats?.kept || 0 }} / {{ session?.targetKeep || 0 }}
                </span>
              </template>
            </template>
          </ImageViewer>

          <!-- 滑动方向提示 -->
          <Teleport :to="rendererEl">
            <div
              v-if="swipeDirection"
              class="absolute inset-0 flex items-center justify-center pointer-events-none transition-opacity duration-200"
              :class="{
                'bg-red-600/30': swipeDirection === 'DOWN',
                'bg-yellow-600/30': swipeDirection === 'UP',
                'bg-green-600/30': swipeDirection === 'RIGHT',
                'bg-slate-600/30': swipeDirection === 'LEFT',
              }"
            >
              <div class="text-6xl font-bold text-white drop-shadow-lg">
                <span v-if="swipeDirection === 'DOWN'">↓ 排除</span>
                <span v-else-if="swipeDirection === 'UP'">↑ 稍后再看</span>
                <span v-else-if="swipeDirection === 'RIGHT'">← 撤销</span>
                <span v-else-if="swipeDirection === 'LEFT'">→ 保留</span>
              </div>
            </div>
          </Teleport>
        </div>

        <div class="text-center text-xs md:text-sm text-slate-400 mb-2 md:mb-4">
          {{ currentImage?.filename || "" }}
        </div>

        <div class="hidden md:flex gap-4 w-full max-w-md mb-4">
          <button
            :disabled="marking"
            class="btn-action flex-1 py-4 px-6 bg-red-600 hover:bg-red-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap"
            @click="markImage('REJECT')"
          >
            <svg
              v-if="marking"
              class="w-6 h-6 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
              <path :d="mdiDeleteOutline" fill="currentColor" />
            </svg>
            <span>{{ marking ? "处理中..." : "排除" }}</span>
          </button>

          <button
            :disabled="marking"
            class="btn-action flex-1 py-4 px-6 bg-yellow-600 hover:bg-yellow-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap"
            @click="markImage('PENDING')"
          >
            <svg
              v-if="marking"
              class="w-6 h-6 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
              <path :d="mdiClockOutline" fill="currentColor" />
            </svg>
            <span>{{ marking ? "处理中..." : "稍后再看" }}</span>
          </button>

          <button
            :disabled="marking"
            class="btn-action flex-1 py-4 px-6 bg-green-600 hover:bg-green-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold text-lg flex items-center justify-center gap-2 whitespace-nowrap"
            @click="markImage('KEEP')"
          >
            <svg
              v-if="marking"
              class="w-6 h-6 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-6 h-6" viewBox="0 0 24 24">
              <path :d="mdiHeartOutline" fill="currentColor" />
            </svg>
            <span>{{ marking ? "处理中..." : "保留" }}</span>
          </button>
        </div>
      </div>
    </main>

    <footer
      class="bg-slate-800 border-t border-slate-700 p-2 text-center text-xs text-slate-400 flex-shrink-0"
    >
      ↓ 排除 | ↑ 稍后再看 | → 保留 | ← 撤销
    </footer>

    <div
      v-if="showMenu"
      class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
      @click.self="showMenu = false"
    >
      <div class="bg-slate-800 rounded-lg p-6 w-full max-w-sm">
        <div class="mb-6">
          <h3 class="text-lg font-bold mb-2">会话信息</h3>
          <div class="text-sm text-slate-400 mb-1">筛选条件</div>
          <div class="text-base">
            {{ session?.filter?.rating?.join(", ") || "无" }}
          </div>
        </div>

        <div class="space-y-3">
          <button
            :disabled="!canUndo"
            class="w-full py-3 px-4 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="
              undo();
              showMenu = false;
            "
          >
            <svg
              v-if="undoing"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiUndo" fill="currentColor" />
            </svg>
            <span>{{ undoing ? "撤销中..." : "撤销" }}</span>
          </button>

          <button
            class="w-full py-3 px-4 bg-slate-700 hover:bg-slate-600 rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="
              showUpdatePresetModal = true;
              showMenu = false;
            "
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCogOutline" fill="currentColor" />
            </svg>
            修改预设
          </button>

          <button
            :disabled="!session?.canCommit"
            class="w-full py-3 px-4 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="
              showCommitModal = true;
              showMenu = false;
            "
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheck" fill="currentColor" />
            </svg>
            提交
          </button>

          <button
            class="w-full py-3 px-4 bg-red-600 hover:bg-red-700 rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="confirmAbandon"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCloseCircleOutline" fill="currentColor" />
            </svg>
            放弃
          </button>
        </div>

        <button
          class="mt-4 w-full py-2 px-4 bg-slate-700 hover:bg-slate-600 rounded-lg text-sm transition-colors"
          @click="showMenu = false"
        >
          关闭
        </button>
      </div>
    </div>

    <CommitModal
      v-if="showCommitModal"
      :session-id="sessionId"
      @close="showCommitModal = false"
      @committed="onCommitted"
    />

    <UpdatePresetModal
      v-if="showUpdatePresetModal"
      :target-keep="updateForm.targetKeep"
      :filter="updateForm.filter"
      :kept="stats?.kept || 0"
      :session-id="sessionId"
      @close="showUpdatePresetModal = false"
      @updated="showUpdatePresetModal = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  GetSessionDocument,
  MarkImageDocument,
  UndoDocument,
  ImageAction,
} from "../graphql/generated";
import CommitModal from "../components/CommitModal.vue";
import ImageViewer from "../components/ImageViewer.vue";
import UpdatePresetModal from "../components/UpdatePresetModal.vue";
import useEventListeners from "../composables/useEventListeners";
import { formatDate } from "../utils/date";
import {
  mdiMenu,
  mdiUndo,
  mdiCloseCircleOutline,
  mdiCheck,
  mdiHome,
  mdiDeleteOutline,
  mdiClockOutline,
  mdiHeartOutline,
  mdiLoading,
  mdiCogOutline,
} from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import { usePresets } from "../composables/usePresets";

const rendererEl = useFullscreenRendererElement();
const route = useRoute();
const router = useRouter();
usePresets();

const sessionId = route.params.id as string;

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);
const showCommitModal = ref<boolean>(false);
const showMenu = ref<boolean>(false);
const showUpdatePresetModal = ref<boolean>(false);
const undoing = ref(false);
const marking = ref(false);

const touchStartX = ref<number>(0);
const touchStartY = ref<number>(0);
const touchEndX = ref<number>(0);
const touchEndY = ref<number>(0);
const isSingleTouch = ref<boolean>(true);

// 更新预设表单
const updateForm = ref({
  targetKeep: 4,
  filter: {
    rating: [0, 4],
  },
});

// 监听session变化，同步更新表单
watch(
  () => session.value,
  (newSession) => {
    if (newSession) {
      updateForm.value.targetKeep = newSession.targetKeep;
      updateForm.value.filter.rating = newSession.filter.rating || [];
    }
  },
  { immediate: true, deep: true },
);

const swipeDirection = computed((): "UP" | "DOWN" | "LEFT" | "RIGHT" | null => {
  if (!isSingleTouch.value) return null;

  const deltaX = touchEndX.value - touchStartX.value;
  const deltaY = touchEndY.value - touchStartY.value;
  const minSwipeDistance = 30;

  if (Math.abs(deltaX) > Math.abs(deltaY)) {
    if (Math.abs(deltaX) > minSwipeDistance) {
      return deltaX > 0 ? "RIGHT" : "LEFT";
    }
  } else {
    if (Math.abs(deltaY) > minSwipeDistance) {
      return deltaY > 0 ? "DOWN" : "UP";
    }
  }
  return null;
});

const { data: sessionData } = useQuery(GetSessionDocument, {
  variables: () => ({ id: sessionId }),
  loadingCount,
});

const session = computed(() => sessionData.value?.session);
const stats = computed(() => sessionData.value?.session?.stats ?? null);
const currentImage = computed(
  () => sessionData.value?.session?.currentImage ?? null,
);

const isCompleted = computed(() => {
  return session.value?.status === "COMPLETED";
});

onMounted(() => {
  useEventListeners(window, ({ on }) => {
    on("keydown", handleKeyDown);
    on("touchstart", handleTouchStart, { passive: false });
    on("touchmove", handleTouchMove, { passive: false });
    on("touchend", handleTouchEnd, { passive: true });
  });
});

async function markImage(action: "REJECT" | "PENDING" | "KEEP") {
  if (!currentImage.value) return;

  marking.value = true;

  try {
    await mutate(MarkImageDocument, {
      variables: {
        input: {
          sessionId,
          imageId: currentImage.value.id,
          action: action as ImageAction,
        },
      },
    });
  } finally {
    marking.value = false;
  }
}

const canUndo = computed(() => session.value?.canUndo && !undoing.value);
async function undo() {
  if (!canUndo.value) return;
  undoing.value = true;

  try {
    await mutate(UndoDocument, {
      variables: { input: { sessionId } },
    });
  } finally {
    undoing.value = false;
  }
}

function confirmAbandon() {
  showMenu.value = false;
  if (confirm("确定要放弃当前会话吗？所有未提交的更改将会丢失。")) {
    router.push("/");
  }
}

function onCommitted() {
  showCommitModal.value = false;
  router.push("/");
}

function handleKeyDown(e: KeyboardEvent) {
  if (showCommitModal.value || showMenu.value) return;

  switch (e.key) {
    case "ArrowDown":
      markImage("REJECT");
      break;
    case "ArrowUp":
      markImage("PENDING");
      break;
    case "ArrowRight":
      markImage("KEEP");
      break;
    case "ArrowLeft":
      undo();
      break;
  }
}

function handleTouchStart(e: TouchEvent) {
  isSingleTouch.value = e.touches.length === 1;
  touchStartX.value = e.changedTouches[0].screenX;
  touchStartY.value = e.changedTouches[0].screenY;
  touchEndX.value = touchStartX.value;
  touchEndY.value = touchStartY.value;

  if (isSingleTouch.value) {
    const target = e.target as HTMLElement;
    const isButton = target.closest("button, a, input, select, textarea");
    if (!isButton) {
      e.preventDefault();
    }
  }
}

function handleTouchMove(e: TouchEvent) {
  isSingleTouch.value = e.touches.length === 1;
  touchEndX.value = e.changedTouches[0].screenX;
  touchEndY.value = e.changedTouches[0].screenY;

  const deltaX = touchEndX.value - touchStartX.value;
  const deltaY = touchEndY.value - touchStartY.value;

  if (Math.abs(deltaY) > Math.abs(deltaX) && Math.abs(deltaY) > 10) {
    e.preventDefault();
  }
}

function handleTouchEnd(e: TouchEvent) {
  touchEndX.value = e.changedTouches[0].screenX;
  touchEndY.value = e.changedTouches[0].screenY;
  handleGesture();

  // 重置触摸坐标，清除滑动方向
  setTimeout(() => {
    touchEndX.value = touchStartX.value;
    touchEndY.value = touchStartY.value;
  }, 100);
}

function handleGesture() {
  if (showCommitModal.value || showMenu.value) return;
  if (!swipeDirection.value) return;

  const minSwipeDistance = 50;
  const deltaX = touchEndX.value - touchStartX.value;
  const deltaY = touchEndY.value - touchStartY.value;

  if (
    Math.abs(deltaX) < minSwipeDistance &&
    Math.abs(deltaY) < minSwipeDistance
  ) {
    return;
  }

  switch (swipeDirection.value) {
    case "UP":
      markImage("PENDING");
      break;
    case "DOWN":
      markImage("REJECT");
      break;
    case "LEFT":
      markImage("KEEP");
      break;
    case "RIGHT":
      undo();
      break;
  }
}
</script>
