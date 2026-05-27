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
          <!-- 移动匹配图片按钮 -->
          <button
            v-if="images.length > 0 && !isBulkMode"
            class="px-2.5 h-8.5 text-xs border rounded-lg transition-all flex items-center gap-1 bg-primary-800 hover:bg-primary-750 border-primary-700 text-primary-200 cursor-pointer select-none"
            title="将当前过滤匹配的图片移动到新目录"
            @click="moveImagesDialog.open()"
          >
            <svg class="w-3.5 h-3.5 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiFolderMove" fill="currentColor" />
            </svg>
            <span>移动匹配图片</span>
          </button>

          <!-- 批量管理按钮 -->
          <button
            v-if="images.length > 0"
            class="px-2.5 h-8.5 text-xs border rounded-lg transition-all flex items-center gap-1 cursor-pointer select-none"
            :class="[
              isBulkMode
                ? 'bg-secondary-600 hover:bg-secondary-700 border-secondary-500 text-white shadow-[0_0_10px_rgba(235,94,85,0.3)] font-semibold'
                : 'bg-primary-800 hover:bg-primary-750 border-primary-700 text-primary-200',
            ]"
            :title="
              isBulkMode
                ? '退出批量管理模式'
                : '进入批量管理模式，对多张图片执行操作'
            "
            @click="toggleBulkMode"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiCheckboxMultipleMarkedOutline" fill="currentColor" />
            </svg>
            <span>{{ isBulkMode ? "退出批量" : "批量管理" }}</span>
          </button>

          <!-- 当用户激活了任何过滤器时，在最左侧显示一键清除筛选按钮 -->
          <button
            v-if="hasActiveFilters"
            class="px-2.5 h-8.5 text-xs border rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border-red-900/50 text-red-300 cursor-pointer"
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
            class="flex items-center gap-1.5 bg-primary-800 border border-primary-700 px-3 h-8.5 rounded-lg overflow-x-auto"
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

      <!-- 滚动容器：包裹列表、空状态、骨架图与加载更多按钮 -->
      <div class="max-h-[60vh] overflow-y-auto pr-1 space-y-4">
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
            v-for="img in images"
            :key="img.id"
            class="group relative bg-primary-800/40 hover:bg-primary-800/90 border rounded-xl overflow-hidden aspect-square cursor-pointer transition-all hover:scale-[1.02] hover:shadow-lg hover:shadow-black/40 flex flex-col justify-between"
            :class="[
              isBulkMode && selectedImageIds.includes(img.id)
                ? 'border-secondary-500 ring-2 ring-secondary-500/50 bg-primary-800/90 scale-[1.02]'
                : 'border-primary-800 hover:border-primary-600/80',
            ]"
            @click="handleImageClick(img)"
          >
            <!-- 缩略图加载 -->
            <div
              class="w-full h-full relative overflow-hidden bg-black/10 flex items-center justify-center"
            >
              <!-- 左上角勾选徽章 -->
              <div
                v-if="isBulkMode"
                class="absolute top-2 left-2 z-10 w-5.5 h-5.5 rounded-full flex items-center justify-center transition-all duration-200 border cursor-pointer"
                :class="[
                  selectedImageIds.includes(img.id)
                    ? 'bg-secondary-500 border-secondary-400 text-white shadow-[0_2px_8px_rgba(235,94,85,0.4)] scale-110'
                    : 'bg-black/40 border-white/20 text-white/50 opacity-0 group-hover:opacity-100 hover:scale-105',
                ]"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
                  <path
                    :d="mdiCheck"
                    fill="currentColor"
                    stroke="currentColor"
                    stroke-width="1.5"
                  />
                </svg>
              </div>

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
              class="absolute inset-x-0 top-0 bg-linear-to-b from-black/80 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
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

        <!-- 懒加载过渡区与加载更多按钮 -->
        <div v-if="hasNextPage" class="flex justify-center pt-2">
          <button
            :disabled="loading"
            class="px-6 py-2.5 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded-xl text-sm font-semibold transition-all flex items-center gap-2 text-primary-200 hover:text-white"
            @click="fetchMore"
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
        </div>
      </div>
    </section>

    <!-- 全屏查看器模态框 -->
    <imageViewerDialog.component
      v-if="currentImageId !== undefined && currentImage"
      @after-leave="handleViewerAfterLeave"
    >
      <div class="w-full h-full flex flex-col justify-between">
        <!-- 侧边关闭按钮 -->
        <button
          class="absolute top-4 right-4 z-60 p-2 rounded-full bg-white/5 hover:bg-white/10 text-white/70 hover:text-white transition-colors border border-white/10"
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
          class="absolute left-4 top-1/2 -translate-y-1/2 z-60 p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="上一张图片 (ArrowLeft)"
          @click="prevImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronLeft" fill="currentColor" />
          </svg>
        </button>

        <!-- 下一张按钮 -->
        <button
          v-if="currentImageIndex >= 0 && currentImageIndex < images.length - 1"
          class="absolute right-4 top-1/2 -translate-y-1/2 z-60 p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="下一张图片 (ArrowRight)"
          @click="nextImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronRight" fill="currentColor" />
          </svg>
        </button>

        <!-- 图像查看器组件 -->
        <ImageViewer
          :image="currentImage"
          class="w-full h-full flex-1"
          @request-next="nextImage"
        >
          <!-- 插入底部信息 -->
          <template #info>
            <span
              class="truncate max-w-72 font-semibold"
              :title="currentImage.filename"
            >
              {{ currentImage.filename }}
            </span>
            <div class="w-px h-4 bg-white/30 mx-1"></div>
            <span> {{ currentImageIndex + 1 }} / {{ images.length }} </span>
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
    </imageViewerDialog.component>

    <!-- 移动图片模态框 -->
    <moveImagesDialog.component container-class="sm:max-w-md p-6">
      <MoveImagesForm
        :directory-id="directoryId"
        :filter-by="moveImagesFilterBy"
        :match-count="moveImagesMatchCount"
        @close="handleMoveClose"
      />
    </moveImagesDialog.component>

    <!-- 批量操作底栏 -->
    <div
      class="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 w-[calc(100%-2rem)] max-w-4xl pointer-events-none"
    >
      <Transition name="slide-up">
        <div
          v-if="isBulkMode"
          class="pointer-events-auto bg-primary-900/90 backdrop-blur-xl border border-primary-700/80 rounded-2xl shadow-[0_10px_30px_-5px_rgba(0,0,0,0.8)] px-4 py-3 flex flex-col md:flex-row md:items-center justify-between gap-4 transition-all duration-300"
        >
          <!-- 左侧：选择状态与全选控制 -->
          <div class="flex items-center justify-between md:justify-start gap-4">
            <div class="flex items-center gap-2">
              <span
                class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-secondary-500/20 border border-secondary-500/30 text-xs font-bold text-secondary-400 animate-pulse"
              >
                {{ selectedImageIds.length }}
              </span>
              <span class="text-xs text-primary-200 font-medium"
                >张图片已选中</span
              >
            </div>
            <div class="h-4 w-px bg-primary-750 hidden md:block"></div>
            <div class="flex items-center gap-2">
              <button
                class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
                @click="selectAll"
              >
                全选
              </button>
              <button
                class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
                :disabled="selectedImageIds.length === 0"
                :class="
                  selectedImageIds.length === 0
                    ? 'opacity-50 cursor-not-allowed'
                    : ''
                "
                @click="deselectAll"
              >
                取消全选
              </button>
            </div>
          </div>

          <!-- 右侧：批量动作 -->
          <div class="flex flex-wrap items-center justify-end gap-3">
            <!-- 批量评分 -->
            <div class="relative group/rating">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-750 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-1.5 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="selectedImageIds.length === 0 || isUpdating"
                :class="
                  selectedImageIds.length === 0
                    ? 'opacity-40 cursor-not-allowed'
                    : ''
                "
              >
                <svg class="w-4 h-4 text-yellow-400" viewBox="0 0 24 24">
                  <path :d="mdiStar" fill="currentColor" />
                </svg>
                <span>批量评星</span>
              </button>

              <!-- 评分悬浮窗 -->
              <div
                v-if="selectedImageIds.length > 0"
                class="absolute bottom-full right-0 mb-2 invisible group-hover/rating:visible opacity-0 group-hover/rating:opacity-100 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2.5 rounded-xl shadow-xl flex items-center gap-1 z-60 w-max"
              >
                <button
                  class="px-2 py-1 text-xs hover:bg-red-950/40 border border-transparent hover:border-red-900/50 rounded-lg text-red-400 transition-colors cursor-pointer select-none"
                  @click="bulkSetRating(0)"
                >
                  无评分
                </button>
                <div class="w-px h-4 bg-primary-750"></div>
                <button
                  v-for="r in [1, 2, 3, 4, 5]"
                  :key="r"
                  class="p-1 text-primary-300 hover:text-yellow-400 transition-colors cursor-pointer"
                  :title="`设置为 ${r} 星`"
                  @click="bulkSetRating(r)"
                >
                  <svg class="w-5 h-5" viewBox="0 0 24 24">
                    <path :d="mdiStar" fill="currentColor" />
                  </svg>
                </button>
              </div>
            </div>

            <!-- 批量标签 -->
            <div class="relative group/label">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-750 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-1.5 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="selectedImageIds.length === 0 || isUpdating"
                :class="
                  selectedImageIds.length === 0
                    ? 'opacity-40 cursor-not-allowed'
                    : ''
                "
              >
                <span
                  class="w-3 h-3 rounded-full bg-linear-to-tr from-sky-400 via-green-400 to-yellow-400"
                ></span>
                <span>颜色标签</span>
              </button>

              <!-- 标签悬浮窗 -->
              <div
                v-if="selectedImageIds.length > 0"
                class="absolute bottom-full right-0 mb-2 invisible group-hover/label:visible opacity-0 group-hover/label:opacity-100 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2.5 rounded-xl shadow-xl z-60 w-max"
              >
                <div class="flex items-center gap-1.5">
                  <button
                    v-for="(colorHex, colorName) in PRESET_COLORS"
                    :key="colorName"
                    class="w-5.5 h-5.5 rounded-full transition-all border border-white/20 hover:scale-120 cursor-pointer relative"
                    :style="{ backgroundColor: colorHex }"
                    :title="colorName"
                    @click="bulkSetLabel(colorName)"
                  ></button>
                  <div class="w-px h-5 bg-primary-750 mx-1"></div>
                  <button
                    class="px-2 py-1 text-xs hover:bg-primary-800 border border-primary-700/60 hover:text-white rounded-lg text-primary-300 transition-colors cursor-pointer select-none"
                    @click="bulkSetLabel('')"
                  >
                    清除
                  </button>
                </div>
              </div>
            </div>

            <!-- 批量移动 -->
            <button
              class="px-3.5 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-750 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-1.5 cursor-pointer hover:border-secondary-500/50 select-none"
              :disabled="selectedImageIds.length === 0 || isUpdating"
              :class="
                selectedImageIds.length === 0
                  ? 'opacity-40 cursor-not-allowed'
                  : ''
              "
              @click="moveImagesDialog.open()"
            >
              <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
                <path :d="mdiFolderMove" fill="currentColor" />
              </svg>
              <span>批量移动</span>
            </button>

            <div class="h-5 w-px bg-primary-750"></div>

            <!-- 关闭批量管理模式 -->
            <button
              class="px-3 h-9 text-xs font-semibold bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 rounded-xl transition-colors cursor-pointer flex items-center gap-1 select-none"
              @click="toggleBulkMode"
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watchEffect, onMounted, nextTick } from "vue";
import {
  mdiImage,
  mdiFilterOff,
  mdiMagnify,
  mdiClose,
  mdiLoading,
  mdiChevronLeft,
  mdiChevronRight,
  mdiFolderMove,
  mdiCheck,
  mdiCheckboxMultipleMarkedOutline,
  mdiStar,
} from "@mdi/js";
import { PRESET_COLORS } from "@/composables/useImageLabel";
import RatingIcon from "./RatingIcon.vue";
import useBulkOperations from "@/composables/useBulkOperations";
import RatingFilter from "./RatingFilter.vue";
import ImageViewer from "./ImageViewer.vue";
import useHotkey from "@/composables/useHotkey";
import { useDirectoryState } from "@/composables/useDirectoryState";
import useBrowseImages from "@/composables/useBrowseImages";
import { formatSize } from "@/utils/formatSize";
import type {
  BrowseImagesQueryVariables,
  ImageFiltersInput,
  ImageFragment,
} from "@/graphql/generated";
import MoveImagesForm from "./MoveImagesForm.vue";
import useModalDialog from "@/composables/useModalDialog";
import useModalFullscreen from "@/composables/useModalFullscreen";
import { openImageViewerByFilename } from "@/events";
import useLocationHash from "@/composables/useLocationHash";

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
const loadingCount = ref(1);
onMounted(() => {
  nextTick(() => {
    loadingCount.value -= 1;
  });
});

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
    first: 20,
  };
});

