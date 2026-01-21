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
    <DirectoryDisplay
      :directory="{ id: directory.id }"
      :filter-rating="filterRating"
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
          class="flex-none px-2 py-0.5 text-xs bg-slate-700 rounded"
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
import { GetMetaDocument } from "../graphql/generated";
import type { DirectoryFragment } from "../graphql/generated";
import useDirectoryStats from "@/composables/useDirectoryStats";

interface Props {
  directory: DirectoryFragment;
  filterRating: readonly number[];
  targetKeep: number;
}

const props = defineProps<Props>();

const { getCachedStats } = useDirectoryStats();

const selectedId = defineModel<string>();

const { data: metaData } = useQuery(GetMetaDocument);
const rootPath = computed(() => metaData.value?.meta?.rootPath || "");

const stats = computed(() => getCachedStats(props.directory.id));

const selected = computed(() => selectedId.value === props.directory.id);

const isTargetMet = computed(() => {
  const statsV = stats.value;
  if (!statsV || !statsV.ratingCounts || statsV.imageCount === 0) {
    return false;
  }
  const matchedCount = statsV.ratingCounts
    .filter((rc: { rating: number; count: number }) =>
      props.filterRating.includes(rc.rating),
    )
    .reduce(
      (sum: number, rc: { rating: number; count: number }) => sum + rc.count,
      0,
    );
  return matchedCount <= props.targetKeep;
});

function select() {
  selectedId.value = props.directory.id;
}
</script>
