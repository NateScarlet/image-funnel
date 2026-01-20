<template>
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

    <DirectorySelector
      v-model="selectedDirectoryId"
      :current-directory="currentDirectory"
      :directories="directories"
      :loading="loading"
      :filter-rating="filterRating"
      :target-keep="targetKeep"
      :root-path="rootPath"
      @go-to-parent="goToParent"
    />

    <button
      :disabled="!canCreate || creatingSession"
      class="w-full py-3 px-6 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
      @click="handleCreate"
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
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { mdiLoading } from "@mdi/js";
import { useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  GetDirectoriesDocument,
  GetMetaDocument,
} from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import StarSelector from "./StarSelector.vue";
import DirectorySelector from "./DirectorySelector.vue";

type Emits = (e: "created") => void;

const emit = defineEmits<Emits>();

const router = useRouter();
const { presets, getPreset, lastSelectedPresetId } = usePresets();

const loadingCount = ref(0);
const creatingSession = ref(false);
const selectedPresetId = ref(lastSelectedPresetId.value || "");
const selectedDirectoryId = ref<string>("");
const filterRating = ref<number[]>([]);
const targetKeep = ref<number>(10);

const { data: metaData } = useQuery(GetMetaDocument, {
  loadingCount,
});

const { data: directoriesData } = useQuery(GetDirectoriesDocument, {
  variables: () => ({ id: selectedDirectoryId.value }),
  loadingCount,
});

const loading = computed(() => loadingCount.value > 0);
const currentDirectory = computed(() => directoriesData.value?.directory);
const directories = computed(() => currentDirectory.value?.directories || []);
const rootPath = computed(() => metaData.value?.meta?.rootPath || "");

const selectedPreset = computed(() => {
  return getPreset(selectedPresetId.value || "");
});

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
    if (newPresets.length > 0 && !selectedPresetId.value) {
      selectedPresetId.value = newPresets[0].id;
    }
  },
  { immediate: true },
);

const canCreate = computed(() => {
  return (filterRating.value?.length || 0) > 0 && (targetKeep.value || 0) > 0;
});

function goToParent() {
  const currentDir = currentDirectory.value;
  if (!currentDir || !currentDir.parentId) {
    selectedDirectoryId.value = "";
    return;
  }

  selectedDirectoryId.value = currentDir.parentId || "";
}

async function handleCreate() {
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

    if (data?.createSession) {
      router.push(`/session/${data.createSession.session.id}`);
      emit("created");
    }
  } finally {
    creatingSession.value = false;
  }
}
</script>
