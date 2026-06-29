<template>
  <div
    class="h-screen bg-primary-900 text-primary-100 flex flex-col overflow-hidden"
  >
    <SessionHeader
      :session
      :undoing="undoing"
      @show-update-session-modal="updateSessionDialog.open()"
      @show-commit-modal="handleCommit"
      @undo="undo"
    >
      <template #extra>
        <div class="mr-4 flex items-center gap-3">
          <!-- 自动排除控制面板 -->
          <div
            v-if="showAutoRejectControl"
            class="flex items-center gap-2 md:gap-4 select-none"
            data-filter-action="true"
          >
            <!-- 自动排除开关 -->
            <ToggleSwitch
              v-model="autoRejectEnabled"
              class="text-xs md:text-sm text-primary-200"
            >
              <span class="hidden sm:inline">自动排除</span>
            </ToggleSwitch>

            <!-- 时间输入框/调节器 -->
            <div class="flex items-center text-xs md:text-sm text-primary-300">
              <NumberInput
                v-model="autoRejectTimeoutSeconds"
                :min="0.1"
                :step="0.1"
                class="w-10 text-center bg-primary-800 border border-primary-700 rounded-lg py-1 text-white font-mono text-xs focus:outline-none focus:border-secondary-500"
                @focus="autoRejectRunning = false"
              />
              <span class="text-primary-400 select-none ml-1">秒</span>
            </div>
          </div>

          <!-- 全部保留按钮 -->
          <button
            v-else-if="showEarlyFinish"
            class="px-4 py-2 bg-green-600 hover:bg-green-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
            :disabled="earlyFinishing"
            @click="earlyFinish"
          >
            <svg
              v-if="earlyFinishing"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheckAll" fill="currentColor" />
            </svg>
            <span>{{ earlyFinishing ? "处理中..." : "全部保留" }}</span>
          </button>
        </div>
      </template>
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
          :session-id="sessionId"
          :preload-images="session?.nextImages ?? []"
          :allow-pan="handleAllowPan"
          @image-loaded="(e) => (lastImageLoadedEvent = e)"
        >
          <template #control-bg>
            <div
              v-if="isAutoRejectActive && currentImage && !swiping"
              :key="`${currentImageId}-${autoRejectTimeoutSeconds}`"
              class="absolute inset-y-0 left-0 bg-orange-600/15 pointer-events-none countdown-progress z-0"
              :style="{
                animationDuration: `${autoRejectTimeoutSeconds}s`,
              }"
            ></div>
          </template>
          <template #progress>
            <SessionProgressBar
              v-if="session"
              :session
              class="pointer-events-none"
            />
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
                保留: {{ session?.stats.totalKept || 0 }} /
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
          class="text-center text-xs md:text-sm text-primary-400 hidden md:block mb-2"
        >
          {{ currentImage?.filename || "" }}
        </div>

        <SessionActions
          v-if="!didUseGesture"
          class="hidden md:flex gap-4 w-full max-w-md mb-4"
          :marking="marking"
          @mark="markImage(currentImageId!, $event)"
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
            <RouterLink
              to="/"
              class="px-6 py-3 bg-secondary-600 hover:bg-secondary-700 rounded-lg font-medium flex items-center gap-2 whitespace-nowrap mx-auto text-white no-underline"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiHome" fill="currentColor" />
              </svg>
              返回主页
            </RouterLink>
          </div>
        </template>
        <template v-else>
          <CompletedView ref="completedView" :session @undo="undo" />
        </template>
      </div>
    </main>

    <footer
      v-if="currentImage"
      class="bg-primary-800 border-t border-primary-700 p-2 text-center text-xs text-primary-400 shrink-0 flex flex-col md:flex-row justify-center items-center gap-2 select-none"
      :class="didUseGesture ? 'hidden' : ''"
    >
      <div class="flex items-center gap-2 flex-wrap justify-center">
        <span class="flex items-center gap-1">
          <kbd
            class="px-2 py-1 bg-primary-950 text-primary-200 rounded border border-primary-800 font-mono text-xs shadow-sm"
            >↓</kbd
          >
          排除
        </span>
        <span class="text-primary-700">|</span>
        <span class="flex items-center gap-1">
          <kbd
            class="px-2 py-1 bg-primary-950 text-primary-200 rounded border border-primary-800 font-mono text-xs shadow-sm"
            >↑</kbd
          >
          搁置
        </span>
        <span class="text-primary-700">|</span>
        <span class="flex items-center gap-1">
          <kbd
            class="px-2 py-1 bg-primary-950 text-primary-200 rounded border border-primary-800 font-mono text-xs shadow-sm"
            >→</kbd
          >
          保留
        </span>
        <span class="text-primary-700">|</span>
        <span class="flex items-center gap-1">
          <kbd
            class="px-2 py-1 bg-primary-950 text-primary-200 rounded border border-primary-800 font-mono text-xs shadow-sm"
            >←</kbd
          >
          撤销
        </span>
      </div>
      <div class="hidden md:block text-primary-600/70">|</div>
      <div class="flex items-center gap-1">
        <span>按</span>
        <kbd
          class="px-2 py-1 bg-primary-950 text-primary-200 rounded border border-primary-800 font-mono text-xs shadow-sm"
          >?</kbd
        >
        <span>查看所有快捷键</span>
      </div>
    </footer>

    <commitDialog.component v-if="session" container-class="sm:max-w-md p-6">
      <CommitForm
        :session="session"
        title="提交更改"
        @committed="commitDialog.close"
      >
        <template #actions="{ committing, commitResult }">
          <button
            v-if="!commitResult"
            type="button"
            :disabled="committing"
            class="flex-1 px-4 py-2 bg-primary-700 hover:bg-primary-600 disabled:bg-primary-800 disabled:cursor-not-allowed rounded-lg transition-colors"
            @click="commitDialog.close"
          >
            取消
          </button>
          <button
            v-if="!commitResult"
            :disabled="committing"
            class="flex-2 px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg flex items-center justify-center gap-2 transition-colors font-bold"
            type="submit"
          >
            <svg
              v-if="committing"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <span>确认提交</span>
          </button>
          <button
            v-else
            class="flex-2 px-4 py-2 bg-green-600 hover:bg-green-700 rounded-lg transition-colors font-bold"
            type="button"
            @click="commitDialog.close"
          >
            完成
          </button>
        </template>
      </CommitForm>
    </commitDialog.component>

    <updateSessionDialog.component
      v-if="session"
      container-class="sm:max-w-md p-6"
    >
      <UpdateSessionForm
        :session="session"
        @close="updateSessionDialog.close"
        @updated="updateSessionDialog.close"
      />
    </updateSessionDialog.component>
  </div>
