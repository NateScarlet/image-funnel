<template>
  <div class="min-h-screen bg-slate-900 text-slate-100 p-4 md:p-8">
    <div class="max-w-4xl mx-auto">
      <header class="mb-8">
        <h1 class="text-3xl md:text-4xl font-bold text-center mb-2">
          ImageFunnel
          <span
            v-if="version"
            class="text-lg md:text-xl text-slate-400 font-normal ml-2"
          >
            {{ version }}
          </span>
        </h1>
        <p class="text-slate-400 text-center">AI生成图片筛选工具</p>
      </header>

      <div class="bg-slate-800 rounded-lg p-6 mb-6">
        <h2 class="text-xl font-semibold mb-4">创建新会话</h2>

        <div class="space-y-6">
          <div>
            <label class="block text-sm font-medium text-slate-300 mb-4">
              选择评分预设
            </label>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div
                v-for="preset in presets"
                :key="preset.id"
                :class="[
                  'p-4 rounded-lg cursor-pointer transition-all border-2',
                  selectedPresetId === preset.id
                    ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
                    : 'bg-slate-700 border-slate-600 hover:border-slate-500 hover:bg-slate-650',
                ]"
                @click="selectedPresetId = preset.id"
              >
                <h3 class="font-semibold text-lg mb-2">{{ preset.name }}</h3>
                <p class="text-sm opacity-80">{{ preset.description }}</p>
              </div>
            </div>
          </div>

          <div v-if="selectedPreset" class="bg-slate-700 rounded-lg p-4">
            <h3 class="font-medium mb-4">筛选条件</h3>
            <div class="mb-4">
              <label class="block text-sm text-slate-400 mb-2"
                >队列评分（多选）</label
              >
              <StarSelector v-model="filterRating" mode="multi" />
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-300 mb-2">
              保留目标数量
            </label>
            <input
              v-model.number="targetKeep"
              type="number"
              min="1"
              max="100"
              class="w-full px-4 py-2 bg-slate-700 border border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-white"
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-300 mb-4">
              选择目录
            </label>
            <div class="bg-slate-700 rounded-lg p-4">
              <div v-if="currentDirectory?.parentId" class="mb-4">
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

              <!-- 当前目录 -->
              <div
                v-if="currentDirectory && currentDirectory.imageCount > 0"
                class="mb-4 p-4 bg-slate-600 rounded-lg border-2 cursor-pointer transition-all"
                :class="[
                  selectedDirectoryId === currentDirectory.id
                    ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
                    : 'border-slate-500 hover:border-slate-400 hover:bg-slate-550',
                ]"
                @click="selectCurrentDirectory"
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
                      {{
                        currentDirectory.parentId
                          ? currentDirectory.path
                          : rootPath
                      }}
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
                v-else-if="
                  currentDirectory && currentDirectory.imageCount === 0
                "
                class="text-center text-slate-400 py-4 mb-4 bg-slate-600 rounded-lg"
              >
                当前目录下没有图片
              </div>

              <div v-if="loadingDirectories" class="space-y-4">
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

              <!-- 子目录列表 -->
              <div
                v-else
                class="max-h-[60vh] overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-4"
              >
                <div
                  v-for="dir in filteredDirectories"
                  :key="dir.id"
                  :class="[
                    'p-4 rounded-lg cursor-pointer transition-all border-2',
                    selectedDirectoryId === dir.id
                      ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
                      : 'bg-slate-600 border-slate-500 hover:border-slate-400 hover:bg-slate-550',
                  ]"
                  @click="selectedDirectoryId = dir.id"
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

          <button
            :disabled="!canCreate || creatingSession"
            class="w-full py-3 px-6 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
            @click="createSession"
          >
            <svg
              v-if="creatingSession"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <span>{{ creatingSession ? "创建中..." : "开始筛选" }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  GetDirectoriesDocument,
  GetMetaDocument,
} from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import StarSelector from "../components/StarSelector.vue";
import RatingIcon from "../components/RatingIcon.vue";
import { formatDate } from "../utils/date";
import { mdiLoading } from "@mdi/js";

