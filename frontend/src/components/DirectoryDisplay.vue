<template>
  <div class="flex items-start gap-3">
    <div class="flex-shrink-0 rounded overflow-hidden relative">
      <img
        v-if="localStats?.latestImage"
        :src="localStats.latestImage.url256"
        :alt="directoryPath"
        class="w-20 bg-slate-700 object-cover"
      />
      <div
        v-else-if="loading"
        class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
      >
        <div class="w-full h-full animate-pulse bg-slate-600"></div>
      </div>
      <slot name="badge"></slot>
    </div>
    <div class="flex-1 min-w-0">
      <h3 class="font-semibold text-lg mb-1">
        <slot name="title">
          <span class="flex-1 break-all">{{ directoryPath }}</span>
        </slot>
      </h3>
      <div class="text-xs text-slate-300 space-y-1">
        <div v-if="localStats">
          <div v-if="localStats.latestImage?.modTime">
            {{ formatDate(localStats.latestImage.modTime) }}
          </div>
          <div
            v-if="localStats.ratingCounts.length > 0"
            class="flex flex-wrap gap-2 mt-2"
          >
            <div
              v-for="rc in sortedRatingCounts(localStats.ratingCounts)"
              :key="rc.rating"
              class="flex items-center gap-1 px-2 py-1 rounded bg-slate-700/50"
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
          <div class="h-3 bg-slate-600 rounded w-3/4 animate-pulse"></div>
          <div class="h-3 bg-slate-600 rounded w-1/2 animate-pulse"></div>
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

interface Props {
  directory: Directory;
  filterRating?: readonly number[];
}

const props = withDefaults(defineProps<Props>(), {
  filterRating: () => [],
});

const loadingCount = ref(0);

// 使用 useStats 自动查询和缓存
const { useStats } = useDirectoryStats();
const data = useStats(props.directory.id, loadingCount);

const localStats = computed(() => data.value?.directory?.stats);
const loading = computed(() => loadingCount.value > 0);
const directoryPath = computed(() => data.value?.directory?.path ?? "");

function sortedRatingCounts(
  ratingCounts: RatingCountFragment[],
): RatingCountFragment[] {
  return sortBy(ratingCounts, [(rc) => rc.rating]);
}
</script>