</template>

<script lang="ts">
import useStorage from "../composables/useStorage";

const { model: autoRejectTimeoutSeconds } = useStorage<number>(
  localStorage,
  "auto_exclude_timeout_d2e1a3",
  () => 1,
);
const { model: autoRejectEnabled } = useStorage(
  localStorage,
  "auto_reject_enabled_29c75583fa91",
  false,
);
</script>

<script setup lang="ts">
import "core-js/actual/disposable-stack";
import { ref, shallowRef, computed, useTemplateRef, watch } from "vue";

import mutate from "../graphql/utils/mutate";
import { UndoDocument, ImageAction } from "../graphql/generated";
import ImageViewer from "../components/ImageViewer.vue";
import SessionHeader from "../components/SessionHeader.vue";
import SessionActions from "../components/SessionActions.vue";
import NumberInput from "../components/NumberInput.vue";
import ToggleSwitch from "../components/ToggleSwitch.vue";

import SwipeDirectionIndicator from "../components/SwipeDirectionIndicator.vue";
import CompletedView from "../components/CompletedView.vue";
import CommitForm from "../components/CommitForm.vue";
import UpdateSessionForm from "../components/UpdateSessionForm.vue";
import SessionProgressBar from "../components/SessionProgressBar.vue";
import useModalDialog from "@/composables/useModalDialog";
import useEventListeners from "../composables/useEventListeners";
import { useHotkeys } from "@/composables/useHotkeys";
import { formatDate } from "../utils/date";
import { mdiCheckAll, mdiHome, mdiLoading } from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import useSession from "../composables/useSession";
import useMarkImage from "@/composables/useMarkImage";
import Time from "@/utils/Time";
import useNotification from "@/composables/useNotification";

const rendererEl = useFullscreenRendererElement();

const autoRejectRunningBuffer = ref(false);
const autoRejectRunning = computed({
  get: () => autoRejectEnabled.value && autoRejectRunningBuffer.value,
  set: (val) => {
    autoRejectRunningBuffer.value = val;
  },
});

const props = defineProps<{
  id: string;
}>();

const sessionId = computed(() => props.id);

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);

const commitDialog = useModalDialog();
const updateSessionDialog = useModalDialog();
const undoing = ref(false);

const touchStartX = ref(0);
const touchStartY = ref(0);
const touchEndX = ref(0);
const touchEndY = ref(0);
const swiping = ref(false);

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

const currentImage = computed(() => session.value?.currentImage ?? undefined);

// 优先使用已加载完成的图片 id，避免在图片切换瞬间使用错误的 id
const currentImageId = computed(
  () => lastImageLoadedEvent.value?.id ?? currentImage.value?.id,
);

const swipeEl = useTemplateRef("swipeEl");

// #region 快捷键注册
const isSessionImageActive = computed(() => !!currentImage.value);

useHotkeys(
  {
    ArrowDown: () => {
      const imageId = currentImageId.value;
      if (imageId) {
        markImage(imageId, ImageAction.REJECT);
      }
    },
    ArrowUp: () => {
      const imageId = currentImageId.value;
      if (imageId) {
        markImage(imageId, ImageAction.SHELVE);
      }
    },
    ArrowRight: () => {
      const imageId = currentImageId.value;
      if (imageId) {
        markImage(imageId, ImageAction.KEEP);
      }
    },
    ArrowLeft: () => {
      const imageId = currentImageId.value;
      if (imageId) {
        undo();
      }
    },
  },
  {
    enabled: isSessionImageActive,
    category: "筛选会话",
  },
);
// #endregion

