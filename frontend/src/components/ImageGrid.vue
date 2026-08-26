<template>
  <div class="space-y-6">
    <!-- 图片网格展示区 -->
    <section
      class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
    >
      <!-- 标题栏与筛选器 -->
      <ImageGridHeader
        :directory-id="directoryId"
        :bulk-mode="{ isBulkMode, toggle: toggleBulkMode }"
        :browse="{
          loading,
          imageCount: images.length,
          outOfFilterImageIdsSize: outOfFilterImageIds.size,
          applyLocalFilter,
        }"
        @open-move-dialog="moveImagesDialog.open()"
      />

      <!-- 滚动容器：包裹列表、空状态、骨架图与加载更多按钮 -->
      <div ref="containerRef" class="max-h-[60vh] overflow-y-auto pr-1 space-y-4">
        <!-- 骨架图加载指示，避免布局抖动 -->
        <div
          v-if="loading && images.length === 0"
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 animate-pulse p-4"
        >
          <div v-for="n in 16" :key="n" class="aspect-square bg-primary-800/50 rounded-xl"></div>
        </div>

        <!-- 无图片空状态 -->
        <div
          v-else-if="images.length === 0"
          class="flex flex-col items-center justify-center py-20 gap-2 select-none"
        >
          <!-- 有图片被本地筛选隐藏时，提示数量并允许一键恢复 -->
          <template v-if="hiddenImageCount > 0">
            <svg class="w-12 h-12 text-primary-500" viewBox="0 0 24 24">
              <path :d="mdiEyeOff" fill="currentColor" />
            </svg>
            <span class="text-sm text-primary-400">
              {{ hiddenImageCount }} 张不符合当前筛选的图片已被隐藏
            </span>
            <button
              class="mt-2 px-4 py-2 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-secondary-500/50 rounded-lg text-xs text-primary-200 hover:text-white transition-colors cursor-pointer"
              @click="clearLocalFilter"
            >
              显示这些图片
            </button>
          </template>
          <template v-else>
            <svg class="w-12 h-12 stroke-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 00-1.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"
              />
            </svg>
            <span class="text-sm text-primary-500">该目录或过滤条件下未找到任何图片</span>
          </template>
        </div>

        <!-- 网格列表 -->
        <div
          v-else
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 p-4"
        >
          <ImageGridCard
            v-for="img in images"
            :key="img.id"
            :img="img"
            :is-bulk-mode="isBulkMode"
            :is-selected="isSelected(img.id)"
            :is-out-of-filter="outOfFilterImageIds.has(img.id)"
            @click="handleImageClick"
          />
        </div>

        <!-- 懒加载过渡区与加载更多按钮 -->
        <div v-if="hasNextPage" class="flex justify-center pt-2">
          <button
            :disabled="loading"
            class="px-6 py-2 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded-xl text-sm font-semibold transition-all flex items-center gap-2 text-primary-200 hover:text-white"
            @click="fetchMore"
          >
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
            <span>{{ loading ? "正在加载…" : "加载更多图片" }}</span>
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
          v-if="currentImageIndex >= 0 && (currentImageIndex < images.length - 1 || hasNextPage)"
          class="absolute right-4 top-1/2 -translate-y-1/2 z-60 p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="下一张图片 (ArrowRight)"
          @click="nextImage"
        >
          <svg
            v-if="currentImageIndex === images.length - 1 && loading"
            class="w-8 h-8 animate-spin text-secondary-500"
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
          <svg v-else class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronRight" fill="currentColor" />
          </svg>
        </button>

        <!-- 图像查看器组件 -->
        <ImageViewer
          :image="currentImage"
          :preload-images="preloadImages"
          class="w-full h-full flex-1"
          @request-next="nextImage"
        >
          <template #info>
            <span class="truncate max-w-72 font-semibold" :title="currentImage.filename">
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
    <moveImagesDialog.component container-class="sm:max-w-md p-6" overflow-visible>
      <MoveImagesForm
        :directory-id="directoryId"
        :filter-by="selectedFilterBy || imagesVariables.filterBy || { id: [] }"
        :match-count="moveImagesMatchCount"
        :is-approximate="!isBulkMode && hasNextPage"
        @close="handleMoveClose"
      />
    </moveImagesDialog.component>

    <!-- 批量操作底栏 -->
    <ImageGridBatchBar :bulk-ops="bulkOps" @move="moveImagesDialog.open()" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, useTemplateRef } from "vue";
