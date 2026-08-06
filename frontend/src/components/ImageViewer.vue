<template>
  <div
    ref="rootEl"
    class="flex flex-col bg-primary-800 rounded-lg overflow-hidden isolate contain-layout h-14"
  >
    <slot name="progress"></slot>
    <div
      class="relative flex-1 w-full min-h-0"
      :class="[
        // 修复火狐全屏时竖屏高度计算错误
        isFullscreen ? 'portrait:order-1 portrait:max-h-[calc(100dvh-var(--spacing)*14)]' : '',
      ]"
    >
      <div
        ref="containerRef"
        class="absolute inset-0 w-full flex items-center [scrollbar-gutter:stable] overflow-auto"
        :class="locked ? 'pointer-events-none' : ''"
        v-bind="!locked ? containerAttrs : {}"
      >
        <!-- zoom -->
        <div v-bind="zoomAttrs" class="contain-layout m-auto flex-none">
          <img
            ref="imgEl"
            :src="src"
            :width="image.width"
            :height="image.height"
            :alt="image.filename"
            :data-image-id="image.id"
            class="object-contain w-full h-full"
            @loadstart="onLoadStart"
            @load="updateLoaded"
            @error="updateLoaded"
          />
        </div>
      </div>
      <!-- 加载提示 -->
      <Transition
        enter-active-class="transition duration-100 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <template v-if="isSlowLoading">
          <div
            class="absolute inset-0 flex items-center justify-center bg-primary-900/25 backdrop-blur-sm"
          >
            <svg class="w-12 h-12 animate-spin text-secondary-400" viewBox="0 0 24 24" fill="none">
              <path
                :d="mdiLoading"
                fill="none"
                stroke="currentColor"
                stroke-width="3"
                stroke-linecap="round"
              />
            </svg>
          </div>
        </template>
      </Transition>
    </div>
    <!-- 图片尺寸和缩放操作 -->
    <div
      data-no-gesture
      class="relative flex-none flex items-center justify-center flex-wrap gap-2 bg-black/70 text-white text-xs px-2 py-1"
    >
      <div class="absolute inset-0 overflow-hidden pointer-events-none">
        <slot name="control-bg"></slot>
      </div>
      <button
        class="hover:bg-white/20 w-6 h-6 items-center justify-center rounded transition-colors hidden md:flex"
        title="缩小"
        @click="zoomOut"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
        </svg>
      </button>
      <span
        class="min-w-12 text-center cursor-pointer"
        :class="isFullscreen ? 'hidden md:block' : ''"
        @click="zoomAsPercent = 100"
        >{{ zoomAsPercent }}%</span
      >
      <button
        class="hover:bg-white/20 w-6 h-6 items-center justify-center rounded transition-colors hidden md:flex"
        title="放大"
        @click="zoomIn"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M12 4v16m8-8H4"
          />
        </svg>
      </button>
      <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>

      <!-- 复制按钮 -->
      <button
        class="hidden md:flex items-center gap-2 select-none transition-colors"
        :class="[
          isCopying
            ? 'text-white/50 opacity-40 cursor-not-allowed'
            : isCopied
              ? 'text-secondary-400 hover:text-secondary-300 cursor-pointer'
              : 'text-white/50 hover:text-white cursor-pointer',
        ]"
        :disabled="isCopying"
        title="复制"
        @click="handleCopy"
      >
        <svg v-if="isCopying" class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="currentColor">
          <path :d="mdiLoading" />
        </svg>
        <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="mdiContentCopy" />
        </svg>
        <span class="text-xs">{{ isCopying ? "正在复制…" : isCopied ? "已复制" : "复制" }}</span>
      </button>
      <div class="hidden md:block w-px h-4 bg-white/30 mx-1"></div>

      <!-- 打开文件按钮 (在资源管理器中定位并聚焦) -->
      <button
        class="hidden md:flex items-center gap-2 cursor-pointer select-none text-white/50 hover:text-white transition-colors"
        title="在资源管理器中定位此文件"
        @click="revealInExplorer(fullFilePath)"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="mdiOpenInNew" />
        </svg>
        <span class="text-xs">打开</span>
      </button>
      <div class="hidden md:block w-px h-4 bg-white/30 mx-1"></div>

      <!-- 评星操作区 (仅在无会话模式下展示和操作) -->
      <template v-if="!sessionId">
        <RatingSelector v-model="ratingModel" />
        <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
      </template>
      <template v-else-if="image.currentRating">
        <RatingIcon :rating="image.currentRating" filled />
        <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
      </template>

      <!-- 颜色标签 -->
      <div ref="popoverContainerRef" class="relative flex items-center select-none" data-no-gesture>
        <!-- 触发按钮 -->
        <button
          class="hover:bg-white/10 px-2 py-1 rounded flex items-center gap-2 transition-all active:scale-95 z-40"
          title="设置标签"
          @click="showPopover = !showPopover"
        >
          <template v-if="currentLabel">
            <!-- 标准预设色：渲染为发光彩色圆点 -->
            <span
              v-if="isPresetColor"
              class="w-4 h-4 rounded-full inline-block shadow-[0_0_8px_rgba(255,255,255,0.4)] transition-transform duration-300"
              :style="{ backgroundColor: currentLabelColor }"
            ></span>
            <!-- 自定义文本标签：渲染为气泡 -->
            <span
              v-else
              class="px-2 py-1 rounded-full text-xs font-bold bg-white/20 text-white max-w-24 truncate transition-all duration-300 shadow-[0_2px_4px_rgba(0,0,0,0.2)]"
            >
              {{ currentLabel }}
            </span>
          </template>
          <template v-else>
            <!-- 空白标签态 -->
            <svg
              class="w-4 h-4 text-white/50 hover:text-white transition-colors"
              viewBox="0 0 24 24"
              fill="currentColor"
            >
              <path :d="mdiTagOutline" />
            </svg>
            <span class="text-white/50 text-xs hidden md:inline">标签</span>
          </template>
        </button>

        <!-- 玻璃磨砂下拉气泡菜单 -->
        <Transition
          enter-active-class="transition duration-150 ease-out"
          enter-from-class="opacity-0 translate-y-2 scale-95"
          enter-to-class="opacity-100 translate-y-0 scale-100"
          leave-active-class="transition duration-100 ease-in"
          leave-from-class="opacity-100 translate-y-0 scale-100"
          leave-to-class="opacity-0 translate-y-2 scale-95"
        >
          <div
            v-if="showPopover"
            class="fixed inset-x-2 bottom-16 z-50 min-w-0 bg-primary-950/90 border border-white/10 backdrop-blur-md rounded-xl p-3 shadow-[0_10px_25px_-5px_rgba(0,0,0,0.5)] flex flex-col gap-2 pointer-events-auto md:absolute md:inset-x-auto md:bottom-full md:mb-2 md:left-1/2 md:-translate-x-1/2 md:w-52"
          >
            <div
              class="text-xs font-bold text-white/40 tracking-wider uppercase select-none text-left"
            >
              选择颜色标签
            </div>

            <!-- 颜色网格 -->
            <div class="grid grid-cols-5 gap-2 justify-items-center">
              <button
                v-for="(color, name) in PRESET_COLORS"
                :key="name"
                class="w-6 h-6 rounded-full transition-all duration-200 relative flex items-center justify-center hover:scale-110 active:scale-95 border"
                :class="[
                  currentLabel === name
                    ? 'border-white scale-105 shadow-[0_0_8px_rgba(255,255,255,0.5)]'
                    : 'border-white/20 hover:border-white/50',
                ]"
                :style="{ backgroundColor: color }"
                :title="name"
                @click="setLabel(name)"
              >
                <!-- 选中勾选标记 -->
                <svg
                  v-if="currentLabel === name"
                  class="w-4 h-4"
                  :class="name === 'White' ? 'text-black' : 'text-white'"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <path :d="mdiCheck" />
                </svg>
              </button>
            </div>

            <!-- 输入自定义文本 -->
            <div class="flex flex-col gap-1 border-t border-white/10 pt-2 text-left">
              <div class="text-xs font-bold text-white/40 tracking-wider uppercase select-none">
                自定义标签
              </div>
              <div class="flex gap-1">
                <input
                  v-model="customLabelInput"
                  type="text"
                  placeholder="文字…"
                  class="flex-1 bg-white/5 border border-white/10 rounded px-2 py-1 text-xs md:text-sm text-white placeholder-white/30 focus:outline-none focus:border-secondary-500 transition-colors w-0"
                  @keydown.enter="saveCustomLabel"
                />
                <button
                  class="bg-secondary-600 hover:bg-secondary-500 transition-colors text-white rounded px-2 py-0.5 text-xs md:text-sm font-bold shrink-0"
                  @click="saveCustomLabel"
                >
                  保存
                </button>
              </div>
            </div>

            <!-- 清除标签按钮 -->
            <button
              v-if="currentLabel"
              class="border-t border-white/10 pt-2 text-center text-xs md:text-sm text-red-400 hover:text-red-300 transition-colors flex items-center justify-center gap-1 w-full"
              @click="setLabel('')"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                <path :d="mdiDeleteOutline" />
              </svg>
              清除标签
            </button>
          </div>
        </Transition>
      </div>
      <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>

      <!-- 笔记按钮 -->
      <button
        class="hover:bg-white/10 px-2 py-1 rounded flex items-center gap-2 transition-all active:scale-95 text-white/50 hover:text-white shrink-0 cursor-pointer"
        :class="image.note.content ? 'text-secondary-400 hover:text-secondary-300' : ''"
        :title="image.note.content ? '编辑笔记' : '添加笔记'"
        @click="noteDialog.open()"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="mdiNoteTextOutline" />
        </svg>
        <span class="truncate max-w-24 md:max-w-36 text-xs">{{
          image.note.content || "添加笔记"
        }}</span>
      </button>
      <div class="w-px h-4 bg-white/30 mx-1"></div>

      <!-- 动作按钮与下拉气泡 -->
      <div
        v-if="dispatchableHooks.length > 0"
        ref="actionPopoverContainerRef"
        class="hidden md:flex relative items-center select-none"
        data-no-gesture
      >
        <!-- 触发按钮 -->
        <button
          class="hover:bg-white/10 px-2 py-1 rounded flex items-center gap-2 transition-all active:scale-95 z-40 text-white/50 hover:text-white cursor-pointer"
          title="执行动作"
          @click="showActionPopover = !showActionPopover"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
            <path :d="mdiPlayOutline" />
          </svg>
          <span class="text-xs">动作</span>
        </button>

        <!-- 玻璃磨砂下拉气泡菜单 -->
        <Transition
          enter-active-class="transition duration-150 ease-out"
          enter-from-class="opacity-0 translate-y-2 scale-95"
          enter-to-class="opacity-100 translate-y-0 scale-100"
          leave-active-class="transition duration-100 ease-in"
          leave-from-class="opacity-100 translate-y-0 scale-100"
          leave-to-class="opacity-0 translate-y-2 scale-95"
        >
          <div
            v-if="showActionPopover"
            class="absolute bottom-full mb-2 left-1/2 -translate-x-1/2 z-50 w-52 bg-primary-950/90 border border-white/10 backdrop-blur-md rounded-xl p-3 shadow-[0_10px_25px_-5px_rgba(0,0,0,0.5)] flex flex-col gap-2 pointer-events-auto"
          >
            <div
              class="text-xs font-bold text-white/40 tracking-wider uppercase select-none text-left"
            >
              执行动作
            </div>

            <!-- 动作列表 -->
            <div class="flex flex-col gap-1 max-h-48 overflow-y-auto pr-1">
              <button
                v-for="hook in dispatchableHooks"
                :key="hook.id"
                :disabled="isDispatching"
                class="px-2 py-1.5 text-xs text-left text-white/80 hover:text-white hover:bg-white/10 rounded-lg transition-colors flex items-center justify-between disabled:opacity-40 disabled:cursor-not-allowed select-none cursor-pointer"
                :title="hook.description || hook.name"
                @click="handleDispatch(hook.id, hook.name)"
              >
                <span class="truncate pr-2">{{ hook.name }}</span>
                <svg
                  v-if="isDispatching && currentDispatchingHookId === hook.id"
                  class="w-3.5 h-3.5 animate-spin shrink-0 text-secondary-400"
                  viewBox="0 0 24 24"
                  fill="none"
                >
                  <path
                    :d="mdiLoading"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="3"
                    stroke-linecap="round"
                  />
                </svg>
                <svg
                  v-else
                  class="w-3.5 h-3.5 shrink-0 text-white/40"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <path :d="mdiPlayOutline" />
                </svg>
              </button>
            </div>
          </div>
        </Transition>
      </div>
      <div
        v-if="dispatchableHooks.length > 0"
        class="hidden md:block w-px h-4 bg-white/30 mx-1"
      ></div>

      <span class="min-w-16">{{ image.width }} × {{ image.height }}</span>
      <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
      <slot name="info" :is-fullscreen />
      <div class="w-px h-4 bg-white/30 mx-1" :class="isFullscreen ? 'hidden md:block' : ''"></div>
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        :title="locked ? '解锁位置' : '锁定位置'"
        :class="locked ? 'text-secondary-500' : ''"
        @click="locked = !locked"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="locked ? mdiLock : mdiLockOpenVariant" />
        </svg>
      </button>
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        :title="isFullscreen ? '退出全屏' : '全屏'"
        @click="handleToggleFullscreen"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="isFullscreen ? mdiFullscreenExit : mdiFullscreen" />
        </svg>
      </button>

      <!-- 溢出菜单 -->
      <div ref="overflowMenuRef" class="relative">
        <button
          class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
          title="更多选项"
          @click="showOverflowMenu = !showOverflowMenu"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
            <path :d="mdiDotsVertical" />
          </svg>
        </button>

        <Transition
          enter-active-class="transition duration-150 ease-out"
          enter-from-class="opacity-0 translate-y-2 scale-95"
          enter-to-class="opacity-100 translate-y-0 scale-100"
          leave-active-class="transition duration-100 ease-in"
          leave-from-class="opacity-100 translate-y-0 scale-100"
          leave-to-class="opacity-0 translate-y-2 scale-95"
        >
          <div
            v-if="showOverflowMenu"
            class="fixed inset-x-2 bottom-16 z-50 min-w-0 bg-primary-950/90 border border-white/10 backdrop-blur-md rounded-xl p-3 shadow-[0_10px_25px_-5px_rgba(0,0,0,0.5)] flex flex-col gap-2 pointer-events-auto overflow-x-hidden md:absolute md:inset-x-auto md:bottom-full md:mb-2 md:right-0 md:w-80"
          >
            <!-- 小屏设备：动作、复制、打开 -->
            <div class="md:hidden flex flex-col gap-1">
              <!-- 动作列表 -->
              <template v-if="dispatchableHooks.length > 0">
                <div class="text-xs font-bold text-white/40 tracking-wider uppercase select-none">
                  执行动作
                </div>
                <button
                  v-for="hook in dispatchableHooks"
                  :key="hook.id"
                  :disabled="isDispatching"
                  class="px-2 py-1.5 text-xs text-left text-white/80 hover:text-white hover:bg-white/10 rounded-lg transition-colors flex items-center justify-between disabled:opacity-40 disabled:cursor-not-allowed select-none cursor-pointer"
                  :title="hook.description || hook.name"
                  @click="handleDispatch(hook.id, hook.name)"
                >
                  <span class="truncate pr-2">{{ hook.name }}</span>
                  <svg
                    v-if="isDispatching && currentDispatchingHookId === hook.id"
                    class="w-3.5 h-3.5 animate-spin shrink-0 text-secondary-400"
                    viewBox="0 0 24 24"
                    fill="none"
                  >
                    <path
                      :d="mdiLoading"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="3"
                      stroke-linecap="round"
                    />
                  </svg>
                  <svg
                    v-else
                    class="w-3.5 h-3.5 shrink-0 text-white/40"
                    viewBox="0 0 24 24"
                    fill="currentColor"
                  >
                    <path :d="mdiPlayOutline" />
                  </svg>
                </button>
                <div class="border-t border-white/10 my-1"></div>
              </template>

              <!-- 复制按钮 -->
              <button
                class="flex items-center gap-2 select-none transition-colors rounded-lg px-2 py-1.5 text-left"
                :class="[
                  isCopying
                    ? 'text-white/50 opacity-40 cursor-not-allowed'
                    : isCopied
                      ? 'text-secondary-400 hover:text-secondary-300 hover:bg-white/10 cursor-pointer'
                      : 'text-white/80 hover:text-white hover:bg-white/10 cursor-pointer',
                ]"
                :disabled="isCopying"
                @click="handleCopy"
              >
                <svg
                  v-if="isCopying"
                  class="w-4 h-4 animate-spin"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <path :d="mdiLoading" />
                </svg>
                <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path :d="mdiContentCopy" />
                </svg>
                <span class="text-xs">{{
                  isCopying ? "正在复制…" : isCopied ? "已复制" : "复制"
                }}</span>
              </button>

              <!-- 打开按钮 -->
              <button
                class="flex items-center gap-2 cursor-pointer select-none text-white/80 hover:text-white hover:bg-white/10 px-2 py-1.5 rounded-lg transition-colors text-left"
                title="在资源管理器中定位此文件"
                @click="handleOpen"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path :d="mdiOpenInNew" />
                </svg>
                <span class="text-xs">打开</span>
              </button>

              <div class="border-t border-white/10 my-1"></div>
            </div>

            <label
              class="flex items-center gap-2 cursor-pointer select-none text-white/70 hover:text-white transition-colors"
            >
              <input
                v-model="useRawImage"
                type="checkbox"
                class="rounded border-white/30 bg-white/5 text-secondary-600 focus:ring-2 focus:ring-secondary-500/50 focus:ring-offset-1 focus:ring-offset-primary-900 focus:outline-none w-4 h-4"
              />
              <span class="text-xs">原图</span>
            </label>
            <div class="border-t border-white/10 pt-2">
              <div class="text-xs font-bold text-white/40 tracking-wider uppercase mb-1">
                文件路径
              </div>
              <div
                class="select-all bg-white/5 border border-white/10 rounded px-2 py-1 text-xs text-white break-all cursor-text"
                :title="fullFilePath"
              >
                {{ fullFilePath }}
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </div>

    <!-- 笔记编辑对话框 -->
    <noteDialog.component container-class="sm:max-w-3xl short:max-w-none">
      <NoteForm
        v-if="image"
        ref="noteDialogRef"
        :note="image.note"
        :hide-image-button="hideImageButton"
        @close="noteDialog.close"
      />
    </noteDialog.component>
  </div>
