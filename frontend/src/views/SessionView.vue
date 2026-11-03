<template>
  <div
    class="session-root round-flash-surface h-screen bg-primary-900 text-primary-100 flex flex-col overflow-hidden"
    :data-round-flash="roundFlashPhase"
  >
    <SessionHeader
      class="round-flash-surface"
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
            <ToggleSwitch v-model="autoRejectEnabled" class="text-xs md:text-sm text-primary-200">
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
            <svg v-if="earlyFinishing" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheckAll" fill="currentColor" />
            </svg>
            <span>{{ earlyFinishing ? "处理中…" : "全部保留" }}</span>
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
          class="round-flash-surface relative w-full flex-1 bg-primary-800 rounded-lg overflow-hidden"
          :image="currentImage"
          :session-id="sessionId"
          :preload-images="session?.nextImages ?? []"
          :allow-pan="handleAllowPan"
          :hide-image-button="true"
          @image-loaded="handleImageLoaded"
        >
          <template #viewport>
            <div
              v-if="ratedRejectSignal"
              :key="`rated-reject-${ratedRejectSignal.seq}`"
              class="absolute inset-0 pointer-events-none rated-reject-flash"
              :style="{ '--flash-color': getStarColorVar(ratedRejectSignal.rating) }"
              @animationend="clearRatedRejectFlash()"
            ></div>
          </template>
          <template #control-bg>
            <div
              v-if="isAutoRejectActive && currentImage && !swiping"
              :key="`${displayedId}-${autoRejectTimeoutSeconds}`"
              class="absolute inset-y-0 left-0 bg-secondary-500/50 pointer-events-none countdown-progress"
              :style="{
                animationDuration: `${autoRejectTimeoutSeconds}s`,
              }"
            ></div>
          </template>
          <template #progress>
            <SessionProgressBar v-if="session" :session class="pointer-events-none" />
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
        <div class="text-center text-xs md:text-sm text-primary-400 hidden md:block mb-2">
          {{ currentImage?.filename || "" }}
        </div>

        <SessionActions
          v-if="!didUseGesture"
          class="hidden md:flex gap-4 w-full max-w-md mb-4"
          :marking="marking"
          @mark="markImage(displayedId, $event)"
        />
      </template>

      <div v-else class="min-h-full flex flex-col items-center justify-center w-full">
        <template v-if="loading">
          <div class="text-center text-primary-400">加载中…</div>
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
          <CompletedView
            ref="completedView"
            :session="session"
            :next-directory-id="nextDirectoryId"
            :resolve-next-directory="resolveNextDirectory"
            @undo="undo"
          />
        </template>
      </div>
    </main>

    <footer
      v-if="currentImage"
      class="round-flash-surface bg-primary-800 border-t border-primary-700 p-2 text-center text-xs text-primary-400 shrink-0 flex flex-col md:flex-row justify-center items-center gap-2 select-none"
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
      <CommitForm :session="session" title="提交更改" @committed="handleCommitCommitted">
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
            <svg v-if="committing" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
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

    <updateSessionDialog.component v-if="session" container-class="sm:max-w-md p-6">
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

const { model: autoRejectEnabled } = useStorage(
  localStorage,
  "auto_reject_enabled_29c75583fa91",
  false,
);
const { model: autoRejectTimeoutSeconds } = useStorage<number>(
  localStorage,
  "auto_reject_timeout_29c75583fa91",
  () => 1,
);
</script>

<script setup lang="ts">
import "core-js/actual/disposable-stack";
import { ref, computed, useTemplateRef, watch } from "vue";

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
import useSession from "../composables/domain/useSession";
import useSessionNextDirectory from "../composables/useSessionNextDirectory";
import useMarkImage from "@/composables/useMarkImage";
import useNotification from "@/composables/useNotification";
import useDisplayedImage, { type ImageLoadedEvent } from "@/composables/useDisplayedImage";
import useRatedRejectFlash from "@/composables/useRatedRejectFlash";
import useRoundChangeFlash from "@/composables/useRoundChangeFlash";
import { getStarColorVar } from "@/utils/starConfig";

const props = defineProps<{
  id: string;
}>();

