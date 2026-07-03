<template>
  <div
    class="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 w-[calc(100%-2rem)] max-w-4xl pointer-events-none"
  >
    <Transition name="slide-up">
      <div
        v-if="bulkOps.isBulkMode.value"
        class="pointer-events-auto bg-primary-900/90 backdrop-blur-xl border border-primary-700/80 rounded-2xl shadow-[0_10px_30px_-5px_rgba(0,0,0,0.8)] px-4 py-3 flex flex-col md:flex-row md:items-center justify-between gap-4 transition-all duration-300"
      >
        <!-- 左侧：选择状态与全选控制 -->
        <div class="flex items-center justify-between md:justify-start gap-4">
          <div class="flex items-center gap-2">
            <span
              class="inline-flex items-center justify-center w-max min-w-6 px-1.5 h-6 rounded-full bg-secondary-500/20 border border-secondary-500/30 text-xs font-bold text-secondary-400 animate-pulse"
            >
              {{ bulkOps.selectedCountText }}
            </span>
            <span class="text-xs text-primary-200 font-medium">张图片已选中</span>
          </div>
          <div class="h-4 w-px bg-primary-700 hidden md:block"></div>
          <div class="flex items-center gap-2">
            <button
              v-if="!bulkOps.isAllMatchingSelected.value"
              class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
              @click="bulkOps.selectAll()"
            >
              全选
            </button>
            <button
              v-if="bulkOps.isAllMatchingSelected.value"
              class="px-2 py-1 text-xs text-red-300 hover:text-white bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 rounded-lg transition-colors cursor-pointer select-none font-semibold"
              @click="bulkOps.deselectAll()"
            >
              清除
            </button>
            <button
              v-if="!bulkOps.isAllMatchingSelected.value"
              class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
              @click="bulkOps.invertSelection()"
            >
              反选
            </button>
          </div>
        </div>

        <!-- 右侧：批量动作 -->
        <div class="flex flex-wrap items-center justify-end gap-3">
          <!-- 批量评分 -->
          <div class="relative group/rating">
            <button
              class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
              :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
              :class="[
                !bulkOps.selectedFilterBy.value ? 'opacity-40 cursor-not-allowed' : '',
                activeDropdown === 'rating'
                  ? 'border-secondary-500/50 text-white bg-primary-700'
                  : '',
              ]"
              @click="toggleDropdown('rating', $event)"
            >
              <svg class="w-4 h-4 text-yellow-400" viewBox="0 0 24 24">
                <path :d="mdiStar" fill="currentColor" />
              </svg>
              <span>评星</span>
            </button>

            <!-- 评分悬浮窗 -->
            <div
              v-if="bulkOps.selectedFilterBy.value"
              class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl flex items-center gap-1 z-60 w-max"
              :class="[
                activeDropdown === 'rating'
                  ? 'visible opacity-100'
                  : 'invisible group-hover/rating:visible opacity-0 group-hover/rating:opacity-100',
              ]"
              @click.stop
            >
              <RatingSelector v-model="bulkRating" />
            </div>
          </div>

          <!-- 批量标签 -->
          <div class="relative group/label">
            <button
              class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
              :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
              :class="[
                !bulkOps.selectedFilterBy.value ? 'opacity-40 cursor-not-allowed' : '',
                activeDropdown === 'label'
                  ? 'border-secondary-500/50 text-white bg-primary-700'
                  : '',
              ]"
              @click="toggleDropdown('label', $event)"
            >
              <span
                class="w-3 h-3 rounded-full bg-linear-to-tr from-sky-400 via-green-400 to-yellow-400"
              ></span>
              <span>标签</span>
            </button>

            <!-- 标签悬浮窗 -->
            <div
              v-if="bulkOps.selectedFilterBy.value"
              class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl z-60 w-max"
              :class="[
                activeDropdown === 'label'
                  ? 'visible opacity-100'
                  : 'invisible group-hover/label:visible opacity-0 group-hover/label:opacity-100',
              ]"
              @click.stop
            >
              <div class="flex items-center gap-2">
                <button
                  v-for="(colorHex, colorName) in PRESET_COLORS"
                  :key="colorName"
                  class="w-6 h-6 rounded-full transition-all border border-white/20 hover:scale-120 cursor-pointer relative"
                  :style="{ backgroundColor: colorHex }"
                  :title="colorName"
                  @click="handleBulkSetLabel(colorName)"
                ></button>
                <div class="w-px h-5 bg-primary-700 mx-1"></div>
                <button
                  class="px-2 py-1 text-xs hover:bg-primary-800 border border-primary-700/60 hover:text-white rounded-lg text-primary-300 transition-colors cursor-pointer select-none"
                  @click="handleBulkSetLabel('')"
                >
                  清除
                </button>
              </div>
            </div>
          </div>

          <!-- 批量移动 -->
          <button
            class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
            :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
            :class="!bulkOps.selectedFilterBy.value ? 'opacity-40 cursor-not-allowed' : ''"
            @click="$emit('move')"
          >
            <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiFolderMove" fill="currentColor" />
            </svg>
            <span>移动</span>
          </button>

          <!-- 批量复制 -->
          <button
            class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 select-none"
            :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isCopying.value"
            :class="
              !bulkOps.selectedFilterBy.value || bulkOps.isCopying.value
                ? 'opacity-40 cursor-not-allowed'
                : 'cursor-pointer hover:border-secondary-500/50'
            "
            @click="bulkOps.copySelectedImages()"
          >
            <svg
              v-if="bulkOps.isCopying.value"
              class="w-4 h-4 text-secondary-400 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiContentCopy" fill="currentColor" />
            </svg>
            <span>{{ bulkOps.isCopying.value ? "复制中" : "复制" }}</span>
          </button>

          <!-- 批量动作 -->
          <div v-if="dispatchableHooks.length > 0" class="relative group/hook">
            <button
              class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 select-none"
              :disabled="!bulkOps.selectedFilterBy.value || isBulkDispatching"
              :class="[
                !bulkOps.selectedFilterBy.value || isBulkDispatching
                  ? 'opacity-40 cursor-not-allowed'
                  : 'cursor-pointer hover:border-secondary-500/50',
                activeDropdown === 'hook'
                  ? 'border-secondary-500/50 text-white bg-primary-700'
                  : '',
              ]"
              @click="toggleDropdown('hook', $event)"
            >
              <svg
                v-if="isBulkDispatching"
                class="w-4 h-4 text-secondary-400 animate-spin"
                viewBox="0 0 24 24"
              >
                <path
                  :d="mdiLoading"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="3"
                  stroke-linecap="round"
                />
              </svg>
              <svg v-else class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
                <path :d="mdiPlayOutline" fill="currentColor" />
              </svg>
              <span>动作</span>
            </button>

            <!-- 动作悬浮窗 -->
            <div
              v-if="bulkOps.selectedFilterBy.value"
              class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl z-60 w-52 flex flex-col gap-1 text-left"
              :class="[
                activeDropdown === 'hook'
                  ? 'visible opacity-100'
                  : 'invisible group-hover/hook:visible opacity-0 group-hover/hook:opacity-100',
              ]"
              @click.stop
            >
              <div
                class="text-xs font-bold text-primary-400 tracking-wider uppercase select-none px-2 py-1"
              >
                选择执行动作
              </div>
              <button
                v-for="hook in dispatchableHooks"
                :key="hook.id"
                class="px-2 py-1 text-xs text-left text-primary-200 hover:text-white hover:bg-primary-800 rounded-lg transition-colors flex items-center justify-between cursor-pointer select-none"
                :title="hook.description || hook.name"
                @click="handleBulkDispatch(hook.id, hook.name)"
              >
                <span class="truncate pr-2">{{ hook.name }}</span>
                <svg
                  class="w-4 h-4 shrink-0 text-primary-500"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <path :d="mdiPlayOutline" fill="currentColor" />
                </svg>
              </button>
            </div>
          </div>

          <div class="h-5 w-px bg-primary-700"></div>

          <!-- 关闭批量管理模式 -->
          <button
            class="px-3 h-9 text-xs font-semibold bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 rounded-xl transition-colors cursor-pointer flex items-center gap-1 select-none"
            @click="bulkOps.toggle()"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiClose" fill="currentColor" />
            </svg>
            <span>退出</span>
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";
import {
  mdiStar,
  mdiFolderMove,
  mdiContentCopy,
  mdiLoading,
  mdiPlayOutline,
  mdiClose,
} from "@mdi/js";
import { PRESET_COLORS } from "@/composables/useImageLabel";
import RatingSelector from "./RatingSelector.vue";
import useImageHooks from "@/composables/useImageHooks";
import useEventListeners from "@/composables/useEventListeners";
import type { ImageFragment, ImageFiltersInput } from "@/graphql/generated";
import type { Ref, ComputedRef } from "vue";

