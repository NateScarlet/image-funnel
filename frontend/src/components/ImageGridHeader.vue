<template>
  <div
    class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-primary-700/50 pb-3"
  >
    <h2
      class="text-base font-bold text-primary-200 tracking-wider flex flex-wrap items-center gap-2 select-none"
    >
      <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
        <path :d="mdiImage" fill="currentColor" />
      </svg>
      <span>图片列表</span>

      <!-- 评星统计 -->
      <div
        v-if="stats && stats.ratingCounts.length > 0"
        class="flex items-center gap-2 ml-2 text-xs font-normal"
      >
        <button
          v-for="rc in sortedRatingCounts"
          :key="rc.rating"
          class="flex items-center gap-1 px-2 py-1 rounded bg-primary-700/50 hover:bg-primary-600/80 transition-colors cursor-pointer select-none"
          :title="
            filterRating.includes(rc.rating)
              ? `取消筛选 ${rc.rating === 0 ? '无评分' : rc.rating + '星'}`
              : `筛选 ${rc.rating === 0 ? '无评分' : rc.rating + '星'}`
          "
          @click="toggleRatingFilter(rc.rating)"
        >
          <RatingIcon :rating="rc.rating" :filled="filterRating.includes(rc.rating)" />
          <span class="text-xs">{{ rc.count }}</span>
        </button>
      </div>

      <!-- 删除低星级图片按钮 -->
      <button
        v-if="deleteUnmatchedInfo"
        :disabled="isDeletingUnmatched"
        class="px-3 h-8 text-xs font-normal rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 cursor-pointer select-none hover:text-white"
        :class="isDeletingUnmatched ? 'opacity-50 cursor-not-allowed' : ''"
        :title="`删除该目录下所有评分在 ${deleteUnmatchedInfo.maxUnmatched} 星及以下的图片`"
        @click="handleDeleteUnmatched"
      >
        <svg
          v-if="isDeletingUnmatched"
          class="w-4 h-4 animate-spin"
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
        <svg v-else class="w-4 h-4" viewBox="0 0 24 24">
          <path :d="mdiDelete" fill="currentColor" />
        </svg>
        <span>删除</span>
        <RatingIcon :rating="deleteUnmatchedInfo.maxUnmatched" filled class="w-4 h-4" />
        <span>以下的图片</span>
      </button>

      <!-- 加载中即使有缓存数据也显示旋转加载提示 -->
      <svg
        v-if="browse.loading"
        class="w-4 h-4 animate-spin text-secondary-500"
        viewBox="0 0 24 24"
        fill="none"
        title="正在加载最新数据…"
      >
        <path
          :d="mdiLoading"
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          stroke-linecap="round"
        />
      </svg>
    </h2>

    <div class="flex flex-wrap items-center gap-3">
      <!-- 移动匹配图片按钮 -->
      <button
        v-if="browse.imageCount > 0 && !bulkMode.isBulkMode"
        class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 bg-primary-800 hover:bg-primary-700 border-primary-700 text-primary-200 cursor-pointer select-none"
        title="将当前过滤匹配的图片移动到新目录"
        @click="$emit('openMoveDialog')"
      >
        <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
          <path :d="mdiFolderMove" fill="currentColor" />
        </svg>
        <span>移动匹配图片</span>
      </button>

      <!-- 批量管理按钮 -->
      <button
        v-if="browse.imageCount > 0"
        class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 cursor-pointer select-none"
        :class="[
          bulkMode.isBulkMode
            ? 'bg-secondary-600 hover:bg-secondary-700 border-secondary-500 text-white shadow-[0_0_10px_rgba(235,94,85,0.3)] font-semibold'
            : 'bg-primary-800 hover:bg-primary-700 border-primary-700 text-primary-200',
        ]"
        :title="bulkMode.isBulkMode ? '退出批量管理模式' : '进入批量管理模式，对多张图片执行操作'"
        @click="bulkMode.toggle()"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24">
          <path :d="mdiCheckboxMultipleMarkedOutline" fill="currentColor" />
        </svg>
        <span>{{ bulkMode.isBulkMode ? "退出批量" : "批量管理" }}</span>
      </button>

      <!-- 当用户激活了任何过滤器时，在最左侧显示一键清除筛选按钮 -->
      <button
        v-if="hasActiveFilters"
        class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border-red-900/50 text-red-300 cursor-pointer"
        @click="clearFilters"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24">
          <path :d="mdiFilterOff" fill="currentColor" />
        </svg>
        <span>清除筛选</span>
      </button>

      <!-- 重新筛选本地已修改且不再满足筛选条件的图片 -->
      <button
        v-if="browse.outOfFilterImageIdsSize > 0"
        class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-2 bg-amber-950/40 hover:bg-amber-900/40 border-amber-900/50 text-amber-300 cursor-pointer animate-pulse"
        title="隐藏不再符合当前过滤条件的图片"
        @click="browse.applyLocalFilter()"
      >
        <svg class="w-4 h-4 text-amber-400" viewBox="0 0 24 24">
          <path :d="mdiRefresh" fill="currentColor" />
        </svg>
        <span>重新筛选 ({{ browse.outOfFilterImageIdsSize }}张已改)</span>
      </button>

      <!-- 搜索输入框 -->
      <div class="relative min-w-36 max-w-60 flex-1 sm:flex-none">
        <input
          v-model="searchQuery"
          name="search"
          autocomplete="off"
          type="text"
          placeholder="搜索文件名…"
          class="w-full pl-8 pr-8 h-8 bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-xs text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all"
        />
        <svg
          class="w-4 h-4 text-primary-400 absolute left-2 top-1/2 -translate-y-1/2 pointer-events-none"
          viewBox="0 0 24 24"
        >
          <path :d="mdiMagnify" fill="currentColor" />
        </svg>
        <button
          v-if="searchQuery"
          class="absolute right-2 top-1/2 -translate-y-1/2 text-primary-400 hover:text-primary-200 transition-colors p-0.5 rounded-full hover:bg-primary-700/50 cursor-pointer"
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
        class="flex items-center gap-2 bg-primary-800 border border-primary-700 px-3 h-8 rounded-lg overflow-x-auto"
      >
        <span class="text-xs text-primary-400 select-none">标签:</span>
        <div class="flex items-center gap-1">
          <button
            v-for="(colorHex, colorName) in PRESET_COLORS"
            :key="colorName"
            class="w-3 h-3 rounded-full transition-all border border-white/20 relative"
            :style="{
              backgroundColor: colorHex,
              borderColor: filterLabels.includes(colorName) ? 'white' : undefined,
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
              class="absolute inset-px rounded-full border border-black/30"
            ></span>
          </button>
        </div>
      </div>
    </div>
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
  mdiFolderMove,
  mdiCheckboxMultipleMarkedOutline,
  mdiRefresh,
  mdiDelete,
} from "@mdi/js";
import { PRESET_COLORS } from "@/composables/useImageLabel";
import RatingIcon from "./RatingIcon.vue";
import RatingFilter from "./RatingFilter.vue";
import { useDirectoryState } from "@/composables/useDirectoryState";
import { useDirectoryStats } from "@/composables/domain/useDirectoryBrowse";
import useTrash from "@/composables/domain/useTrash";