useEventListeners(window, ({ on }) => {
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
              el.role == "dialog" ||
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

  on("keydown", (e) => {
    if (!autoRejectRunning.value) return;
    if (["ArrowUp", "ArrowDown", "ArrowRight"].includes(e.key)) {
      return;
    }
    autoRejectRunning.value = false;
  });

  on("pointerdown", (e) => {
    if (!autoRejectRunning.value) return;
    const target = e.target as HTMLElement;
    if (target.closest("[data-filter-action]")) {
      return;
    }
    if (insideSwipeArea(e)) {
      return;
    }
    autoRejectRunning.value = false;
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
const { marking, mark: originalMarkImage } = useMarkImage(
  sessionId,
  imageLoadedAt,
);
async function markImage(id: string, action: ImageAction) {
  if (autoRejectEnabled.value) {
    autoRejectRunning.value = true;
  }
  await originalMarkImage(id, action);
}

const completedView =
  useTemplateRef<InstanceType<typeof CompletedView>>("completedView");

const showEarlyFinish = computed(() => {
  const s = session.value;
  if (!s || !currentImage.value) return false;
  const currentRoundKept = s.currentRoundActions.filter(
    (a) => a === ImageAction.KEEP,
  ).length;
  return currentRoundKept + s.stats.currentRoundRemaining <= s.targetKeep;
});

const earlyFinishing = ref(false);
async function earlyFinish() {
  const image = currentImage.value;
  const s = session.value;
  if (!image || !s) return;
  const ids = [image.id, ...(s.nextImages ?? []).map((i) => i.id)];
  earlyFinishing.value = true;
  try {
    for (const id of ids) {
      await markImage(id, ImageAction.KEEP);
    }
  } finally {
    earlyFinishing.value = false;
  }
}

function handleCommit() {
  if (!currentImage.value && completedView.value) {
    completedView.value.submit();
  } else {
    commitDialog.open();
  }
}

const { show: showNotification, remove: removeNotification } =
  useNotification();

const canUndo = computed(() => session.value?.canUndo && !undoing.value);
async function undo() {
  if (!canUndo.value) return;
  autoRejectRunning.value = false;
  undoing.value = true;

  using stack = new DisposableStack();
  stack.defer(() => {
    undoing.value = false;
  });

  stack.adopt(
    setTimeout(() => {
      const id = showNotification("正在撤销，请稍候...", "info", 0);
      stack.adopt(id, removeNotification);
    }, 800),
    clearTimeout,
  );

  await mutate(UndoDocument, {
    variables: { input: { sessionId: sessionId.value } },
  });
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
  const imageId = currentImageId.value;
  if (!imageId) {
    return;
  }
  switch (swipeDirection.value) {
    case "UP":
      markImage(imageId, ImageAction.SHELVE);
      break;
    case "DOWN":
      markImage(imageId, ImageAction.REJECT);
      break;
    case "LEFT":
      markImage(imageId, ImageAction.KEEP);
      break;
    case "RIGHT":
      undo();
      break;
    default:
      return;
  }
  didUseGesture.value = true;
}

// #region 自动排除相关逻辑
const showAutoRejectControl = computed(() => {
  const s = session.value;
  if (!s || !currentImage.value) return false;
  return s.stats.totalKept + s.stats.currentRoundRemaining > s.targetKeep * 2;
});

const isAutoRejectActive = computed(() => {
  return autoRejectRunning.value && showAutoRejectControl.value;
});

// 监听图片加载、超时时长、实际是否生效，内联控制定时器启动与清理
watch(
  [isAutoRejectActive, autoRejectTimeoutSeconds, lastImageLoadedEvent, swiping],
  ([active, timeout, loadedEvent, isSwiping], _, onCleanup) => {
    if (!active || !loadedEvent || !currentImage.value || isSwiping) {
      return;
    }

    const imageId = currentImageId.value;
    const timeoutMs = (timeout ?? 1) * 1000;

    // 使用词法作用域局部常量存储定时器 ID，由每个 watch 实例自己负责清理自己
    const timer = window.setTimeout(async () => {
      const currentId = currentImageId.value;
      // 触发时检验：确认图片未被手动切换，未处于标记中，且自动排除依旧生效
      if (
        currentId &&
        currentId === imageId &&
        !marking.value &&
        isAutoRejectActive.value
      ) {
        await markImage(currentId, ImageAction.REJECT);
      }
    }, timeoutMs);

    // 3. 注册清理函数：当下一次 watch 被触发（状态改变）或组件销毁时，自动清理
    onCleanup(() => {
      clearTimeout(timer);
    });
  },
  { immediate: true },
);
// #endregion
</script>

<style scoped>
.countdown-progress {
  animation: shrinkWidth linear forwards;
}

@keyframes shrinkWidth {
  from {
    width: 100%;
  }
  to {
    width: 0;
  }
}
</style>