interface BulkOps {
  isBulkMode: Ref<boolean>;
  selectedCountText: ComputedRef<string>;
  isAllMatchingSelected: Ref<boolean>;
  selectedFilterBy: ComputedRef<ImageFiltersInput | undefined>;
  isUpdating: Ref<boolean>;
  selectedImages: ComputedRef<ImageFragment[]>;
  selectAll: () => void;
  deselectAll: () => void;
  invertSelection: () => void;
  toggle: () => void;
  setRating: (rating: number) => Promise<void>;
  setLabel: (label: string) => Promise<void>;
  copySelectedImages: () => Promise<void>;
  isCopying: Ref<boolean>;
}

const props = defineProps<{
  bulkOps: BulkOps;
}>();

defineEmits<{
  move: [];
}>();

// #region 下拉菜单管理
const activeDropdown = ref<"rating" | "label" | "hook" | null>(null);

function toggleDropdown(menu: "rating" | "label" | "hook", event: Event) {
  event.stopPropagation();
  if (activeDropdown.value === menu) {
    activeDropdown.value = null;
  } else {
    activeDropdown.value = menu;
  }
}

function closeDropdowns() {
  activeDropdown.value = null;
}

useEventListeners(document, ({ on }) => {
  on("click", closeDropdowns);
});

