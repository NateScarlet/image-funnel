<template>
  <div
    :class="[
      'p-4 rounded-lg cursor-pointer transition-all border-2 bg-primary-600',
      selected
        ? 'border-secondary-500 shadow-lg shadow-secondary-500/30'
        : [
            filteredOut
              ? 'border-dashed border-yellow-600 hover:border-yellow-500'
              : 'border-primary-500 hover:border-primary-400',
            'hover:bg-primary-550',
          ],
    ]"
    @click="select"
  >
    <DirectoryDisplay :directory="{ id: directory.id }" :filter-rating="filterRating">
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
        <div
          v-if="filteredOut"
          class="flex-none px-2 py-0.5 text-xs bg-yellow-600/80 text-yellow-100 rounded flex items-center gap-1 mt-1"
        >
          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"
            />
          </svg>
          不匹配筛选
        </div>
      </template>
      <template #title>
        <span class="flex-1 break-all">
          {{
            directory.root
              ? rootAbsPath
              : selected
                ? directory.relPath
                : directory.relPath.split(/[\\/]/).pop()
          }}
        </span>
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
import { useDirectoryStats } from "@/composables/domain/useDirectoryBrowse";
import { evaluateDirectoryCompletion } from "@/utils/directoryCompletion";

const props = withDefaults(
  defineProps<{
    directory: DirectoryFragment;
    filterRating?: readonly number[];
    targetKeep?: number;
    isCompleted?: boolean;
    // 标记目录是否因为不匹配全局筛选条件（如已达标、小未评级等）而被过滤隐藏
    filteredOut?: boolean;
  }>(),
  {
    filterRating: () => [],
    targetKeep: 0,
    isCompleted: undefined,
    filteredOut: false,
  },
);

const { getCachedStats } = useDirectoryStats();

const selectedId = defineModel<string>();

const { data: metaData } = useQuery(MetaDocument);
const rootAbsPath = computed(() => metaData.value?.meta?.rootAbsPath || "");

const stats = computed(() => getCachedStats(props.directory.id));

const selected = computed(() => selectedId.value === props.directory.id);

// 判定目录是否已达标，优先取明确传入的 isCompleted
const isTargetMet = computed(() => {
  if (props.isCompleted !== undefined) {
    return props.isCompleted;
  }
  return evaluateDirectoryCompletion(props.directory, stats.value).isCompleted;
});

function select() {
  selectedId.value = props.directory.id;
}
</script>