import { mdiLoading, mdiChevronLeft, mdiChevronRight, mdiClose, mdiEyeOff } from "@mdi/js";
import useInfiniteScroll from "@/composables/useInfiniteScroll";
import useBulkOperations from "@/composables/useBulkOperations";
import ImageViewer from "./ImageViewer.vue";
import { useHotkeys } from "@/composables/useHotkeys";
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
import { useClipboard } from "@/composables/useClipboard";
import useTrash from "@/composables/domain/useTrash";
import { PRESET_COLORS, COLOR_NAMES_CN } from "@/composables/useImageLabel";
import useImageViewer from "@/composables/useImageViewer";
import ImageGridHeader from "./ImageGridHeader.vue";
import ImageGridCard from "./ImageGridCard.vue";
import ImageGridBatchBar from "./ImageGridBatchBar.vue";

// #region 属性
const props = defineProps<{
  directoryId: string;
}>();
// #endregion

// #region 筛选状态（模块级单例，与 ImageGridHeader 共享）
const { imageFilters, searchQuery, clearFilters } = useDirectoryState(() => props.directoryId);
// #endregion

// #region 图片加载
const loadingCount = ref(1);
onMounted(() => {
  nextTick(() => {
    loadingCount.value -= 1;
  });
});

const imagesVariables = computed<BrowseImagesQueryVariables>(() => {
  const filterBy: ImageFiltersInput = imageFilters.value ?? {};
  return {
    id: props.directoryId,
    filterBy,
    first: 20,
  };
});

const loading = computed(() => loadingCount.value > 0);

const {
  images,
  hasNextPage,
  fetchMore,
  outOfFilterImageIds,
  hiddenImageCount,
  applyLocalFilter,
  clearLocalFilter,
} = useBrowseImages(imagesVariables, {
  loadingCount,
});

const containerRef = useTemplateRef<HTMLElement>("containerRef");

useInfiniteScroll(containerRef, async () => {
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});
// #endregion

// #region 批量操作
const {
  isBulkMode,
  selectedFilterBy,
  selectedImages,
  isSelected,
  selectedCountText,
  isUpdating,
  isAllMatchingSelected,
  toggleBulkMode,
  toggleSelectImage,
  selectRange,
  selectAll,
  deselectAll,
  invertSelection,
  bulkSetRating,
  bulkSetLabel,
} = useBulkOperations(
  images,
  () => props.directoryId,
  () => imagesVariables.value.filterBy || {},
  () => hasNextPage.value || false,
);

const anchorImageId = ref<string | null>(null);

const moveImagesMatchCount = computed(() => {
  if (isBulkMode.value) {
    return selectedImages.value.length;
  }
  return images.value.length;
});
// #endregion

// #region 复制
const copyLoadingCount = ref(0);
const { copyFiles } = useClipboard({
  loadingCount: copyLoadingCount,
});

const isCopying = computed(() => copyLoadingCount.value > 0);

async function copySelectedImages() {
  if (isCopying.value || !selectedFilterBy.value) return;

  if (isAllMatchingSelected.value && hasNextPage.value) {
    let fetchCount = 0;

    let limit = 2;
    while (hasNextPage.value) {
      await fetchMore();
      fetchCount++;

      if (fetchCount >= limit) {
        const ok = confirm(
          `已自动加载 ${fetchCount} 页，是否继续加载更多图片并复制？\n\n点击【确定】继续加载；\n点击【取消】仅复制当前已加载的 ${images.value.length} 张图片。`,
        );
        if (!ok) {
          break;
        }
        limit = Math.max(8, limit * 2);
      }
    }
  }

  const paths = selectedImages.value.map((img) => img.relPath);
  if (paths.length === 0) return;
  await copyFiles(paths);
}
// #endregion

// #region 移动 & 关闭
const moveImagesDialog = useModalDialog();

function handleMoveClose() {
  moveImagesDialog.close();
  if (isBulkMode.value) {
    deselectAll();
  }
}
// #endregion

// #region 图片点击
function handleImageClick(img: ImageFragment, event?: MouseEvent) {
  const isCtrlPressed = event ? event.ctrlKey || event.metaKey : false;
  const isShiftPressed = event ? event.shiftKey : false;

  if ((isCtrlPressed || isShiftPressed) && !isBulkMode.value) {
    deselectAll();
    isBulkMode.value = true;
  }

  if (isShiftPressed && isBulkMode.value && anchorImageId.value !== null) {
    selectRange(anchorImageId.value, img.id);
    return;
  }

  if (isCtrlPressed || isBulkMode.value) {
    toggleSelectImage(img.id);
    anchorImageId.value = img.id;
    return;
  }

  openViewer(img);
  anchorImageId.value = null;
}
// #endregion

