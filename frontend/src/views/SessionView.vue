<template>
  <div
    class="h-screen bg-primary-900 text-primary-100 flex flex-col overflow-hidden"
  >
    <SessionHeader
      :session
      :undoing="undoing"
      @show-menu="showMenu = true"
      @show-update-session-modal="showUpdateSessionModal = true"
      @show-commit-modal="showCommitModal = true"
    >
      <template #extra>
        <button
          class="p-2 mr-2 rounded-lg hover:bg-primary-700 transition-colors flex items-center"
          :class="isImageLocked ? 'text-secondary-400' : 'text-primary-400'"
          @click="isImageLocked = !isImageLocked"
        >
          <svg class="w-6 h-6" viewBox="0 0 24 24">
            <path
              :d="isImageLocked ? mdiLock : mdiLockOpenVariant"
              fill="currentColor"
            />
          </svg>
          <span class="hidden md:inline">
            {{ isImageLocked ? "解锁图片位置" : "锁定图片位置" }}
          </span>
        </button>
      </template>
    </SessionHeader>

    <main
      class="flex-1 flex flex-col items-center justify-center p-2 md:p-4 overflow-hidden"
    >
      <template v-if="currentImage">
        <ImageViewer
          class="relative w-full flex-1 bg-primary-800 rounded-lg overflow-hidden"
          :image="currentImage"
          :next-images="session?.nextImages ?? []"
          :locked="isImageLocked"
          :allow-pan="handleAllowPan"
        >
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
        <Teleport :to="rendererEl">
          <div
            ref="swipeEl"
            class="fixed bottom-0 left-0 right-0 top-1/2 overflow-hidden pointer-events-none"
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

      <template v-else-if="loading">
        <div v-if="loading" class="text-center text-primary-400">加载中...</div>
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
        <CompletedView :session />
      </template>
    </main>

    <footer
      class="bg-primary-800 border-t border-primary-700 p-2 text-center text-xs text-primary-400 shrink-0"
      :class="didUseGesture ? 'hidden' : ''"
    >
      ↓ 排除 | ↑ 稍后再看 | → 保留 | ← 撤销
    </footer>

    <SessionMenu
      v-model:show="showMenu"
      :session
      :can-undo="canUndo"
      :undoing="undoing"
      @show-commit-modal="showCommitModal = true"
      @show-update-session-modal="showUpdateSessionModal = true"
    />

    <CommitModal
      v-if="showCommitModal"
      :session
      @close="showCommitModal = false"
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
import { ref, computed, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import mutate from "../graphql/utils/mutate";
import {
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
import { mdiHome, mdiLock, mdiLockOpenVariant } from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import useSession from "../composables/useSession";

const rendererEl = useFullscreenRendererElement();
const router = useRouter();

const { id: sessionId } = defineProps<{
  id: string;
}>();

const isImageLocked = ref(false);

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);
const showMenu = ref<boolean>(false);
const showUpdateSessionModal = ref<boolean>(false);
const showCommitModal = ref<boolean>(false);
const undoing = ref(false);
const marking = ref(false);

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
  } else {
    if (Math.abs(deltaY) > SWIPE_THRESHOLD) {
      return deltaY > 0 ? "DOWN" : "UP";
    }
  }
  return null;
});

const { session } = useSession(() => sessionId, { loadingCount });

const currentImage = computed(() => session.value?.currentImage);

const swipeEl = useTemplateRef("swipeEl");
useEventListeners(window, ({ on }) => {
  on("keydown", (e) => {
    if (showMenu.value) return;

    switch (e.key) {
      case "ArrowDown":
        markImage(ImageAction.REJECT);
        break;
      case "ArrowUp":
        markImage(ImageAction.PENDING);
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

      e.preventDefault();
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
      e.preventDefault();
      touchEndX.value = e.changedTouches[0].clientX;
      touchEndY.value = e.changedTouches[0].clientY;
    },
    { passive: false },
  );
  on(
    "touchend",
    (e) => {
      if (!swiping.value) {
        return;
      }
      e.preventDefault();
      touchEndX.value = e.changedTouches[0].clientX;
      touchEndY.value = e.changedTouches[0].clientY;
      handleGesture();
      swiping.value = false;
    },
    { passive: false },
  );
});

async function markImage(action: ImageAction) {
  if (!currentImage.value) return;

  marking.value = true;

  try {
    await mutate(MarkImageDocument, {
      variables: {
        input: {
          sessionId,
          imageId: currentImage.value.id,
          action,
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
  console.log(swipeDirection.value);
  switch (swipeDirection.value) {
    case "UP":
      markImage(ImageAction.PENDING);
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
