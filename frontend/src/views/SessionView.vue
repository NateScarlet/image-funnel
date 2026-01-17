<template>
  <div class="h-screen bg-slate-900 text-slate-100 flex flex-col overflow-hidden">
    <header class="bg-slate-800 border-b border-slate-700 p-2 md:p-4 flex-shrink-0">
      <div class="max-w-7xl mx-auto flex items-center justify-between">
        <div class="flex-1">
          <div class="text-xs md:text-sm text-slate-400">
            {{ session?.directory || "加载中..." }}
          </div>
          <div class="text-sm md:text-lg font-semibold">
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
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </button>

        <div class="hidden md:flex items-center gap-4">
          <button
            :disabled="!session?.canUndo"
            class="px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors"
            @click="undo"
          >
            撤销
          </button>

          <button
            class="px-4 py-2 rounded-lg font-medium transition-colors bg-red-600 hover:bg-red-700"
            @click="confirmAbandon"
          >
            放弃
          </button>

          <button
            :disabled="!session?.canCommit"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors"
            @click="showCommitModal = true"
          >
            提交
          </button>
        </div>
      </div>
    </header>

    <main class="flex-1 flex items-center justify-center p-2 md:p-4 overflow-hidden">
      <div v-if="loading" class="text-center text-slate-400">加载中...</div>

      <div v-else-if="!session" class="text-center">
        <div class="text-4xl mb-4">🔍</div>
        <h2 class="text-2xl font-bold mb-2">会话不存在</h2>
        <p class="text-slate-400 mb-4">找不到指定的筛选会话</p>
        <button
          class="px-6 py-3 bg-blue-600 hover:bg-blue-700 rounded-lg font-medium"
          @click="router.push('/')"
        >
          返回主页
        </button>
      </div>

      <div v-else-if="isCompleted" class="text-center">
        <div class="text-4xl mb-4">🎉</div>
        <h2 class="text-2xl font-bold mb-2">筛选完成！</h2>
        <p class="text-slate-400 mb-4">保留了 {{ stats?.kept }} 张图片</p>
        <button
          class="px-6 py-3 bg-blue-600 hover:bg-blue-700 rounded-lg font-medium"
          @click="showCommitModal = true"
        >
          提交更改
        </button>
      </div>

      <div v-else-if="!currentImage" class="text-center text-slate-400">
        没有更多图片
      </div>

      <div v-else class="w-full max-w-5xl flex flex-col items-center h-full min-h-0">
        <div
          class="relative w-full flex-1 bg-slate-800 rounded-lg overflow-hidden mb-2 md:mb-4 min-h-0"
        >
          <ImageViewer
            v-if="currentImage"
            :src="currentImage.url"
            :alt="currentImage.filename"
          />

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
        </div>

        <div class="text-center text-xs md:text-sm text-slate-400 mb-2 md:mb-4">
          {{ currentImage?.filename || "" }}
        </div>

        <div class="hidden md:flex gap-4 w-full max-w-md mb-4">
          <button
            class="btn-action flex-1 py-4 px-6 bg-red-600 hover:bg-red-700 rounded-lg font-bold text-lg"
            @click="markImage('REJECT')"
          >
            排除
          </button>

          <button
            class="btn-action flex-1 py-4 px-6 bg-yellow-600 hover:bg-yellow-700 rounded-lg font-bold text-lg"
            @click="markImage('PENDING')"
          >
            稍后再看
          </button>

          <button
            class="btn-action flex-1 py-4 px-6 bg-green-600 hover:bg-green-700 rounded-lg font-bold text-lg"
            @click="markImage('KEEP')"
          >
            保留
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
          <div class="text-base">{{ session?.filter?.rating?.join(', ') || '无' }}</div>
        </div>

        <div class="space-y-3">
          <button
            :disabled="!session?.canUndo"
            class="w-full py-3 px-4 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors"
            @click="undo(); showMenu = false"
          >
            撤销
          </button>

          <button
            :disabled="!session?.canCommit"
            class="w-full py-3 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors"
            @click="showCommitModal = true; showMenu = false"
          >
            提交
          </button>

          <button
            class="w-full py-3 px-4 bg-red-600 hover:bg-red-700 rounded-lg font-medium transition-colors"
            @click="confirmAbandon"
          >
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
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
import useEventListeners from "../composables/useEventListeners";
import useNotification from "../composables/useNotification";

const route = useRoute();
const router = useRouter();

const sessionId = route.params.id as string;

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);
const showCommitModal = ref<boolean>(false);
const showMenu = ref<boolean>(false);

const { showError } = useNotification();

const touchStartX = ref<number>(0);
const touchStartY = ref<number>(0);
const touchEndX = ref<number>(0);
const touchEndY = ref<number>(0);
const isSingleTouch = ref<boolean>(true);

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
  () => sessionData.value?.session?.currentImage ?? null
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
  } catch (err: unknown) {
    showError(
      "操作失败: " + (err instanceof Error ? err.message : "Unknown error")
    );
  }
}

async function undo() {
  try {
    await mutate(UndoDocument, {
      variables: { input: { sessionId } },
    });
  } catch (err: unknown) {
    showError(
      "撤销失败: " + (err instanceof Error ? err.message : "Unknown error")
    );
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
    e.preventDefault();
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
