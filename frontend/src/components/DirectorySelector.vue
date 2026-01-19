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

      <div
        v-if="
          currentDirectory?.root ||
          (currentDirectory && currentDirectory.imageCount > 0)
        "
        class="mb-4 p-4 bg-slate-600 rounded-lg border-2 cursor-pointer transition-all"
        :class="[
          modelValue === currentDirectory.id
            ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
            : 'border-slate-500 hover:border-slate-400 hover:bg-slate-550',
        ]"
        @click="selectDirectory(currentDirectory.id)"
      >
        <div class="flex items-start gap-3">
          <div
            v-if="currentDirectory.latestImagePath"
            class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
          >
            <img
              v-if="currentDirectory.latestImageUrl"
              :src="currentDirectory.latestImageUrl"
              :alt="currentDirectory.path"
              class="w-full h-full object-cover"
            />
          </div>
          <div class="flex-1 min-w-0">
            <h3 class="font-semibold text-lg mb-1">
              {{ currentDirectory.root ? rootPath : currentDirectory.path }}
            </h3>
            <div class="text-xs text-slate-300 space-y-1">
              <div v-if="currentDirectory.latestImageModTime">
                {{ formatDate(currentDirectory.latestImageModTime) }}
              </div>
              <div
                v-if="
                  currentDirectory.ratingCounts &&
                  currentDirectory.ratingCounts.length > 0
                "
                class="flex flex-wrap gap-2 mt-2"
              >
                <div
                  v-for="rc in sortedRatingCounts(
                    currentDirectory.ratingCounts,
                  )"
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
          </div>
        </div>
      </div>

      <div
        v-else-if="currentDirectory && currentDirectory.imageCount === 0"
        class="text-center text-slate-400 py-4 mb-4 bg-slate-600 rounded-lg"
      >
        当前目录下没有图片
      </div>

      <div v-if="loading" class="space-y-4">
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

      <div
        v-else-if="filteredDirectories.length === 0"
        class="text-center text-slate-400 py-4"
      >
        当前目录下没有可用的子目录
      </div>

      <div
        v-else
        class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
      >
        <div
          v-for="dir in filteredDirectories"
          :key="dir.id"
          :class="[
            'p-4 rounded-lg cursor-pointer transition-all border-2',
            modelValue === dir.id
              ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
              : 'bg-slate-600 border-slate-500 hover:border-slate-400 hover:bg-slate-550',
          ]"
          @click="selectDirectory(dir.id)"
        >
          <div class="flex items-start gap-3">
            <div
              v-if="dir.latestImagePath"
              class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
            >
              <img
                v-if="dir.latestImageUrl"
                :src="dir.latestImageUrl"
                :alt="dir.path"
                class="w-full h-full object-cover"
              />
            </div>
            <div class="flex-1 min-w-0">
              <h3
                class="font-semibold text-lg mb-1 truncate flex items-center gap-2"
              >
                {{ getDirectoryName(dir.path) }}
                <span
                  v-if="dir.subdirectoryCount > 0"
                  class="px-2 py-0.5 text-xs bg-slate-700 rounded"
                  >{{ dir.subdirectoryCount }}子目录</span
                >
              </h3>
              <div class="text-xs text-slate-300 space-y-1">
                <div v-if="dir.latestImageModTime">
                  {{ formatDate(dir.latestImageModTime) }}
                </div>
                <div
                  v-if="dir.ratingCounts && dir.ratingCounts.length > 0"
                  class="flex flex-wrap gap-2 mt-2"
                >
                  <div
                    v-for="rc in sortedRatingCounts(dir.ratingCounts)"
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
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import RatingIcon from "./RatingIcon.vue";
import { formatDate } from "../utils/date";

interface RatingCount {
  rating: number;
  count: number;
}

interface Directory {
  id: string;
  parentId: string | null;
  path: string;
  root: boolean;
  imageCount: number;
  subdirectoryCount: number;
  latestImageModTime: string;
  latestImagePath: string | null;
  latestImageUrl: string | null;
  ratingCounts: RatingCount[];
}

interface Props {
  modelValue: string;
  currentDirectory: Directory | null | undefined;
  directories: Directory[];
  loading: boolean;
  filterRating: number[];
  targetKeep: number;
  rootPath: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "go-to-parent": [];
}>();

const filteredDirectories = computed(() => {
  return props.directories
    .filter((dir) => {
      if (dir.subdirectoryCount > 0) {
        return true;
      }
      const matchedCount = getMatchedImageCount(dir);
      return matchedCount > props.targetKeep;
    })
    .sort((a, b) => {
      // 按照最新图片修改日期从旧到新排序（最老的在前面）
      const timeA = a.latestImageModTime || "";
      const timeB = b.latestImageModTime || "";
      if (timeA < timeB) return -1;
      if (timeA > timeB) return 1;
      return 0;
    });
});

function getMatchedImageCount(dir: Directory): number {
  if (!dir.ratingCounts || props.filterRating.length === 0) {
    return 0;
  }
  return dir.ratingCounts
    .filter((rc) => props.filterRating.includes(rc.rating))
    .reduce((sum, rc) => sum + rc.count, 0);
}

function sortedRatingCounts(ratingCounts: RatingCount[]): RatingCount[] {
  return [...ratingCounts].sort((a, b) => a.rating - b.rating);
}

function getDirectoryName(path: string): string {
  const parts = path.split("/");
  return parts[parts.length - 1] || path;
}

function selectDirectory(id: string) {
  emit("update:modelValue", id);
}

function goToParent() {
  emit("go-to-parent");
}
</script>
