<template>
  <div
    class="h-screen bg-slate-900 text-slate-100 flex flex-col overflow-hidden"
  >
    <SessionHeader
      :session="session"
      :stats="stats"
      :undoing="undoing"
      @show-menu="showMenu = true"
      @undo="undo"
      @show-update-session-modal="showUpdateSessionModal = true"
      @abandon="confirmAbandon"
      @show-commit-modal="showCommitModal = true"
    />

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

      <CompletedView
        v-else-if="isCompleted"
        :session-id="sessionId"
        :stats="stats"
        @committed="onCommitted"
      />

      <div v-else-if="!currentImage" class="text-center text-slate-400">
        没有更多图片
      </div>

      <div v-else class="w-full flex flex-col items-center h-full min-h-0">
        <div
          class="relative w-full flex-1 bg-slate-800 rounded-lg overflow-hidden mb-2 md:mb-4 min-h-0"
        >
          <ImageViewer v-if="currentImage" :image="currentImage">
            <template #info="{ isFullscreen }">
              <span class="lg:min-w-24 hidden md:block">
                {{ formatDate(currentImage.modTime) }}
              </span>
              <template v-if="isFullscreen">
                <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
                <span class="lg:min-w-24">
                  {{ session?.currentIndex || 0 }} /
                  {{ session?.currentSize || 0 }}
                </span>
                <div class="w-px h-4 bg-white/30 mx-1"></div>
                <span class="lg:min-w-24 text-green-400">
                  保留: {{ stats?.kept || 0 }} / {{ session?.targetKeep || 0 }}
                </span>
              </template>
            </template>
          </ImageViewer>

          <SwipeDirectionIndicator
            :direction="swipeDirection"
            :renderer-el="rendererEl"
          />
        </div>

        <div class="text-center text-xs md:text-sm text-slate-400 mb-2 md:mb-4">
          {{ currentImage?.filename || "" }}
        </div>

        <SessionActions :marking="marking" @mark="markImage" />
      </div>
    </main>

    <footer
      class="bg-slate-800 border-t border-slate-700 p-2 text-center text-xs text-slate-400 flex-shrink-0"
    >
      ↓ 排除 | ↑ 稍后再看 | → 保留 | ← 撤销
    </footer>

    <SessionMenu
      v-model:show="showMenu"
      :session="session"
      :can-undo="canUndo"
      :undoing="undoing"
      :session-id="sessionId"
      :stats="stats"
      @abandoned="onAbandoned"
      @show-commit-modal="showCommitModal = true"
      @show-update-session-modal="showUpdateSessionModal = true"
    />

    <CommitModal
      v-if="showCommitModal"
      :session-id="sessionId"
      @close="showCommitModal = false"
      @committed="onCommitted"
    />

    <UpdateSessionModal
      v-if="showUpdateSessionModal"
      :target-keep="session?.targetKeep"
      :filter="{ rating: session?.filter?.rating || [] }"
      :kept="stats?.kept || 0"
      :session-id="sessionId"
      @close="showUpdateSessionModal = false"
      @updated="showUpdateSessionModal = false"
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
import ImageViewer from "../components/ImageViewer.vue";
import SessionHeader from "../components/SessionHeader.vue";
import SessionActions from "../components/SessionActions.vue";
import SessionMenu from "../components/SessionMenu.vue";
import SwipeDirectionIndicator from "../components/SwipeDirectionIndicator.vue";
import CompletedView from "../components/CompletedView.vue";
import CommitModal from "../components/CommitModal.vue";
import UpdateSessionModal from "../components/UpdateSessionModal.vue";
import useEventListeners from "../composables/useEventListeners";
import { formatDate } from "../utils/date";
import { mdiHome } from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import { usePresets } from "../composables/usePresets";

const rendererEl = useFullscreenRendererElement();
const route = useRoute();
const router = useRouter();
usePresets();

const sessionId = route.params.id as string;

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);
const showMenu = ref<boolean>(false);
const showUpdateSessionModal = ref<boolean>(false);
const showCommitModal = ref<boolean>(false);
const undoing = ref(false);
const marking = ref(false);

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
const stats = computed(() => sessionData.value?.session?.stats);
const currentImage = computed(() => sessionData.value?.session?.currentImage);

const isCompleted = computed(() => {
  return stats.value?.isCompleted || false;
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

function onAbandoned() {
  router.push("/");
}

function onCommitted() {
  router.push("/");
}

function confirmAbandon() {
  if (confirm("确定要放弃当前会话吗？所有未提交的更改将会丢失。")) {
    router.push("/");
  }
}

function handleKeyDown(e: KeyboardEvent) {
  if (showMenu.value) return;

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
  if (showMenu.value) return;
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