// 对 loading 状态的综合追踪
const loading = computed(() => loadingCount.value > 0);

// 调用 useBrowseImages 获取图片列表
const { images, hasNextPage, fetchMore } = useBrowseImages(imagesVariables, {
  loadingCount,
});

// #region 批量操作状态与逻辑管理
const {
  isBulkMode,
  selectedImageIds,
  isUpdating,
  toggleBulkMode,
  toggleSelectImage,
  selectAll,
  deselectAll,
  bulkSetRating,
  bulkSetLabel,
} = useBulkOperations(images);

// 在批量模式下，根据选中的图片 ID 动态生成 filterBy 和匹配图片数量
const moveImagesFilterBy = computed<ImageFiltersInput>(() => {
  if (isBulkMode.value) {
    return {
      id: selectedImageIds.value,
    };
  }
  return imagesVariables.value.filterBy || {};
});

const moveImagesMatchCount = computed(() => {
  if (isBulkMode.value) {
    return selectedImageIds.value.length;
  }
  return images.value.length;
});

// 批量模式下移动成功后，关闭弹框，过滤掉已不存在于当前列表的图片ID并保持批量模式
function handleMoveClose() {
  moveImagesDialog.close();
  if (isBulkMode.value) {
    const currentIds = images.value.map((img) => img.id);
    selectedImageIds.value = selectedImageIds.value.filter((id) =>
      currentIds.includes(id),
    );
  }
}

