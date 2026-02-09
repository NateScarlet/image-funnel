<template>
  <div
    class="h-screen bg-primary-900 text-primary-100 flex flex-col overflow-hidden"
  >
    <SessionHeader
      :session
      :undoing="undoing"
      @show-update-session-modal="showUpdateSessionModal = true"
      @show-commit-modal="handleCommit"
      @undo="undo"
    >
    </SessionHeader>

    <main
      class="flex-1 w-full p-2 md:p-4"
      :class="
        currentImage
          ? 'flex flex-col items-center justify-center overflow-hidden'
          : 'overflow-y-auto'
      "
    >
      <KeepAlive>
        <ImageViewer
          v-if="currentImage"
          class="relative w-full flex-1 bg-primary-800 rounded-lg overflow-hidden"
          :image="currentImage"
          :next-images="session?.nextImages ?? []"
          :allow-pan="handleAllowPan"
          @image-loaded="(e) => (lastImageLoadedEvent = e)"
        >
          <template #progress>
            <div v-if="session" class="h-1 bg-black/20 pointer-events-none">
              <div
                class="h-full transition-all duration-300 ease-out"
                :class="progressClass"
                :style="{
                  width: `${Math.min(100, Math.max(0, progress))}%`,
                }"
              ></div>
            </div>
          </template>
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
                保留: {{ session?.stats.kept || 0 }} /
                {{ session?.targetKeep || 0 }}
              </span>
            </template>
          </template>
        </ImageViewer>
      </KeepAlive>

      <Teleport v-if="session" :to="rendererEl">
        <div
          ref="swipeEl"
          class="fixed bottom-0 left-0 right-0 top-1/2 overflow-hidden pointer-events-none z-20"
        >
          <Transition
            enter-active-class="transition duration-100 ease-out"
            enter-from-class="opacity-0"
            enter-to-class="opacity-100"
            leave-active-class="transition duration-100 ease-in"
            leave-from-class="opacity-100"
            leave-to-class="opacity-0"
          >
            <SwipeDirectionIndicator
              v-if="swipeDirection"
              class="h-full w-full"
              :direction="swipeDirection"
              :renderer-el="rendererEl"
            />
          </Transition>
        </div>
      </Teleport>

      <template v-if="currentImage">
        <div
          class="text-center text-xs md:text-sm text-primary-400 hidden md:block"
        >
          {{ currentImage?.filename || "" }}
        </div>

        <SessionActions
          v-if="!didUseGesture"
          class="hidden md:flex gap-4 w-full max-w-md mb-4"
          :marking="marking"
          @mark="markImage"
        />
      </template>

      <div
        v-else
        class="min-h-full flex flex-col items-center justify-center w-full"
      >
        <template v-if="loading">
          <div class="text-center text-primary-400">加载中...</div>
        </template>
        <template v-else-if="!session">
          <div class="text-center">
            <div class="text-4xl mb-4">🔍</div>
            <h2 class="text-2xl font-bold mb-2">会话不存在</h2>
            <p class="text-primary-400 mb-4">找不到指定的筛选会话</p>
            <button
              class="px-6 py-3 bg-secondary-600 hover:bg-secondary-700 rounded-lg font-medium flex items-center gap-2 whitespace-nowrap mx-auto"
              @click="router.push('/')"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiHome" fill="currentColor" />
              </svg>
              返回主页
            </button>
          </div>
        </template>
        <template v-else>
          <CompletedView ref="completedView" :session @undo="undo" />
        </template>
      </div>
    </main>

    <footer
      v-if="currentImage"
      class="bg-primary-800 border-t border-primary-700 p-2 text-center text-xs text-primary-400 shrink-0"
      :class="didUseGesture ? 'hidden' : ''"
    >
      ↓ 排除 | ↑ 搁置 | → 保留 | ← 撤销
    </footer>

    <CommitModal
      v-if="showCommitModal"
      :session
      @close="showCommitModal = false"
      @committed="showCommitModal = false"
    />

    <UpdateSessionModal
      v-if="showUpdateSessionModal && session"
      :session
      @close="showUpdateSessionModal = false"
      @updated="showUpdateSessionModal = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, computed, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import mutate from "../graphql/utils/mutate";