</template>

<script setup lang="ts">
import RatingIcon from "./RatingIcon.vue";
import RatingSelector from "./RatingSelector.vue";
import { ref, computed, useTemplateRef, shallowRef, watch, nextTick } from "vue";
import useImageZoom from "../composables/useImageZoom";
import useGrabScroll from "../composables/useGrabScroll";
import useEventListeners from "../composables/useEventListeners";
import useElementFullscreen from "../composables/useElementFullscreen";
import {
  mdiFullscreen,
  mdiFullscreenExit,
  mdiLoading,
  mdiLock,
  mdiLockOpenVariant,
  mdiTagOutline,
  mdiCheck,
  mdiDeleteOutline,
  mdiContentCopy,
  mdiDotsVertical,
  mdiOpenInNew,
  mdiNoteTextOutline,
  mdiPlayOutline,
} from "@mdi/js";
import type { ImageFragment } from "@/graphql/generated";
import useImageLabel, { PRESET_COLORS } from "@/composables/useImageLabel";
import useImage from "@/composables/domain/useImage";
import useClickOutside from "@/composables/useClickOutside";
import { getImageUrlByZoom } from "@/utils/image";
import useCurrentTime from "@/composables/useCurrentTime";
import Time from "@/utils/Time";
import useAsyncTask from "@/composables/useAsyncTask";
import useQuery from "@/graphql/utils/useQuery";
import { MetaDocument } from "@/graphql/generated";
import useImageHooks from "@/composables/useImageHooks";
import { useHotkeys } from "@/composables/useHotkeys";
import { useOpenDir } from "@/composables/useOpenDir";
import NoteForm from "./NoteForm.vue";
import useNote from "@/composables/domain/useNote";
import useModalDialog from "@/composables/useModalDialog";
import { useClipboard } from "@/composables/useClipboard";

