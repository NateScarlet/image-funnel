<template>
  <div class="space-y-6">
    <!-- 图片网格展示区 -->
    <section
      class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
    >
      <!-- 图片列表标题与图片专用的筛选过滤条件 -->
      <div
        class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-primary-700/50 pb-3"
      >
        <h2
          class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
        >
          <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiImage" fill="currentColor" />
          </svg>
          图片列表 ({{ images.length }} 张)
        </h2>

        <div class="flex flex-wrap items-center gap-3">
          <!-- 当用户激活了任何过滤器时，在最左侧显示一键清除筛选按钮 -->
          <button
            v-if="hasActiveFilters"
            class="px-2.5 h-[34px] text-xs border rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border-red-900/50 text-red-300 cursor-pointer"
            @click="clearFilters"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiFilterOff" fill="currentColor" />
            </svg>
            <span>清除筛选</span>
          </button>

          <!-- 搜索输入框 -->
          <div class="relative min-w-36 max-w-60 flex-1 sm:flex-none">
            <input
              v-model="searchQuery"
              type="text"
              placeholder="搜索文件名..."
              class="w-full pl-8 pr-8 h-8 bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-xs text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all"
            />
            <svg
              class="w-3.5 h-3.5 text-primary-400 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none"
              viewBox="0 0 24 24"
            >
              <path :d="mdiMagnify" fill="currentColor" />
            </svg>
            <button
              v-if="searchQuery"
              class="absolute right-2.5 top-1/2 -translate-y-1/2 text-primary-400 hover:text-primary-200 transition-colors p-0.5 rounded-full hover:bg-primary-700/50 cursor-pointer"
              title="清空"
              @click="searchQuery = ''"
            >
              <svg class="w-3 h-3" viewBox="0 0 24 24">
                <path :d="mdiClose" fill="currentColor" />
              </svg>
            </button>
          </div>

          <!-- 评星过滤器 -->
          <RatingFilter v-model="filterRating" />

          <!-- 颜色标签过滤器 -->
          <div
            class="flex items-center gap-1.5 bg-primary-800 border border-primary-700 px-3 h-[34px] rounded-lg overflow-x-auto"
          >
            <span class="text-xs text-primary-400 select-none">标签:</span>
            <div class="flex items-center gap-1">
              <button
                v-for="(colorHex, colorName) in PRESET_COLORS"
                :key="colorName"
                class="w-3.5 h-3.5 rounded-full transition-all border border-white/20 relative"
                :style="{
                  backgroundColor: colorHex,
                  borderColor: filterLabels.includes(colorName)
                    ? 'white'
                    : undefined,
                }"
                :class="[
                  filterLabels.includes(colorName)
                    ? 'scale-115 shadow-[0_0_8px_rgba(255,255,255,0.6)]'
                    : 'opacity-60 hover:opacity-100 hover:scale-110',
                ]"
                :title="colorName"
                @click="toggleLabelFilter(colorName)"
              >
                <!-- 选中指示点 -->
                <span
                  v-if="filterLabels.includes(colorName)"
                  class="absolute inset-0.5 rounded-full border border-black/30"
                ></span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 骨架图加载指示，避免布局抖动 -->
      <div
        v-if="loading && images.length === 0"
        class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 animate-pulse"
      >
        <div
          v-for="n in 16"
          :key="n"
          class="aspect-square bg-primary-800/50 rounded-xl"
        ></div>
      </div>

      <!-- 无图片空状态 -->
      <div
        v-else-if="images.length === 0"
        class="flex flex-col items-center justify-center py-20 text-primary-500 gap-2"
      >
        <svg
          class="w-12 h-12 stroke-[1.5]"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 00-1.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"
          />
        </svg>
        <span class="text-sm">该目录或过滤条件下未找到任何图片</span>
      </div>

      <!-- 网格列表 -->
      <div
        v-else
        class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4"
      >
        <div
          v-for="(img, index) in images"
          :key="img.id"
          class="group relative bg-primary-800/40 hover:bg-primary-800/90 border border-primary-800 hover:border-primary-600/80 rounded-xl overflow-hidden aspect-square cursor-pointer transition-all hover:scale-[1.02] hover:shadow-lg hover:shadow-black/40 flex flex-col justify-between"
          @click="openViewer(index)"
        >
          <!-- 缩略图加载 -->
          <div
            class="w-full h-full relative overflow-hidden bg-black/10 flex items-center justify-center"
          >
            <img
              :src="img.url256 || img.url"
              :alt="img.filename"
              loading="lazy"
              class="object-cover w-full h-full select-none"
            />

            <!-- 评星与标签的悬浮徽章 -->
            <div
              class="absolute bottom-2 left-2 right-2 flex items-center justify-between pointer-events-none opacity-90 group-hover:opacity-100 transition-opacity"
            >
              <!-- 评分图标 -->
              <RatingIcon
                v-if="img.currentRating"
                :rating="img.currentRating"
                filled
              />

              <!-- 颜色标签 -->
              <span
                v-if="img.label"
                class="w-3.5 h-3.5 rounded-full shadow-md border border-white/20 ml-auto"
                :style="{
                  backgroundColor: PRESET_COLORS[img.label] || '#94a3b8',
                }"
                :title="img.label"
              ></span>
            </div>
          </div>

          <!-- 卡片底部的文件名遮罩 -->
          <div
            class="absolute inset-x-0 top-0 bg-gradient-to-b from-black/80 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
          >
            <p
              class="text-[10px] text-white font-medium truncate"
              :title="img.filename"
            >
              {{ img.filename }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- 懒加载过渡区与加载更多按钮 -->
    <section v-if="hasNextPage" class="flex justify-center pt-4">
      <button
        :disabled="loading"
        class="px-6 py-2.5 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded-xl text-sm font-semibold transition-all flex items-center gap-2 text-primary-200 hover:text-white"
        @click="loadMore"
      >
        <!-- 加载中动画 -->
        <svg
          v-if="loading"
          class="w-4 h-4 animate-spin text-secondary-500"
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
        <span>{{ loading ? "正在加载..." : "加载更多图片" }}</span>
      </button>
    </section>

    <!-- 全屏查看器遮罩层 -->
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="currentImageIndex !== undefined && currentImage"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/95 backdrop-blur-sm select-none"
      >
        <!-- 侧边关闭按钮 -->
        <button
          class="absolute top-4 right-4 z-[60] p-2 rounded-full bg-white/5 hover:bg-white/10 text-white/70 hover:text-white transition-colors border border-white/10"
          title="关闭查看器 (Esc)"
          @click="closeViewer"
        >
          <svg class="w-6 h-6" viewBox="0 0 24 24">
            <path :d="mdiClose" fill="currentColor" />
          </svg>
        </button>

        <!-- 上一张按钮 -->
        <button
          v-if="currentImageIndex > 0"
          class="absolute left-4 top-1/2 -translate-y-1/2 z-[60] p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="上一张图片 (ArrowLeft)"
          @click="prevImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronLeft" fill="currentColor" />
          </svg>
        </button>

        <!-- 下一张按钮 -->
        <button
          v-if="currentImageIndex < images.length - 1"
          class="absolute right-4 top-1/2 -translate-y-1/2 z-[60] p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="下一张图片 (ArrowRight)"
          @click="nextImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronRight" fill="currentColor" />
          </svg>
        </button>

        <!-- 图像查看器组件 -->
        <div class="w-full h-full flex flex-col justify-between">
          <ImageViewer :image="currentImage" class="w-full h-full flex-1">
            <!-- 插入底部信息 -->
            <template #info>
              <span
                class="truncate max-w-72 font-semibold"
                :title="currentImage.filename"
              >
                {{ currentImage.filename }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span>
                {{ (currentImageIndex || 0) + 1 }} / {{ images.length }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span class="text-white/60">
                {{ currentImage.width || 0 }}x{{ currentImage.height || 0 }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span class="text-white/60">
                {{ formatSize(currentImage.size) }}
              </span>
            </template>
          </ImageViewer>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import {
  mdiImage,
  mdiFilterOff,
  mdiMagnify,
  mdiClose,
  mdiLoading,
  mdiChevronLeft,
  mdiChevronRight,
} from "@mdi/js";
import { PRESET_COLORS } from "@/composables/useImageLabel";
import RatingIcon from "./RatingIcon.vue";
import RatingFilter from "./RatingFilter.vue";
import ImageViewer from "./ImageViewer.vue";
import useHotkey from "@/composables/useHotkey";
import { useDirectoryState } from "@/composables/useDirectoryState";
import useBrowseImages from "@/composables/useBrowseImages";
import { formatSize } from "@/utils/formatSize";
import type {
  BrowseImagesQueryVariables,
  ImageFiltersInput,
} from "@/graphql/generated";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
}>();
// #endregion

// #region 状态管理
const {
  filterRating,
  filterLabels,
  searchQuery,
  hasActiveFilters,
  clearFilters,
} = useDirectoryState(() => props.directoryId);

// 提取加载状态计数，以实现精细的骨架图切换与加载动画
const loadingCount = ref(0);

// 构建图片查询 variables
const imagesVariables = computed<BrowseImagesQueryVariables>(() => {
  const filterBy: ImageFiltersInput = {
    rating: filterRating.value,
    label: filterLabels.value.length > 0 ? filterLabels.value : null,
    query: searchQuery.value || null,
  };
  return {
    id: props.directoryId,
    filterBy,
    first: 100, // 每页 100 张
    after: null,
  };
});

// 对 loading 状态的综合追踪
const loading = computed(() => loadingCount.value > 0);

// 调用 useBrowseImages 获取图片列表
const {
  images,
  hasNextPage,
  loadMore: imagesLoadMore,
} = useBrowseImages(imagesVariables, { loadingCount });

// 触发分页加载更多图片
function loadMore() {
  if (loading.value || !hasNextPage.value) return;
  imagesLoadMore();
}
// #endregion

// #region 过滤器操作逻辑
function toggleLabelFilter(label: string) {
  const nextLabels = [...filterLabels.value];
  const index = nextLabels.indexOf(label);
  if (index >= 0) {
    nextLabels.splice(index, 1);
  } else {
    nextLabels.push(label);
  }
  filterLabels.value = nextLabels;
}
// #endregion

// #region 全屏查看器模块
const currentImageIndex = ref<number | undefined>(undefined);
const currentImage = computed(() => {
  if (currentImageIndex.value === undefined) return undefined;
  return images.value[currentImageIndex.value];
});

function openViewer(index: number) {
  currentImageIndex.value = index;
}

function closeViewer() {
  currentImageIndex.value = undefined;
}

function prevImage() {
  if (currentImageIndex.value !== undefined && currentImageIndex.value > 0) {
    currentImageIndex.value--;
  }
}

function nextImage() {
  if (
    currentImageIndex.value !== undefined &&
    currentImageIndex.value < images.value.length - 1
  ) {
    currentImageIndex.value++;
  }
}

// 查看器打开时：左右方向键切换图片，Esc 关闭查看器
const isViewerOpen = computed(() => currentImageIndex.value !== undefined);

useHotkey(
  "arrowleft",
  () => {
    prevImage();
  },
  {
    allowInInputs: true,
    description: "上一张图片",
    enabled: isViewerOpen,
    category: "图片浏览",
  },
);
useHotkey(
  "arrowright",
  () => {
    nextImage();
  },
  {
    allowInInputs: true,
    description: "下一张图片",
    enabled: isViewerOpen,
    category: "图片浏览",
  },
);
useHotkey(
  "escape",
  () => {
    closeViewer();
  },
  {
    allowInInputs: true,
    description: "关闭查看器",
    enabled: isViewerOpen,
    category: "图片浏览",
  },
);
// #endregion
</script>