import { UndoDocument, ImageAction } from "../graphql/generated";
import ImageViewer from "../components/ImageViewer.vue";
import SessionHeader from "../components/SessionHeader.vue";
import SessionActions from "../components/SessionActions.vue";

import SwipeDirectionIndicator from "../components/SwipeDirectionIndicator.vue";
import CompletedView from "../components/CompletedView.vue";
import CommitModal from "../components/CommitModal.vue";
import UpdateSessionModal from "../components/UpdateSessionModal.vue";
import useEventListeners from "../composables/useEventListeners";
import { formatDate } from "../utils/date";
import { mdiHome } from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import useSession from "../composables/useSession";
import useMarkImage from "@/composables/useMarkImage";
import Time from "@/utils/Time";

const rendererEl = useFullscreenRendererElement();
const router = useRouter();

const props = defineProps<{
  id: string;
}>();

const sessionId = computed(() => props.id);

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);

const showUpdateSessionModal = ref<boolean>(false);
const showCommitModal = ref<boolean>(false);
const undoing = ref(false);

// TODO: refactor to touchStart touchEnd
// TODO: 移除多余的类型标注
const touchStartX = ref<number>(0);
const touchStartY = ref<number>(0);
const touchEndX = ref<number>(0);
const touchEndY = ref<number>(0);
const swiping = ref<boolean>(false);

const SWIPE_THRESHOLD = 50;

const swipeDirection = computed((): "UP" | "DOWN" | "LEFT" | "RIGHT" | null => {
  if (!swiping.value) return null;

  const deltaX = touchEndX.value - touchStartX.value;
  const deltaY = touchEndY.value - touchStartY.value;

  if (Math.abs(deltaX) > Math.abs(deltaY)) {
    if (Math.abs(deltaX) > SWIPE_THRESHOLD) {
      return deltaX > 0 ? "RIGHT" : "LEFT";
    }
  } else if (currentImage.value) {
    if (Math.abs(deltaY) > SWIPE_THRESHOLD) {
      return deltaY > 0 ? "DOWN" : "UP";
    }
  }
  return null;
});

const { session } = useSession(sessionId, { loadingCount });

const progress = computed(() => {
  if (!session.value || session.value.currentSize === 0) return 0;
  return (session.value.currentIndex / session.value.currentSize) * 100;
});

const progressClass = computed(() => {
  const kept = session.value?.stats.kept || 0;
  const target = session.value?.targetKeep || 0;

  if (kept === 0) {
    return "bg-primary-500";
  }
  if (kept <= target) {
    return "bg-success-500";
  }
  return "bg-secondary-500";
});

const currentImage = computed(() => session.value?.currentImage ?? undefined);