// 批量模式下点击图片执行选择，正常模式打开大图查看器
function handleImageClick(img: ImageFragment) {
  if (isBulkMode.value) {
    toggleSelectImage(img.id);
  } else {
    openViewer(img);
  }
}
// #endregion
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
const currentImageId = ref<string | undefined>(undefined);
const currentImage = computed(() => {
  if (currentImageId.value === undefined) return undefined;
  return images.value.find((img) => img.id === currentImageId.value);
});

// 计算当前图片在列表中的索引，用于 UI 进度和边界判断
const currentImageIndex = computed(() => {
  if (currentImageId.value === undefined) return -1;
  return images.value.findIndex((img) => img.id === currentImageId.value);
});

function prevImage() {
  const index = currentImageIndex.value;
  if (index > 0) {
    const img = images.value[index - 1];
    if (img) {
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
    }
  }
}

function nextImage() {
  const index = currentImageIndex.value;
  if (index !== -1 && index < images.value.length - 1) {
    const img = images.value[index + 1];
    if (img) {
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
    }
  }
}

// 查看器打开时：左右方向键切换图片，Esc 关闭查看器
const isViewerOpen = computed(() => currentImageId.value !== undefined);

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
  "home",
  () => {
    if (images.value.length > 0) {
      const img = images.value[0];
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
    }
  },
  {
    allowInInputs: true,
    description: "切换到第一张图片",
    enabled: isViewerOpen,
    category: "图片浏览",
  },
);
useHotkey(
  "end",
  async () => {
    let pageCount = 0;
    while (hasNextPage.value) {
      if (pageCount > 0 && pageCount % 10 === 0) {
        const shouldContinue = confirm(
          `已经自动加载了 ${pageCount} 页图片，是否继续加载？`,
        );
        if (!shouldContinue) {
          break;
        }
      }
      const prevLength = images.value.length;
      await fetchMore();
      await nextTick();
      if (images.value.length <= prevLength) {
        break;
      }
      pageCount++;
    }
    if (images.value.length > 0) {
      const img = images.value[images.value.length - 1];
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
    }
  },
  {
    allowInInputs: true,
    description: "自动向后加载并切换到最后一张图片",
    enabled: isViewerOpen,
    category: "图片浏览",
  },
);
// #endregion

