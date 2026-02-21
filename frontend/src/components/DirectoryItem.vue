<template>
  <div
    :class="[
      'p-4 rounded-lg cursor-pointer transition-all border-2 bg-primary-600',
      selected
        ? ' border-secondary-500 shadow-lg shadow-secondary-500/30'
        : ' border-primary-500 hover:border-primary-400 hover:bg-primary-550',
    ]"
    @click="select"
  >
    <DirectoryDisplay
      :directory="{ id: directory.id }"
      :filter-rating="filterRating"
      :loading="loading"
    >
      <template #badge>
        <div
          v-if="isTargetMet"
          class="flex-none px-2 py-0.5 text-xs bg-emerald-600/80 text-emerald-100 rounded flex items-center gap-1"
        >
          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
              clip-rule="evenodd"
            />
          </svg>
          已达标
        </div>
      </template>
      <template #title>
        <span class="flex-1 break-all">
          {{
            directory.root
              ? rootPath
              : selected
                ? directory.path
                : directory.path.split(/[\\/]/).pop()
          }}
        </span>
        <span
          v-if="stats?.subdirectoryCount && stats.subdirectoryCount > 0"
          class="flex-none px-2 py-0.5 text-xs bg-primary-700 rounded"
          >{{ stats.subdirectoryCount }}子目录</span
        >
      </template>
    </DirectoryDisplay>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import DirectoryDisplay from "./DirectoryDisplay.vue";
import useQuery from "../graphql/utils/useQuery";
import { MetaDocument } from "../graphql/generated";
import type { DirectoryFragment } from "../graphql/generated";
import useDirectoryStats from "@/composables/useDirectoryStats";

const { directory, filterRating, targetKeep, loading } = defineProps<{
  directory: DirectoryFragment;
  filterRating: readonly number[];
  targetKeep: number;
  loading?: boolean;
}>();

const { getCachedStats } = useDirectoryStats();

const selectedId = defineModel<string>();

const { data: metaData } = useQuery(MetaDocument);
const rootPath = computed(() => metaData.value?.meta?.rootPath || "");

const stats = computed(() => getCachedStats(directory.id));

const selected = computed(() => selectedId.value === directory.id);

const isTargetMet = computed(() => {
  const statsV = stats.value;
  if (!statsV || !statsV.ratingCounts || statsV.imageCount === 0) {
    return false;
  }
  const matchedCount = statsV.ratingCounts
    .filter((rc: { rating: number; count: number }) =>
      filterRating.includes(rc.rating),
    )
    .reduce(
      (sum: number, rc: { rating: number; count: number }) => sum + rc.count,
      0,
    );
  return matchedCount <= targetKeep;
});

function select() {
  selectedId.value = directory.id;
}
</script>