const props = defineProps<{
  directoryId: string;
  bulkMode: {
    isBulkMode: boolean;
    toggle: () => void;
  };
  browse: {
    loading: boolean;
    imageCount: number;
    outOfFilterImageIdsSize: number;
    applyLocalFilter: () => void;
  };
}>();

defineEmits<{
  openMoveDialog: [];
}>();

// #region 筛选状态（模块级单例，与父组件共享）
const { filterRating, filterLabels, searchQuery, hasActiveFilters, clearFilters } =
  useDirectoryState(() => props.directoryId);

function toggleRatingFilter(rating: number) {
  const index = filterRating.value.indexOf(rating);
  if (index >= 0) {
    filterRating.value = filterRating.value.filter((r) => r !== rating);
  } else {
    const previousRating = filterRating.value;
    clearFilters();
    filterRating.value = [...previousRating, rating].toSorted((a, b) => a - b);
  }
}

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

// #region 统计信息
const { useStats } = useDirectoryStats();
const statsData = useStats(() => props.directoryId);
const stats = computed(() => {
  const node = statsData.value?.node;
  return node?.__typename === "Directory" ? node.stats : undefined;
});

const sortedRatingCounts = computed(() => {
  if (!stats.value?.ratingCounts) return [];
  return [...stats.value.ratingCounts].toSorted((a, b) => a.rating - b.rating);
});
// #endregion

// #region 删除低星级图片
const { trashImages } = useTrash();
const deletingUnmatchedBuffer = ref({
  directoryId: props.directoryId,
  value: false,
});
const isDeletingUnmatched = computed({
  get: () =>
    deletingUnmatchedBuffer.value.directoryId === props.directoryId
      ? deletingUnmatchedBuffer.value.value
      : false,
  set: (val) => {
    deletingUnmatchedBuffer.value = {
      directoryId: props.directoryId,
      value: val,
    };
  },
});

const deleteUnmatchedInfo = computed(() => {
  if (!stats.value?.ratingCounts || filterRating.value.length === 0) return null;

  const existingRatingsWithCount = stats.value.ratingCounts.filter((rc) => rc.count > 0);
  if (existingRatingsWithCount.length === 0) return null;

  const existingRatings = existingRatingsWithCount.map((rc) => rc.rating);
  const matchedRatings = existingRatings.filter((r) => filterRating.value.includes(r));
  const unmatchedRatings = existingRatings.filter((r) => !filterRating.value.includes(r));

  if (matchedRatings.length === 0 || unmatchedRatings.length === 0) return null;

  const minMatched = Math.min(...matchedRatings);
  const maxUnmatched = Math.max(...unmatchedRatings);

  if (maxUnmatched < minMatched) {
    const totalCount = existingRatingsWithCount
      .filter((rc) => unmatchedRatings.includes(rc.rating))
      .reduce((sum, rc) => sum + rc.count, 0);

    return {
      maxUnmatched,
      totalCount,
    };
  }

  return null;
});

async function handleDeleteUnmatched() {
  const info = deleteUnmatchedInfo.value;
  if (!info || isDeletingUnmatched.value) return;

  isDeletingUnmatched.value = true;
  try {
    const ratingsToDelete = Array.from({ length: info.maxUnmatched + 1 }, (_, i) => i);

    await trashImages(props.directoryId, {
      rating: ratingsToDelete,
    });
  } finally {
    isDeletingUnmatched.value = false;
  }
}
// #endregion
</script>