// #region 移动匹配图片模块
const moveImagesDialog = useModalDialog();
const imageViewerDialog = useModalFullscreen();
// #endregion

// #region URL Hash 状态持久化（文件名方式，便于跨筛选条件搜索）
const viewerHash = useLocationHash();

function openViewer(image: ImageFragment) {
  currentImageId.value = image.id;
  viewerHash.value = image.filename;
  imageViewerDialog.open();
}

function closeViewer() {
  imageViewerDialog.close();
}

function handleViewerAfterLeave() {
  currentImageId.value = undefined;
  viewerHash.value = "";
}

function tryOpenViewerByFilename(filename: string): boolean {
  console.log("try open", filename);
  const image = images.value.find(
    (img: ImageFragment) => img.filename === filename,
  );
  if (image) {
    openViewer(image);
    return true;
  }
  return false;
}

async function searchAndOpenViewer(filename: string) {
  if (tryOpenViewerByFilename(filename)) {
    return;
  }
  clearFilters();
  searchQuery.value = filename;
  await waitLoading();
  tryOpenViewerByFilename(filename);
}

async function waitLoading() {
  await nextTick();
  using stack = new DisposableStack();
  await new Promise<void>((resolve) => {
    stack.defer(
      watchEffect(() => {
        if (!loading.value) {
          resolve();
        }
      }),
    );
  });
}

onMounted(async () => {
  if (viewerHash.value) {
    await waitLoading();
    searchAndOpenViewer(viewerHash.value);
  }
});

// #endregion

// #region 响应 MemoList 打开图片查看器的事件
watchEffect((onCleanup) => {
  const unsubscribe = openImageViewerByFilename.subscribe((event) => {
    searchAndOpenViewer(event.detail.filename);
  });
  onCleanup(unsubscribe);
});
// #endregion
</script>

<style scoped>
/* 底部批量管理栏的升降过渡动画 */
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