const sessionId = computed(() => props.id);

const rendererEl = useFullscreenRendererElement();

const autoRejectRunningBuffer = ref({ id: props.id, running: false });
const autoRejectRunning = computed({
  get: () =>
    autoRejectEnabled.value &&
    autoRejectRunningBuffer.value.id === props.id &&
    autoRejectRunningBuffer.value.running,
  set: (val) => {
    autoRejectRunningBuffer.value = { id: props.id, running: val };
  },
});

const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);

const commitDialog = useModalDialog();
const updateSessionDialog = useModalDialog();
const undoing = ref(false);

// 提交成功结果需在弹窗中短暂展示后关闭，关闭时机由本视图掌控
function handleCommitCommitted() {
  setTimeout(() => commitDialog.close(), 500);
}

// #region 手势处理与滑动控制
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

// 会话入口即生成导航游标：完成视图挂载时当前目录往往已被筛成完成态，
// 不能再依赖挂载时的列表重新生成。
const { nextDirectoryId, resolveNextDirectory } = useSessionNextDirectory(
  () => sessionId.value,
  () => session.value?.directory,
);

const currentImage = computed(() => session.value?.currentImage ?? undefined);

// 显示中的图片（标记与手势的目标）按会话作用域隔离：
// SessionView 在路由参数变化时复用，上一会话残留的加载状态不得被新会话继承
const {
  displayedId,
  loadedAt,
  gestureTargetId,
  handleImageLoaded: handleDisplayedImageLoaded,
  recordGestureEnd,
} = useDisplayedImage(sessionId, () => currentImage.value?.id);

// #region 换轮提示
// 同一会话内轮次增大后，等下一轮首图真正加载完成再触发背景变色。
// 会话 ID 与轮次都取自同一份会话数据，切换会话时二者原子更新，不会误判为换轮。
const { phase: roundFlashPhase, handleImageLoaded: handleRoundChangeLoaded } = useRoundChangeFlash(
  () => session.value?.id,
  () => session.value?.currentRound,
);

// image-loaded 是"图片真正显示出来"的唯一信号：先交给 useDisplayedImage 记录加载状态，
// 再由 loadedAt 确认它是会话队列的当前图，避免旧的加载事件提前播放提示
function handleImageLoaded(event: ImageLoadedEvent | null | undefined) {
  handleDisplayedImageLoaded(event);
  if (loadedAt.value) {
    handleRoundChangeLoaded();
  }
}
// #endregion

const swipeEl = useTemplateRef("swipeEl");

// #region 快捷键注册
const isSessionImageActive = computed(() => !!currentImage.value);

useHotkeys(
  {
    ArrowDown: () => {
      markImage(displayedId.value, ImageAction.REJECT);
    },
    ArrowUp: () => {
      markImage(displayedId.value, ImageAction.SHELVE);
    },
    ArrowRight: () => {
      markImage(displayedId.value, ImageAction.KEEP);
    },
    ArrowLeft: () => {
      undo();
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
      recordGestureEnd();
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
    autoRejectRunning.value = false;
  });
});

const { marking, mark: originalMarkImage } = useMarkImage(sessionId, loadedAt);
// flash 信号按会话作用域隔离，防止提交后切换到下一目录会话时重放残留动画
const {
  signal: ratedRejectSignal,
  flash: ratedRejectFlash,
  clear: clearRatedRejectFlash,
} = useRatedRejectFlash(sessionId);
async function markImage(id: string | undefined, action: ImageAction) {
  // 视图侧副作用（自动排除运行态、星级扫过动画）需要有显示中的图片，
  // 无目标时由 mark 静默丢弃请求
  if (!id) {
    return;
  }
  if (autoRejectEnabled.value) {
    autoRejectRunning.value = true;
  }
  // 在标记调用前从当前图片同步读取评分，触发已评分图片的排除星级扫过动画
  ratedRejectFlash(action, currentImage.value?.currentRating ?? 0);
  await originalMarkImage(id, action);
}

const completedView = useTemplateRef<InstanceType<typeof CompletedView>>("completedView");

const currentRoundKept = computed(() => {
  const s = session.value;
  if (!s) return 0;
  return s.currentRoundActions.filter((a) => a === ImageAction.KEEP).length;
});