const { revealInExplorer } = useOpenDir();

const emit = defineEmits<{
  (e: "image-loaded", payload: { id: string; time: Time }): void;
  (e: "request-next"): void;
}>();

// 接收组件属性。其中 sessionId 是可选的，仅在需要非筛选会话提交耦合的标签设置流程中作为入参
const {
  image,
  sessionId = undefined,
  preloadImages,
  allowPan = () => true,
} = defineProps<{
  image: ImageFragment;
  sessionId?: string;
  preloadImages: ImageFragment[];
  allowPan?: (e: PointerEvent) => boolean;
  hideImageButton?: boolean;
}>();

const metaLoadingCount = ref(0);
const { data: metaData } = useQuery(MetaDocument, {
  loadingCount: metaLoadingCount,
});

const fullFilePath = computed(() => {
  const rootPath = metaData.value?.meta?.rootAbsPath;
  const relPath = image.relPath;
  if (!rootPath || !relPath) {
    return "";
  }
  if (rootPath.includes("\\")) {
    return rootPath + "\\" + relPath.replace(/\//g, "\\");
  }
  return rootPath + "/" + relPath.replace(/\\/g, "/");
});

const showOverflowMenu = ref(false);
const overflowMenuRef = useTemplateRef<HTMLElement>("overflowMenuRef");

useClickOutside(overflowMenuRef, () => {
  showOverflowMenu.value = false;
});

const copyLoadingCount = ref(0);
const { copyWorkflowOrFile, copyFiles, copiedImageIds } = useClipboard({
  loadingCount: copyLoadingCount,
});

const isCopying = computed(() => copyLoadingCount.value > 0);
const isCopied = computed(() => copiedImageIds.value.includes(image.id));

async function handleCopy() {
  if (isCopying.value) return;
  await copyWorkflowOrFile(fullFilePath.value, image.id);
  showOverflowMenu.value = false;
}

async function copyAbsoluteFilePath() {
  if (isCopying.value) return;
  await copyFiles(fullFilePath.value);
}

function handleOpen() {
  revealInExplorer(fullFilePath.value);
  showOverflowMenu.value = false;
}

// #region 快捷键复制

// 绑定快捷键 Ctrl+C 改为与界面复制相同的逻辑（优先复制工作流，没有则复制绝对路径）
useHotkeys(
  {
    "ctrl+c": async (e) => {
      // 若页面上有文本处于被选中状态，则不拦截，走浏览器原生的复制行为
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      if (isCopying.value) return;
      await handleCopy();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制工作流或路径",
    category: "图片操作",
  },
);

// 绑定快捷键 Ctrl+Shift+C 总是直接复制路径
useHotkeys(
  {
    "ctrl+shift+c": async (e) => {
      // 若页面上有文本处于被选中状态，则不拦截，走浏览器原生的复制行为
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      if (isCopying.value) return;
      await copyAbsoluteFilePath();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制绝对路径",
    category: "图片操作",
  },
);
// #endregion

// 使用 composable 提取的 XMP 评分管理逻辑
const { setRating } = useImage(() => image);

const ratingModel = computed({
  get() {
    return image.currentRating;
  },
  set(value) {
    setRating(value);
  },
});

// 绑定快捷键 Ctrl+0~5 用于直接修改评分
for (let r = 0; r <= 5; r++) {
  useHotkeys(
    {
      [`ctrl+digit${r}`]: (e) => {
        e.preventDefault();
        e.stopPropagation();
        setRating(r);
      },
    },
    {
      description: `设置评分为 ${r} 星`,
      category: "图片评分",
    },
  );
}

// 绑定小键盘 0-5 快捷键用于标记评分并切换到下一张
for (let r = 0; r <= 5; r++) {
  useHotkeys(
    {
      [`numpad${r}`]: (e) => {
        e.preventDefault();
        e.stopPropagation();
        setRating(r);
        emit("request-next");
      },
    },
    {
      description: `标记评分为 ${r} 星并切换到下一张`,
      category: "图片评分",
    },
  );
}

// 使用 composable 提取的 XMP 标签管理逻辑
const {
  showPopover,
  customLabelInput,
  currentLabel,
  isPresetColor,
  currentLabelColor,
  setLabel,
  saveCustomLabel,
} = useImageLabel(() => image);

// 绑定标签选择面板的外层容器 ref
const popoverContainerRef = useTemplateRef<HTMLElement>("popoverContainerRef");
// 当点击该标签选择面板外部时，自动收起下拉菜单
useClickOutside(popoverContainerRef, () => {
  showPopover.value = false;
});

const locked = ref(false);
const useRawImage = ref(localStorage.getItem("use-raw-image") === "true");
watch(useRawImage, (val) => {
  localStorage.setItem("use-raw-image", String(val));
});

const containerRef = ref<HTMLElement>();
const rootEl = ref<HTMLElement>();

const { toggleFullscreen, isFullscreen } = useElementFullscreen(rootEl);

function handleToggleFullscreen() {
  toggleFullscreen();
}

const isPinching = ref(false);

const zoom = useImageZoom({
  container: containerRef,
  size: () => image,
  allowTransition: () => loaded.value && !isPinching.value,
});
const { containerAttrs, zoomAsPercent, toggleZoom, zoomIn, zoomOut, zoomAttrs } = zoom;

const src = computed(() =>
  useRawImage.value ? image.rawURL || image.url : getImageUrlByZoom(image, zoom.zoom.value),
);

const activeContainer = computed(() => (locked.value ? null : containerRef.value));

// 主动按顺序预加载后续图片
useAsyncTask({
  args() {
    return [
      [
        useRawImage.value ? image.rawURL || image.url : getImageUrlByZoom(image, zoom.zoom.value),
        ...preloadImages.map((img) =>
          useRawImage.value ? img.rawURL || img.url : getImageUrlByZoom(img, zoom.zoom.value),
        ),
      ],
    ];
  },
  async task(urls, ctx) {
    const concurrency = 8;
    const queue = [...urls];

    const worker = async () => {
      while (queue.length > 0) {
        const url = queue.shift();
        if (!url) {
          break;
        }
        if (ctx.signal().aborted) {
          return;
        }

        const img = new window.Image();
        img.src = url;
        try {
          await img.decode();
        } catch (err) {
          console.error("图片 decode 失败", url, err);
        }
      }
    };

    await Promise.all(Array.from({ length: concurrency }, worker));
  },
});

useGrabScroll(
  () => {
    if (!zoom.fitContainer.value) {
      return activeContainer.value;
    }
  },
  {
    beforeStart: allowPan,
  },
);

const imgEl = useTemplateRef("imgEl");
const loadedId = ref("");
const loaded = computed(() => loadedId.value === image.id);
const lastLoading = shallowRef({ image, startAt: Time.now() });
const { currentTime, refreshOn } = useCurrentTime();
const slowLoadingTimeoutMs = 100;
const isSlowLoading = computed(
  () =>
    !loaded.value &&
    lastLoading.value.image.id === image.id &&
    currentTime.value.sub(lastLoading.value.startAt) > slowLoadingTimeoutMs,
);

watch(
  () => image.id,
  () => {
    lastLoading.value = { image, startAt: Time.now() };
    nextTick(() => {
      updateLoaded();
    });
  },
);

function onLoadStart() {
  if (lastLoading.value.image.id !== image.id) {
    lastLoading.value = { image, startAt: Time.now() };
  }
}
refreshOn(() => lastLoading.value.startAt.add(slowLoadingTimeoutMs + 1));
function updateLoaded() {
  const el = imgEl.value;
  if (el?.complete) {
    loadedId.value = el.dataset.imageId || "";
    emit("image-loaded", { id: loadedId.value, time: Time.now() });
  }
}

let initialPinchDistance = 0;
let initialZoom = 1;

function getTouchDistance(touches: TouchList): number {
  if (touches.length < 2) return 0;
  const dx = touches[0].clientX - touches[1].clientX;
  const dy = touches[0].clientY - touches[1].clientY;
  return Math.sqrt(dx * dx + dy * dy);
}

function getTouchCenter(touches: TouchList) {
  return {
    clientX: (touches[0].clientX + touches[1].clientX) / 2,
    clientY: (touches[0].clientY + touches[1].clientY) / 2,
  };
}

let initialAnchorImage: { x: number; y: number } | undefined;

useEventListeners(containerRef, ({ on }) => {
  on(
    "touchstart",
    (e) => {
      if (locked.value) return;
      if (e.touches.length === 2) {
        isPinching.value = true;
        e.preventDefault();

        initialPinchDistance = getTouchDistance(e.touches);
        initialZoom = zoom.zoom.value;

        // Set anchor based on initial finger position
        const center = getTouchCenter(e.touches);
        const anchor = zoom.anchorFromClientPosition(center);
        if (anchor) {
          zoom.scrollAnchor.value = anchor;
          initialAnchorImage = anchor.image;
        }
      }
    },
    { passive: false },
  );

  on(
    "touchmove",
    (e) => {
      if (e.touches.length === 2) {
        e.preventDefault();

        const currentDistance = getTouchDistance(e.touches);
        if (initialPinchDistance > 0) {
          const scale = currentDistance / initialPinchDistance;
          zoom.zoom.value = Math.max(0.1, Math.min(10, initialZoom * scale));

          // Update anchor to track finger movement (panning while zooming)
          const center = getTouchCenter(e.touches);
          const currentAnchor = zoom.anchorFromClientPosition(center);
          if (currentAnchor && initialAnchorImage) {
            zoom.scrollAnchor.value = {
              viewport: currentAnchor.viewport,
              image: initialAnchorImage,
            };
          }
        }
      }
    },
    { passive: false },
  );

  on("touchend", (e) => {
    if (e.touches.length < 2) {
      isPinching.value = false;
      initialPinchDistance = 0;
      zoom.scrollAnchor.value = undefined;
      initialAnchorImage = undefined;
    }
  });
});

// #region 外部后置动作
const { dispatchableHooks, isDispatching, currentDispatchingHookId, dispatch } = useImageHooks({
  selectedFilterBy: () => ({ id: [image.id] }),
});

const showActionPopover = ref(false);
const actionPopoverContainerRef = useTemplateRef<HTMLElement>("actionPopoverContainerRef");
useClickOutside(actionPopoverContainerRef, () => {
  showActionPopover.value = false;
});

async function handleDispatch(hookId: string, hookName: string) {
  await dispatch(hookId, hookName, { id: [image.id] });
  if (!isDispatching.value) {
    showActionPopover.value = false;
    showOverflowMenu.value = false;
  }
}
// #endregion

const noteDialog = useModalDialog({
  onDidOpen() {
    document.body.style.overflow = "hidden";
    nextTick(() => {
      noteDialogRef.value?.focus();
    });
  },
  onWillClose() {
    document.body.style.overflow = "";
    noteDialogRef.value?.flush();
  },
});

const noteDialogRef = useTemplateRef<InstanceType<typeof NoteForm>>("noteDialogRef");

// 开启当前查看图片的笔记实时订阅，保证外部修改时能自动同步
useNote(() => image.note.id);

// 绑定快捷键 m 或 shift+m 来编辑笔记
useHotkeys(
  {
    "m, shift+m": () => {
      noteDialog.open();
    },
  },
  {
    description: "编辑图片笔记",
    category: "图片操作",
  },
);

// 当笔记编辑器打开时，启用 escape 键以关闭笔记框，同时阻断外层查看器关闭的快捷键
useHotkeys(
  {
    escape: () => {
      noteDialog.close();
    },
  },
  {
    allowInInputs: true,
    preventDefault: true,
    stopPropagation: true,
    description: "关闭笔记",
    scope: noteDialog.scopeId,
    category: "图片操作",
  },
);

// 当笔记编辑器打开时，阻断 arrowleft 和 arrowright 的快捷键处理，避免光标移动误触发图片切换
useHotkeys(
  {
    "arrowleft, arrowright": () => {
      // 仅用于在此上下文拦截左右方向键分发，保留默认光标移动
    },
  },
  {
    allowInInputs: true,
    preventDefault: false,
    stopPropagation: true,
    scope: noteDialog.scopeId,
  },
);

defineExpose({
  zoomIn,
  zoomOut,
  toggleZoom,
  zoom: zoom.zoom,
  fitContainer: zoom.fitContainer,
  zoomAsPercent,
});
</script>
