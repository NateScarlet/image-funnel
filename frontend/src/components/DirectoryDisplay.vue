<template>
  <div class="flex items-start gap-3">
    <div class="shrink-0 rounded overflow-hidden relative">
      <img
        v-if="stats?.latestImage"
        :src="stats.latestImage.url256"
        :alt="directoryPath"
        loading="lazy"
        class="w-20 bg-primary-700 object-cover"
      />
      <div
        v-else-if="loading"
        class="w-20 h-20 shrink-0 bg-primary-700 rounded overflow-hidden"
      >
        <div class="w-full h-full animate-pulse bg-primary-600"></div>
      </div>
      <!-- 加载提示：当显示缓存数据的同时正在后台更新时显示 -->
      <div
        v-if="loading && stats"
        class="absolute right-0 top-0 z-10 rounded-bl bg-black/30 p-1 text-white backdrop-blur-[1px]"
        title="正在刷新..."
      >
        <svg
          class="h-3 w-3 animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          ></circle>
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          ></path>
        </svg>
      </div>
      <slot name="badge"></slot>
    </div>
    <div class="flex-1 min-w-0">
      <h3 class="font-semibold text-lg mb-1">
        <slot name="title">
          <span class="flex-1 break-all">{{ directoryPath }}</span>
        </slot>
      </h3>
      <div class="text-xs text-primary-300 space-y-1">
        <div v-if="stats">
          <div v-if="stats.latestImage?.modTime">
            {{ formatDate(stats.latestImage.modTime) }}
          </div>
          <div
            v-if="stats.ratingCounts.length > 0"
            class="flex flex-wrap gap-2 mt-2"
          >
            <div
              v-for="rc in sortedRatingCounts(stats.ratingCounts)"
              :key="rc.rating"
              class="flex items-center gap-1 px-2 py-1 rounded bg-primary-700/50"
            >
              <RatingIcon
                :rating="rc.rating"
                :filled="filterRating.includes(rc.rating)"
              />
              <span class="text-xs">{{ rc.count }}</span>
            </div>
          </div>
        </div>
        <div v-else-if="loading" class="space-y-2">
          <div class="h-3 bg-primary-600 rounded w-3/4 animate-pulse"></div>
          <div class="h-3 bg-primary-600 rounded w-1/2 animate-pulse"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { sortBy } from "es-toolkit";
import RatingIcon from "./RatingIcon.vue";
import { formatDate } from "../utils/date";
import type { RatingCountFragment } from "../graphql/generated";
import useDirectoryStats from "../composables/useDirectoryStats";

interface Directory {
  id: string;
}

const {
  directory,
  filterRating = [],
  loading: externalLoading,
} = defineProps<{
  directory: Directory;
  filterRating?: readonly number[];
  loading?: boolean;
}>();

const loadingCount = ref(0);

// 使用 useStats 自动查询和缓存
const { useStats } = useDirectoryStats();
const data = useStats(() => directory.id, loadingCount);

const directoryData = computed(() => {
  const node = data.value?.node;
  return node?.__typename === "Directory" ? node : undefined;
});
const stats = computed(() => directoryData.value?.stats);
const loading = computed(() => loadingCount.value > 0 || !!externalLoading);
const directoryPath = computed(() => directoryData.value?.path ?? "");

function sortedRatingCounts(
  ratingCounts: RatingCountFragment[],
): RatingCountFragment[] {
  return sortBy(ratingCounts, [(rc) => rc.rating]);
}
</script>
