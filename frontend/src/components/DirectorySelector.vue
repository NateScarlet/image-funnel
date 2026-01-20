<template>
  <div>
    <label class="block text-sm font-medium text-slate-300 mb-4">
      选择目录
    </label>
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
      />

      <div
        v-if="filteredDirectories.length > 0"
        class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
      >
        <DirectoryItem
          v-for="dir in filteredDirectories"
          :key="dir.id"
          v-model="selectedId"
          :directory="dir"
          :filter-rating="filterRating"
        />
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
import type { DirectoryFragment } from "../graphql/generated";

interface Props {
  currentDirectory: DirectoryFragment | null | undefined;
  directories: DirectoryFragment[];
  loading: boolean;
  filterRating: number[];
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

const filteredDirectories = computed(() => {
  return sortBy(props.directories, [
    (dir) => {
      const dirItem = directoryItemRefs.value?.find(
        (item) => item.$props.directory.id === dir.id,
      );
      const stats = dirItem?.stats;
      const hasStats = stats !== undefined && stats !== null;
      return !hasStats;
    },
    (dir) => {
      const dirItem = directoryItemRefs.value?.find(
        (item) => item.$props.directory.id === dir.id,
      );
      const stats = dirItem?.stats;
      return stats?.latestImage?.modTime || "";
    },
  ]);
});

function goToParent() {
  emit("go-to-parent");
}
</script>
