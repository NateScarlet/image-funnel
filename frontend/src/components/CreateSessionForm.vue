<template>
  <div class="space-y-6">
    <div>
      <label class="block text-sm font-medium text-primary-300 mb-4">
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
              : 'bg-primary-700 border-primary-600 hover:border-primary-500 hover:bg-primary-650',
          ]"
          @click="selectedPresetId = preset.id"
        >
          <h3 class="font-semibold text-lg mb-2">{{ preset.name }}</h3>
          <p class="text-sm opacity-80">{{ preset.description }}</p>
        </div>
      </div>
    </div>

    <div class="bg-primary-700 rounded-lg p-4">
      <h3 class="font-medium mb-4">筛选条件</h3>
      <div class="mb-4">
        <label class="block text-sm text-primary-400 mb-2">评分（多选）</label>
        <RatingSelector v-model="filterRating" />
      </div>
    </div>

    <div>
      <label class="block text-sm font-medium text-primary-300 mb-2">
        保留目标数量
      </label>
      <input
        v-model.number="targetKeep"
        type="number"
        min="1"
        max="100"
        class="w-full px-4 py-2 bg-primary-700 border border-primary-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-secondary-500 text-white"
      />
    </div>

    <DirectorySelector
      v-model="selectedDirectoryId"
      :filter-rating="filterRating"
      :target-keep="targetKeep"
      :root-path="rootPath"
    />

    <div class="flex gap-4">
      <button
        type="button"
        title="浏览该目录下的图片"
        class="py-3 px-5 bg-primary-700 hover:bg-primary-600 rounded-lg transition-colors flex items-center justify-center gap-2 border border-primary-600 text-primary-300 hover:text-white"
        @click="handleBrowse"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24">
          <path :d="mdiFolder" fill="currentColor" />
        </svg>
        <span>浏览</span>
      </button>

      <button
        :disabled="!canCreate || creatingSession"
        class="flex-1 py-3 px-6 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
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
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { mdiLoading, mdiFolder } from "@mdi/js";
import { useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  MetaDocument,
  RootDirectoryDocument,
} from "../graphql/generated";
import RatingSelector from "./RatingSelector.vue";
import DirectorySelector from "./DirectorySelector.vue";
import { useSessionConfig } from "../composables/useSessionConfig";
import useRouteQuery from "../composables/useRouteQuery";
import { updateLastSession } from "../composables/useDirectoryState";

type Emits = (e: "created") => void;

const emit = defineEmits<Emits>();

const router = useRouter();

const {
  presets,
  selectedPresetId,
  targetKeep,
  rating: filterRating,
} = useSessionConfig();

const loadingCount = ref(0);
const creatingSession = ref(false);

const { data: metaData } = useQuery(MetaDocument, {
  loadingCount,
});

const { data: rootData } = useQuery(RootDirectoryDocument, {
  loadingCount,
});

const dirQuery = useRouteQuery("dir");

const selectedDirectoryId = computed({
  get() {
    return dirQuery.value[0] ?? rootData.value?.rootDirectory.id ?? "";
  },
  set(v) {
    if (v === selectedDirectoryId.value) return;
    dirQuery.value = v ? [v] : [];
  },
});

const rootPath = computed(() => metaData.value?.meta?.rootAbsPath || "");

const canCreate = computed(() => {
  return (filterRating.value?.length || 0) > 0 && (targetKeep.value || 0) > 0;
});

async function handleCreate() {
  creatingSession.value = true;

  try {
    const { data } = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          filter: {
            rating: filterRating.value.slice(),
          },
          targetKeep: targetKeep.value,
          directoryId: selectedDirectoryId.value,
        },
      },
    });

    if (data?.createSession) {
      const session = data.createSession.session;
      updateLastSession(session);

      router.push(`/session/${session.id}`);
      emit("created");
    }
  } finally {
    creatingSession.value = false;
  }
}

function handleBrowse() {
  router.push({
    path: "/browse",
    query: { dir: selectedDirectoryId.value },
  });
}
</script>