const showEarlyFinish = computed(() => {
  const s = session.value;
  if (!s || !currentImage.value) return false;
  return currentRoundKept.value + s.stats.currentRoundRemaining <= s.targetKeep;
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

const { show: showNotification, remove: removeNotification } = useNotification();

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
      const id = showNotification("正在撤销，请稍候…", "info", 0);
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
  // 没有显示中的图片就没有手势目标：整段手势（含撤销）都不生效
  const imageId = gestureTargetId.value;
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
// #endregion

// #region 自动排除相关逻辑
const showAutoRejectControl = computed(() => {
  const s = session.value;
  if (!s || !currentImage.value) return false;
  // 已评级（评级 > 0）的图片不自动排除，且不显示自动排除开关
  if (currentImage.value.currentRating > 0) return false;
  return currentRoundKept.value + s.stats.currentRoundRemaining > s.targetKeep * 2;
});

const isAutoRejectActive = computed(() => {
  return autoRejectRunning.value && showAutoRejectControl.value;
});

// 监听图片加载、超时时长、实际是否生效，内联控制定时器启动与清理
watch(
  [isAutoRejectActive, autoRejectTimeoutSeconds, loadedAt, swiping, marking],
  ([active, timeout, imageLoadedAt, isSwiping], _, onCleanup) => {
    // loadedAt 仅在会话队列的当前图片加载完成时给出，视图不再自行比对图片 id
    if (!active || !imageLoadedAt || isSwiping) {
      return;
    }

    const imageId = displayedId.value;
    const timeoutMs = (timeout ?? 1) * 1000;

    // 使用词法作用域局部常量存储定时器 ID，由每个 watch 实例自己负责清理自己
    const timer = window.setTimeout(async () => {
      // 触发时检验：确认显示中的图片未被手动切换，未处于标记中，且自动排除依旧生效
      if (imageId && displayedId.value === imageId && !marking.value && isAutoRejectActive.value) {
        await markImage(imageId, ImageAction.REJECT);
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
/* #region 换轮背景提示 */
/* 让自定义属性可被过渡，从而只需在会话根元素上维护一份透明度状态，
   所有深色背景面通过继承与 background-image 统一响应 */
@property --round-flash-opacity {
  syntax: "<number>";
  inherits: true;
  initial-value: 0;
}

.session-root {
  --round-flash-opacity: 0;
  transition: --round-flash-opacity 200ms ease-out;
}

/* 淡入与保持阶段处于变色状态；淡出阶段移除变量，由过渡完成 200ms 淡回 */
.session-root[data-round-flash="fade-in"],
.session-root[data-round-flash="hold"] {
  --round-flash-opacity: 0.18;
}

/* 减少动效：全局样式已把过渡压成瞬时，这里让变色状态延续到完整时长结束，
   使恢复时刻仍为 1.0 秒末尾，而不是提前 200ms 消失 */
@media (prefers-reduced-motion: reduce) {
  .session-root[data-round-flash="fade-out"] {
    --round-flash-opacity: 0.18;
  }
}

/* 只给深色背景面叠加蓝色背景层：图片本身与图片上的叠加反馈不是变色面 */
.round-flash-surface {
  background-image: linear-gradient(
    rgb(59 130 246 / var(--round-flash-opacity)),
    rgb(59 130 246 / var(--round-flash-opacity))
  );
}

/* 图片容器内的控制栏位于 ImageViewer 内部，需经 :deep 纳入同一份共享状态 */
:deep(.round-flash-surface) {
  background-image: linear-gradient(
    rgb(59 130 246 / var(--round-flash-opacity)),
    rgb(59 130 246 / var(--round-flash-opacity))
  );
}
/* #endregion */

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

.rated-reject-flash {
  background: linear-gradient(to right, transparent, var(--flash-color) 30%, transparent);
  animation: sweepRightToLeft 300ms linear forwards;
}

@keyframes sweepRightToLeft {
  from {
    transform: translateX(100%);
    opacity: 0.6;
  }
  to {
    transform: translateX(-100%);
    opacity: 0;
  }
}
</style>
