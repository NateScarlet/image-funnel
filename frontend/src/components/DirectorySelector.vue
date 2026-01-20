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
          currentDirectory &&
          (currentDirectory.root ||
            (currentDirectory.stats && currentDirectory.stats.imageCount > 0))
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
            v-if="currentDirectory.stats?.latestImage"
            class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
          >
            <img
              v-if="getImageUrl(currentDirectory.stats.latestImage)"
              :src="getImageUrl(currentDirectory.stats.latestImage)"
              :alt="currentDirectory.path"
              class="w-full h-full object-cover"
            />
          </div>
          <div class="flex-1 min-w-0">
            <h3 class="font-semibold text-lg mb-1">
              {{ currentDirectory.root ? rootPath : currentDirectory.path }}
            </h3>
            <div class="text-xs text-slate-300 space-y-1">
              <div v-if="currentDirectory.stats?.latestImage?.modTime">
                {{ formatDate(currentDirectory.stats.latestImage.modTime) }}
              </div>
              <div
                v-if="
                  currentDirectory.stats?.ratingCounts &&
                  currentDirectory.stats.ratingCounts.length > 0
                "
                class="flex flex-wrap gap-2 mt-2"
              >
                <div
                  v-for="rc in sortedRatingCounts(
                    currentDirectory.stats.ratingCounts,
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
        v-else-if="
          currentDirectory &&
          currentDirectory.stats &&
          currentDirectory.stats.imageCount === 0
        "
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
              v-if="dir.stats?.latestImage"
              class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
            >
              <img
                v-if="getImageUrl(dir.stats.latestImage)"
                :src="getImageUrl(dir.stats.latestImage)"
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
                  v-if="
                    dir.stats?.subdirectoryCount &&
                    dir.stats.subdirectoryCount > 0
                  "
                  class="px-2 py-0.5 text-xs bg-slate-700 rounded"
                  >{{ dir.stats.subdirectoryCount }}子目录</span
                >
              </h3>
              <div class="text-xs text-slate-300 space-y-1">
                <div v-if="dir.stats?.latestImage?.modTime">
                  {{ formatDate(dir.stats.latestImage.modTime) }}
                </div>
                <div
                  v-if="
                    dir.stats?.ratingCounts && dir.stats.ratingCounts.length > 0
                  "
                  class="flex flex-wrap gap-2 mt-2"
                >
                  <div
                    v-for="rc in sortedRatingCounts(dir.stats.ratingCounts)"
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

interface Image {
  id: string;
  filename: string;
  url: string;
  url256: string;
  url512: string;
  url1024: string;
  url2048: string;
  url4096: string;
  modTime: string;
  width: number;
  height: number;
  currentRating: number | null;
  xmpExists: boolean;
}

interface DirectoryStats {
  imageCount: number;
  subdirectoryCount: number;
  latestImage: Image | null;
  ratingCounts: RatingCount[];
}

interface Directory {
  id: string;
  parentId: string | null;
  path: string;
  root: boolean;
  stats?: DirectoryStats | null;
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
      if (dir.stats?.subdirectoryCount && dir.stats.subdirectoryCount > 0) {
        return true;
      }
      const matchedCount = getMatchedImageCount(dir);
      return matchedCount > props.targetKeep;
    })
    .sort((a, b) => {
      // 按照最新图片修改日期从旧到新排序（最老的在前面）
      const timeA = a.stats?.latestImage?.modTime || "";
      const timeB = b.stats?.latestImage?.modTime || "";
      if (timeA < timeB) return -1;
      if (timeA > timeB) return 1;
      return 0;
    });
});

function getMatchedImageCount(dir: Directory): number {
  if (!dir.stats?.ratingCounts || props.filterRating.length === 0) {
    return 0;
  }
  return dir.stats.ratingCounts
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

function getImageUrl(image: Image | null | undefined): string | undefined {
  if (!image) return undefined;
  // 使用 GraphQL 生成的 url256 字段
  return image.url256 || image.url;
}
</script>