const swipeEl = useTemplateRef("swipeEl");
useEventListeners(window, ({ on }) => {
  on("keydown", (e) => {
    switch (e.key) {
      case "ArrowDown":
        markImage(ImageAction.REJECT);
        break;
      case "ArrowUp":
        markImage(ImageAction.SHELVE);
        break;
      case "ArrowRight":
        markImage(ImageAction.KEEP);
        break;
      case "ArrowLeft":
        undo();
        break;
    }
  });
  on(
    "touchstart",
    (e) => {
      const touch = e.changedTouches[0];
      if (e.touches.length !== 1 || !insideSwipeArea(touch)) {
        // 只支持单指操作
        return;
      }
      if (
        document
          .elementsFromPoint(touch.clientX, touch.clientY)
          .some(
            (el) =>
              el.hasAttribute("data-no-gesture") ||
              el.role == "input" ||
              el.tagName == "BUTTON" ||
              el.tagName == "INPUT" ||
              el.tagName == "TEXTAREA" ||
              el.tagName == "SELECT",
          )
      ) {
        // 避免干扰交互区域
        return;
      }

      if (currentImage.value) {
        e.preventDefault();
      }
      swiping.value = true;
      touchStartX.value = touch.clientX;
      touchStartY.value = touch.clientY;
      touchEndX.value = touchStartX.value;
      touchEndY.value = touchStartY.value;
    },
    { passive: false },
  );
  on(
    "touchmove",
    (e) => {
      if (!swiping.value) {
        return;
      }
      if (e.touches.length > 1) {
        // 用户想要进行其他操作
        swiping.value = false;
        return;
      }
      const touch = e.changedTouches[0];
      const deltaX = touch.clientX - touchStartX.value;
      const deltaY = touch.clientY - touchStartY.value;

      // 如果有当前图片，阻止默认行为（滚动）
      // 如果没有当前图片（完成状态），只在水平滑动时阻止默认行为，允许垂直滚动
      if (currentImage.value) {
        if (e.cancelable) e.preventDefault();
      } else if (Math.abs(deltaX) > Math.abs(deltaY)) {
        if (e.cancelable) e.preventDefault();
      }

      touchEndX.value = touch.clientX;
      touchEndY.value = touch.clientY;
    },
    { passive: false },
  );
  on(
    "touchend",
    (e) => {
      if (!swiping.value) {
        return;
      }
      // 在完成状态下，仅当识别到水平手势（如撤销）或有当前图片时才阻止默认行为，
      // 以防止干扰按钮点击等正常交互，同时仍然保留手势功能。
      if (e.cancelable) {
        if (currentImage.value || swipeDirection.value) {
          e.preventDefault();
        }
      }

      touchEndX.value = e.changedTouches[0].clientX;
      touchEndY.value = e.changedTouches[0].clientY;
      handleGesture();
      swiping.value = false;
    },
    { passive: false },
  );
  on("touchcancel", () => {
    swiping.value = false;
  });
});

const lastImageLoadedEvent = shallowRef<{ id: string; time: Time }>();
const imageLoadedAt = computed(() => {
  const event = lastImageLoadedEvent.value;
  if (event && event.id === currentImage.value?.id) {
    return event.time;
  }
  return undefined;
});
const { marking, mark } = useMarkImage(sessionId, currentImage, imageLoadedAt);

const markImage = mark;

const completedView =
  useTemplateRef<InstanceType<typeof CompletedView>>("completedView");

function handleCommit() {
  if (!currentImage.value && completedView.value) {
    completedView.value.submit();
  } else {
    showCommitModal.value = true;
  }
}

const canUndo = computed(() => session.value?.canUndo && !undoing.value);
async function undo() {
  if (!canUndo.value) return;
  undoing.value = true;

  try {
    await mutate(UndoDocument, {
      variables: { input: { sessionId: sessionId.value } },
    });
  } finally {
    undoing.value = false;
  }
}

function insideSwipeArea(e: { clientX: number; clientY: number }) {
  const el = swipeEl.value;
  if (!el) {
    return false;
  }
  const rect = el.getBoundingClientRect();
  return (
    e.clientX >= rect.left &&
    e.clientX <= rect.right &&
    e.clientY >= rect.top &&
    e.clientY <= rect.bottom
  );
}

function handleAllowPan(e: PointerEvent) {
  if (e.pointerType === "touch" && insideSwipeArea(e)) {
    return false;
  }
  return true;
}

const didUseGesture = ref(false);
function handleGesture() {
  switch (swipeDirection.value) {
    case "UP":
      markImage(ImageAction.SHELVE);
      break;
    case "DOWN":
      markImage(ImageAction.REJECT);
      break;
    case "LEFT":
      markImage(ImageAction.KEEP);
      break;
    case "RIGHT":
      undo();
      break;
    default:
      return;
  }
  didUseGesture.value = true;
}
</script>