watch([() => props.bulkOps.isBulkMode.value, props.bulkOps.selectedFilterBy], () => {
  if (!props.bulkOps.isBulkMode.value || !props.bulkOps.selectedFilterBy.value) {
    closeDropdowns();
  }
});
// #endregion

// #region 批量评分 computed
const bulkRating = computed<number>({
  get() {
    const images = props.bulkOps.selectedImages.value;
    if (images.length === 0) return 0;
    const firstImg = images[0];
    const rating = firstImg.currentRating || 0;
    const allSame = images.every((img) => (img.currentRating || 0) === rating);
    return allSame ? rating : 0;
  },
  set(val) {
    if (typeof val === "number") {
      void handleBulkSetRating(val);
    }
  },
});

async function handleBulkSetRating(rating: number) {
  await props.bulkOps.setRating(rating);
  closeDropdowns();
}

async function handleBulkSetLabel(label: string) {
  await props.bulkOps.setLabel(label);
  closeDropdowns();
}

async function handleBulkDispatch(hookId: string, hookName: string) {
  const filterBy = props.bulkOps.selectedFilterBy.value;
  if (!filterBy) return;
  await dispatch(hookId, hookName, filterBy);
  closeDropdowns();
}
// #endregion

// #region 动作派发
const {
  dispatchableHooks,
  isDispatching: isBulkDispatching,
  dispatch,
} = useImageHooks({
  selectedFilterBy: () => props.bulkOps.selectedFilterBy.value,
});
// #endregion

// #endregion
</script>

<style scoped>
.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.slide-up-enter-from {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}
</style>
