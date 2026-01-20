<template>
  <div
    :class="[
      'p-4 rounded-lg cursor-pointer transition-all border-2 bg-slate-600',
      selected
        ? ' border-secondary-500 shadow-lg shadow-secondary-500/30'
        : ' border-slate-500 hover:border-slate-400 hover:bg-slate-550',
    ]"
    @click="select"
  >
    <div class="flex items-start gap-3">
      <div
        v-if="localStats?.latestImage"
        class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
      >
        <img
          :src="localStats.latestImage.url256"
          :alt="directory.path"
          class="w-full h-full object-cover"
        />
      </div>
      <div
        v-else-if="loading"
        class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
      >
        <div class="w-full h-full animate-pulse bg-slate-600"></div>
      </div>
      <div class="flex-1 min-w-0">
        <h3 class="font-semibold text-lg mb-1 truncate flex items-center gap-2">
          {{ directory.root ? rootPath : directory.path }}
          <span
            v-if="
              localStats?.subdirectoryCount && localStats.subdirectoryCount > 0
            "
            class="px-2 py-0.5 text-xs bg-slate-700 rounded"
            >{{ localStats.subdirectoryCount }}子目录</span
          >
          <span
            v-if="isTargetMet"
            class="px-2 py-0.5 text-xs bg-emerald-600/80 text-emerald-100 rounded flex items-center gap-1"
          >
            <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                clip-rule="evenodd"
              />
            </svg>
            已达标
          </span>
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
          <div v-else class="font-mono text-slate-400">
            {{ props }}
          </div>
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
import useQuery from "../graphql/utils/useQuery";
import {
  GetDirectoryStatsDocument,
  GetMetaDocument,
} from "../graphql/generated";
import type {
  DirectoryFragment,
  RatingCountFragment,
} from "../graphql/generated";

interface Props {
  directory: DirectoryFragment;
  filterRating: readonly number[];
  targetKeep: number;
}

const props = defineProps<Props>();

const selectedId = defineModel<string>();

const loadingCount = ref(0);

const { data: metaData } = useQuery(GetMetaDocument);
const rootPath = computed(() => metaData.value?.meta?.rootPath || "");

const { data: statsData } = useQuery(GetDirectoryStatsDocument, {
  variables: () => ({ id: props.directory.id }),
  loadingCount,
});

const localStats = computed(() => statsData.value?.directory?.stats);
const loading = computed(() => loadingCount.value > 0);
const selected = computed(() => selectedId.value === props.directory.id);

const isTargetMet = computed(() => {
  const stats = localStats.value;
  if (!stats || !stats.ratingCounts || stats.imageCount === 0) {
    return false;
  }
  const matchedCount = stats.ratingCounts
    .filter((rc) => props.filterRating.includes(rc.rating))
    .reduce((sum, rc) => sum + rc.count, 0);
  return matchedCount <= props.targetKeep;
});

function sortedRatingCounts(
  ratingCounts: RatingCountFragment[],
): RatingCountFragment[] {
  return sortBy(ratingCounts, [(rc) => rc.rating]);
}

function select() {
  selectedId.value = props.directory.id;
}

defineExpose({
  stats: localStats,
});
</script>