const router = useRouter();
const { presets, getPreset } = usePresets();

const loadingCount = ref(0);
const creatingSession = ref(false);

const selectedPresetId = ref<string>("");
const targetKeep = ref<number>(10);
const filterRating = ref<number[]>([]);

const selectedDirectoryId = ref<string>("");

const { data: metaData } = useQuery(GetMetaDocument, {
  loadingCount,
});

const { data: directoriesData } = useQuery(GetDirectoriesDocument, {
  variables: () => ({ id: selectedDirectoryId.value }),
  loadingCount,
});

const loadingDirectories = computed(() => loadingCount.value > 0);

const currentDirectory = computed(() => directoriesData.value?.directory);
const directories = computed(() => currentDirectory.value?.directories || []);

const rootPath = computed(() => metaData.value?.meta?.rootPath || "");
const version = computed(() => metaData.value?.meta?.version || "");

const filteredDirectories = computed(() => {
  return directories.value.filter((dir) => {
    if (dir.subdirectoryCount > 0) {
      return true;
    }
    const matchedCount = getMatchedImageCount(dir);
    return matchedCount > targetKeep.value;
  });
});

const selectedPreset = computed(() => {
  return getPreset(selectedPresetId.value);
});

const canCreate = computed(() => {
  // 根目录是允许创建会话的
  return (
    filterRating.value.length > 0 &&
    targetKeep.value > 0 &&
    selectedDirectoryId.value !== ""
  );
});

function getMatchedImageCount(dir: {
  ratingCounts?: { rating: number; count: number }[];
}): number {
  if (!dir.ratingCounts || filterRating.value.length === 0) {
    return 0;
  }
  return dir.ratingCounts
    .filter((rc) => filterRating.value.includes(rc.rating))
    .reduce((sum, rc) => sum + rc.count, 0);
}

function sortedRatingCounts(
  ratingCounts: { rating: number; count: number }[],
): { rating: number; count: number }[] {
  return [...ratingCounts].sort((a, b) => a.rating - b.rating);
}

watch(
  () => selectedPreset.value,
  (preset) => {
    if (preset) {
      filterRating.value = [...preset.filter.rating];
      targetKeep.value = preset.targetKeep;
    }
  },
  { immediate: true },
);

watch(
  presets,
  (newPresets) => {
    if (newPresets.length > 0) {
      const lastPresetId = localStorage.getItem("lastSelectedPresetId");
      if (lastPresetId && newPresets.find((p) => p.id === lastPresetId)) {
        selectedPresetId.value = lastPresetId;
      } else {
        selectedPresetId.value = newPresets[0].id;
      }
    }
  },
  { immediate: true },
);

function getDirectoryName(path: string): string {
  const parts = path.split("/");
  return parts[parts.length - 1] || path;
}

function selectCurrentDirectory() {
  if (currentDirectory.value && currentDirectory.value.imageCount > 0) {
    selectedDirectoryId.value = currentDirectory.value.id || "";
  }
}

function goToParent() {
  const currentDir = currentDirectory.value;
  if (!currentDir || !currentDir.parentId) {
    selectedDirectoryId.value = "";
    return;
  }

  selectedDirectoryId.value = currentDir.parentId || "";
}

async function createSession() {
  creatingSession.value = true;

  try {
    const { data } = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          filter: {
            rating: filterRating.value,
          },
          targetKeep: targetKeep.value,
          directoryId: selectedDirectoryId.value,
        },
      },
    });

    localStorage.setItem("lastSelectedPresetId", selectedPresetId.value);
    if (data?.createSession) {
      router.push(`/session/${data.createSession.session.id}`);
    }
  } finally {
    creatingSession.value = false;
  }
}
</script>