// #region 查看器
const {
  imageViewerDialog,
  currentImageId,
  currentImage,
  currentImageIndex,
  preloadImages,
  openViewer,
  closeViewer,
  handleViewerAfterLeave,
  prevImage,
  nextImage,
} = useImageViewer({
  images,
  hasNextPage,
  loading,
  fetchMore: async () => {
    await fetchMore();
  },
  clearFilters,
  searchQuery,
});
// #endregion

// #region 批量操作热键
useHotkeys(
  {
    "ctrl+a": (e) => {
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      if (!isBulkMode.value) {
        isBulkMode.value = true;
      }
      selectAll();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "全选所有图片",
    category: "批量操作",
  },
);

useHotkeys(
  {
    "ctrl+c": (e) => {
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      copySelectedImages();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制选中的图片文件",
    enabled: computed(() => isBulkMode.value && !!selectedFilterBy.value),
    category: "批量操作",
  },
);

useHotkeys(
  {
    escape: () => {
      isBulkMode.value = false;
      anchorImageId.value = null;
    },
  },
  {
    allowInInputs: false,
    description: "退出批量模式",
    enabled: isBulkMode,
    category: "批量操作",
  },
);

// 批量模式下删除选中图片（Delete 键）
const { trashImages } = useTrash();
const isBulkDeleting = ref(false);

useHotkeys(
  {
    delete: async () => {
      if (!selectedFilterBy.value || isBulkDeleting.value) return;

      isBulkDeleting.value = true;
      try {
        await trashImages(props.directoryId, selectedFilterBy.value, "Delete键删除");
        deselectAll();
      } finally {
        isBulkDeleting.value = false;
      }
    },
  },
  {
    allowInInputs: false,
    description: "删除选中的图片及其配套文件",
    enabled: computed(
      () =>
        isBulkMode.value &&
        !!selectedFilterBy.value &&
        !isBulkDeleting.value &&
        !imageViewerDialog.visible.value &&
        !moveImagesDialog.visible.value,
    ),
    category: "批量操作",
  },
);

// 批量操作可用性检查
const isBulkActionEnabled = computed(() => {
  return (
    isBulkMode.value &&
    !!selectedFilterBy.value &&
    !isBulkDeleting.value &&
    !imageViewerDialog.visible.value &&
    !moveImagesDialog.visible.value
  );
});

// 绑定快捷键 Ctrl+0~5 以及小键盘 0~5 用于批量修改评分
for (let r = 0; r <= 5; r++) {
  useHotkeys(
    [
      {
        keys: [`ctrl+digit${r}`, `numpad${r}`],
        handler: (e) => {
          e.preventDefault();
          e.stopPropagation();
          bulkSetRating(r);
        },
      },
    ],
    {
      description: `批量设置评分为 ${r} 星`,
      category: "批量操作",
      enabled: isBulkActionEnabled,
    },
  );
}

// 批量设置颜色标签快捷键 Ctrl+Shift+1~9，以及清除标签 Ctrl+Shift+0
const colorNames = Object.keys(PRESET_COLORS);
for (let i = 0; i < 9; i++) {
  const colorName = colorNames[i];
  const colorCn = COLOR_NAMES_CN[colorName] || colorName;
  useHotkeys(
    {
      [`ctrl+shift+${i + 1}`]: (e) => {
        e.preventDefault();
        e.stopPropagation();
        bulkSetLabel(colorName);
      },
    },
    {
      description: `批量设置标签为 ${colorCn}`,
      category: "批量操作",
      enabled: isBulkActionEnabled,
    },
  );
}

useHotkeys(
  {
    "ctrl+shift+0": (e) => {
      e.preventDefault();
      e.stopPropagation();
      bulkSetLabel("");
    },
  },
  {
    description: "批量清除图片标签",
    category: "批量操作",
    enabled: isBulkActionEnabled,
  },
);
// #endregion

// #region bulkOps 打包对象（传递给 ImageGridBatchBar）
const bulkOps = {
  isBulkMode,
  selectedCountText,
  isAllMatchingSelected,
  selectedFilterBy,
  isUpdating,
  selectedImages,
  selectAll: selectAll as () => void,
  deselectAll: deselectAll as () => void,
  invertSelection: invertSelection as () => void,
  toggle: toggleBulkMode as () => void,
  setRating: bulkSetRating,
  setLabel: bulkSetLabel,
  copySelectedImages,
  isCopying,
};
// #endregion
</script>
