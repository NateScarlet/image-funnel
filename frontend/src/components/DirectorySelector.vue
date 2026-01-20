<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <label class="block text-sm font-medium text-slate-300"> 选择目录 </label>
      <template v-if="completedCount">
        <label class="flex items-center gap-2 cursor-pointer">
          <span class="text-sm text-slate-400"
            >显示已达标目录（{{ completedCount }}）</span
          >
          <div class="relative">
            <input
              v-model="showCompletedDirectories"
              type="checkbox"
              class="sr-only peer"
            />
            <div
              class="w-11 h-6 bg-slate-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-secondary-600"
            ></div>
          </div>
        </label>
      </template>
    </div>
    <div class="bg-slate-700 rounded-lg p-4">
      <div v-if="!currentDirectory?.root" class="mb-4">
        <button
          class="text-secondary-400 hover:text-secondary-300 text-sm flex items-center gap-1"
          @click="goToParent"
        >
          <svg
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
          返回上级
        </button>
      </div>

      <DirectoryItem
        v-if="currentDirectory"
        :key="currentDirectory.id"
        v-model="selectedId"
        class="mb-2"
        :directory="currentDirectory"
        :filter-rating="filterRating"
        :target-keep="targetKeep"
      />

      <div
        v-if="visibleItems.length > 0"
        class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
      >
        <template v-for="item in visibleItems" :key="item.key">
          <DirectoryItem
            ref="directoryItemRefs"
            v-model="selectedId"
            :directory="item.dir"
            :filter-rating="filterRating"
            :target-keep="targetKeep"
          />
        </template>
      </div>

      <div v-else-if="loading" class="space-y-4">
        <div class="bg-slate-700 rounded-lg p-4">
          <div class="animate-pulse">
            <div class="h-4 bg-slate-600 rounded mb-2 w-3/4"></div>
            <div class="h-3 bg-slate-600 rounded w-1/2"></div>
          </div>
        </div>
        <div class="bg-slate-700 rounded-lg p-4">
          <div class="animate-pulse">
            <div class="h-4 bg-slate-600 rounded mb-2 w-3/4"></div>
            <div class="h-3 bg-slate-600 rounded w-1/2"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useTemplateRef } from "vue";
import { sortBy } from "es-toolkit";
import DirectoryItem from "./DirectoryItem.vue";
import useStorage from "../composables/useStorage";
import type { DirectoryFragment } from "../graphql/generated";

interface Props {
  currentDirectory: DirectoryFragment | null | undefined;
  directories: DirectoryFragment[];
  loading: boolean;
  filterRating: readonly number[];
  targetKeep: number;
  rootPath: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  "go-to-parent": [];
}>();

const directoryItemRefs =
  useTemplateRef<InstanceType<typeof DirectoryItem>[]>("directoryItemRefs");

const selectedId = defineModel<string>();

const showCompletedDirectoriesStorage = useStorage<boolean>(
  localStorage,
  "showCompletedDirectories@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
  () => false,
);
const showCompletedDirectories = showCompletedDirectoriesStorage.model;

const statsByID = computed(
  () =>
    new Map(
      directoryItemRefs.value?.map(
        (i) => [i.stats?.directory?.id, i.stats?.directory?.stats] as const,
      ) || [],
    ),
);
const items = computed(() => {
  return props.directories.map((dir) => {
    const stats = statsByID.value.get(dir.id);
    let keepCount = undefined;
    if (stats && stats.ratingCounts) {
      keepCount = stats.ratingCounts.reduce(
        (sum, rc) =>
          sum + (props.filterRating.includes(rc.rating) ? rc.count : 0),
        0,
      );
    }
    return {
      key: dir.id,
      dir,
      stats,
      keepCount,
    };
  });
});

const completedCount = computed(() => {
  return items.value.filter(isCompleted).length;
});

function isCompleted(item: { keepCount?: number }) {
  return item.keepCount !== undefined && item.keepCount <= props.targetKeep;
}

const visibleItems = computed(() => {
  let dirs = items.value;

  if (!showCompletedDirectories.value) {
    dirs = dirs.filter((item) => !isCompleted(item));
  }

  return sortBy(dirs, [
    (item) => {
      return !item.stats;
    },
    (item) => item.stats?.imageCount === 0,
    (item) => item.stats?.latestImage?.modTime || "",
  ]);
});

function goToParent() {
  emit("go-to-parent");
}
</script>
