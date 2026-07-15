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
          <AppDropdown
            placement="top-end"
            content-class="w-max flex items-center gap-1"
            :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
          >
            <template #trigger="{ isOpen, toggle }">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
                :class="[
                  !bulkOps.selectedFilterBy.value ? 'opacity-40 cursor-not-allowed' : '',
                  isOpen ? 'border-secondary-500/50 text-white bg-primary-700' : '',
                ]"
                @click="toggle"
              >
                <svg class="w-4 h-4 text-yellow-400" viewBox="0 0 24 24">
                  <path :d="mdiStar" fill="currentColor" />
                </svg>
                <span>评星</span>
              </button>
            </template>
            <template #content="{ close }">
              <RatingSelector
                :modelValue="bulkRating"
                @update:modelValue="(val) => handleBulkSetRating(val, close)"
              />
            </template>
          </AppDropdown>

          <!-- 批量标签 -->
          <AppDropdown
            placement="top-end"
            content-class="w-max"
            :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
          >
            <template #trigger="{ isOpen, toggle }">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="!bulkOps.selectedFilterBy.value || bulkOps.isUpdating.value"
                :class="[
                  !bulkOps.selectedFilterBy.value ? 'opacity-40 cursor-not-allowed' : '',
                  isOpen ? 'border-secondary-500/50 text-white bg-primary-700' : '',
                ]"
                @click="toggle"
              >
                <span
                  class="w-3 h-3 rounded-full bg-linear-to-tr from-sky-400 via-green-400 to-yellow-400"
                ></span>
                <span>标签</span>
              </button>
            </template>
            <template #content="{ close }">
              <div class="flex items-center gap-2">
                <button
                  v-for="(colorHex, colorName) in PRESET_COLORS"
                  :key="colorName"
                  class="w-6 h-6 rounded-full transition-all border border-white/20 hover:scale-120 cursor-pointer relative"
                  :style="{ backgroundColor: colorHex }"
                  :title="colorName"
                  @click="handleBulkSetLabel(colorName, close)"
                ></button>
                <div class="w-px h-5 bg-primary-700 mx-1"></div>
                <button
                  class="px-2 py-1 text-xs hover:bg-primary-800 border border-primary-700/60 hover:text-white rounded-lg text-primary-300 transition-colors cursor-pointer select-none"
                  @click="handleBulkSetLabel('', close)"
                >
                  清除
                </button>
              </div>
            </template>
          </AppDropdown>

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
          <AppDropdown
            v-if="dispatchableHooks.length > 0"
            placement="top-end"
            content-class="w-52 flex flex-col gap-1 text-left"
            :disabled="!bulkOps.selectedFilterBy.value || isBulkDispatching"
          >
            <template #trigger="{ isOpen, toggle }">
              <button
                class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 select-none"
                :disabled="!bulkOps.selectedFilterBy.value || isBulkDispatching"
                :class="[
                  !bulkOps.selectedFilterBy.value || isBulkDispatching
                    ? 'opacity-40 cursor-not-allowed'
                    : 'cursor-pointer hover:border-secondary-500/50',
                  isOpen ? 'border-secondary-500/50 text-white bg-primary-700' : '',
                ]"
                @click="toggle"
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
            </template>
            <template #content="{ close }">
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
                @click="handleBulkDispatch(hook.id, hook.name, close)"
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
            </template>
          </AppDropdown>

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
import { computed } from "vue";
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
import AppDropdown from "./AppDropdown.vue";
import useImageHooks from "@/composables/useImageHooks";
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

// #region 批量评分 computed
const bulkRating = computed<number>(() => {
  const images = props.bulkOps.selectedImages.value;
  if (images.length === 0) return 0;
  const firstImg = images[0];
  const rating = firstImg.currentRating || 0;
  const allSame = images.every((img) => (img.currentRating || 0) === rating);
  return allSame ? rating : 0;
});

async function handleBulkSetRating(
  rating: number | null | readonly number[] | undefined,
  closeDropdown: () => void,
) {
  if (typeof rating === "number") {
    await props.bulkOps.setRating(rating);
    closeDropdown();
  }
}

async function handleBulkSetLabel(label: string, closeDropdown: () => void) {
  await props.bulkOps.setLabel(label);
  closeDropdown();
}

async function handleBulkDispatch(hookId: string, hookName: string, closeDropdown: () => void) {
  const filterBy = props.bulkOps.selectedFilterBy.value;
  if (!filterBy) return;
  await dispatch(hookId, hookName, filterBy);
  closeDropdown();
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
  transition:
    opacity 0.3s cubic-bezier(0.16, 1, 0.3, 1),
    transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
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
