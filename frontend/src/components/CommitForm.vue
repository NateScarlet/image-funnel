<template>
  <form class="space-y-4" @submit.prevent="commit">
    <div v-if="showHeader" class="mb-4">
      <h2 v-if="title" class="text-xl font-bold mb-4">{{ title }}</h2>
      <p class="text-primary-300 mb-2">
        将
        {{
          (session?.stats.kept ?? 0) +
          (session?.stats.reviewed ?? 0) +
          (session?.stats.rejected ?? 0)
        }}
        个操作写入 XMP 文件
      </p>
    </div>

    <div v-if="!commitResult" class="bg-primary-700/50 rounded-lg p-4">
      <h3 class="font-medium mb-4">写入操作设置</h3>
      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-sm font-bold text-green-400">保留</span>
            <span
              class="text-xs bg-primary-800 text-primary-300 px-2 py-1 rounded-full"
            >
              {{ session?.stats.kept || 0 }} 张
            </span>
          </div>
          <RatingSelector v-model="keepRating" />
        </div>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-sm font-bold text-yellow-400">搁置</span>
            <span
              class="text-xs bg-primary-800 text-primary-300 px-2 py-1 rounded-full"
            >
              {{ session?.stats.reviewed || 0 }} 张
            </span>
          </div>
          <RatingSelector v-model="shelveRating" />
        </div>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-sm font-bold text-red-400">排除</span>
            <span
              class="text-xs bg-primary-800 text-primary-300 px-2 py-1 rounded-full"
            >
              {{ session?.stats.rejected || 0 }} 张
            </span>
          </div>
          <RatingSelector v-model="rejectRating" />
        </div>
      </div>
    </div>

    <div v-if="commitResult" class="bg-primary-700/50 rounded-lg p-4">
      <div
        :class="commitResult.success ? 'text-green-400' : 'text-red-400'"
        class="font-bold flex items-center gap-2 mb-1"
      >
        <span>{{ commitResult.success ? "✓ 提交成功" : "✗ 提交失败" }}</span>
      </div>
      <div class="text-sm text-primary-400">
        写入: {{ commitResult.written }} | 失败: {{ commitResult.failed }}
      </div>
      <div
        v-if="commitResult.errors.length > 0"
        class="mt-2 text-sm text-red-300 space-y-1"
      >
        <div
          v-for="(err, i) in commitResult.errors"
          :key="i"
          class="bg-red-900/20 p-2 rounded"
        >
          {{ err }}
        </div>
      </div>
    </div>

    <div class="flex gap-3">
      <slot
        name="actions"
        :committing="committing"
        :commit-result="commitResult"
        :commit="commit"
      >
        <button
          v-if="!commitResult"
          :disabled="committing"
          class="flex-1 px-4 py-3 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-bold flex items-center justify-center gap-2 transition-colors"
          type="submit"
        >
          <svg
            v-if="committing"
            class="w-5 h-5 animate-spin"
            viewBox="0 0 24 24"
          >
            <path :d="mdiLoading" fill="currentColor" />
          </svg>
          <span>确认提交</span>
        </button>
        <button
          v-else
          class="flex-1 px-4 py-3 bg-green-600 hover:bg-green-700 rounded-lg font-bold transition-colors"
          @click="$emit('committed')"
        >
          完成
        </button>
      </slot>
    </div>
  </form>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import mutate from "../graphql/utils/mutate";
import {
  CommitChangesDocument,
  type SessionFragment,
} from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import RatingSelector from "./RatingSelector.vue";
import { mdiLoading } from "@mdi/js";

const {
  session,
  showHeader = true,
  title = "",
} = defineProps<{
  session: SessionFragment;
  showHeader?: boolean;
  title?: string;
}>();

const emit = defineEmits<(e: "committed") => void>();
const { getPreset, lastSelectedPresetId } = usePresets();

const committing = ref(false);
const commitResult = ref<{
  success: boolean;
  written: number;
  failed: number;
  errors: string[];
} | null>(null);

const selectedPreset = computed(() => {
  return getPreset(lastSelectedPresetId.value);
});

// #region Rating Computeds
// 为每一个评分字段创建单独的计算属性，以便 v-model 正确工作
const keepRatingBuffer = ref<number>();
const keepRating = computed({
  get: () =>
    keepRatingBuffer.value ??
    selectedPreset.value?.writeActions.keepRating ??
    0,
  set: (v: number) => {
    keepRatingBuffer.value = v;
  },
});

const shelveRatingBuffer = ref<number>();
const shelveRating = computed({
  get: () =>
    shelveRatingBuffer.value ??
    selectedPreset.value?.writeActions.shelveRating ??
    0,
  set: (v: number) => {
    shelveRatingBuffer.value = v;
  },
});

const rejectRatingBuffer = ref<number>();
const rejectRating = computed({
  get: () =>
    rejectRatingBuffer.value ??
    selectedPreset.value?.writeActions.rejectRating ??
    0,
  set: (v: number) => {
    rejectRatingBuffer.value = v;
  },
});
// #endregion

async function commit() {
  if (committing.value) return;
  committing.value = true;

  try {
    const { data } = await mutate(CommitChangesDocument, {
      variables: {
        input: {
          sessionId: session.id,
          writeActions: {
            keepRating: keepRating.value,
            shelveRating: shelveRating.value,
            rejectRating: rejectRating.value,
          },
        },
      },
    });

    if (data) {
      commitResult.value = data.commitChanges;

      if (data.commitChanges.success && data.commitChanges.failed === 0) {
        setTimeout(() => {
          emit("committed");
        }, 500);
      }
    }
  } catch (err: unknown) {
    commitResult.value = {
      success: false,
      written: 0,
      failed: 1,
      errors: [err instanceof Error ? err.message : "Unknown error"],
    };
  } finally {
    committing.value = false;
  }
}

defineExpose({
  commit,
});
</script>
