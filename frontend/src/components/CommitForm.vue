<template>
  <div class="space-y-4">
    <div v-if="showHeader" class="mb-4">
      <h2 v-if="title" class="text-xl font-bold mb-4">{{ title }}</h2>
      <p class="text-slate-300 mb-2">
        将
        {{ stats.kept + stats.reviewed + stats.rejected }}
        个操作写入 XMP 文件
      </p>
      <div class="grid grid-cols-3 gap-2 text-sm">
        <div class="bg-green-900 bg-opacity-30 rounded p-2 text-center">
          <div class="text-green-400 font-bold">{{ stats?.kept || 0 }}</div>
          <div class="text-slate-400">保留</div>
        </div>
        <div class="bg-yellow-900 bg-opacity-30 rounded p-2 text-center">
          <div class="text-yellow-400 font-bold">
            {{ stats?.reviewed || 0 }}
          </div>
          <div class="text-slate-400">稍后</div>
        </div>
        <div class="bg-red-900 bg-opacity-30 rounded p-2 text-center">
          <div class="text-red-400 font-bold">{{ stats?.rejected || 0 }}</div>
          <div class="text-slate-400">排除</div>
        </div>
      </div>
    </div>

    <div v-if="!commitResult" class="bg-slate-700/50 rounded-lg p-4">
      <h3 class="font-medium mb-4">写入操作设置</h3>
      <div class="space-y-4">
        <div>
          <label class="block text-sm text-slate-400 mb-2">保留图片评分</label>
          <StarSelector v-model="writeActions.keepRating" mode="single" />
        </div>
        <div>
          <label class="block text-sm text-slate-400 mb-2">稍后图片评分</label>
          <StarSelector v-model="writeActions.pendingRating" mode="single" />
        </div>
        <div>
          <label class="block text-sm text-slate-400 mb-2">排除图片评分</label>
          <StarSelector v-model="writeActions.rejectRating" mode="single" />
        </div>
      </div>
    </div>

    <div v-if="commitResult" class="bg-slate-700/50 rounded-lg p-4">
      <div
        :class="commitResult.success ? 'text-green-400' : 'text-red-400'"
        class="font-bold flex items-center gap-2 mb-1"
      >
        <span>{{ commitResult.success ? "✓ 提交成功" : "✗ 提交失败" }}</span>
      </div>
      <div class="text-sm text-slate-400">
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
          class="flex-1 px-4 py-3 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-bold flex items-center justify-center gap-2 transition-colors"
          @click="commit"
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import mutate from "../graphql/utils/mutate";
import { CommitChangesDocument } from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import StarSelector from "./StarSelector.vue";
import { mdiLoading } from "@mdi/js";

interface Props {
  sessionId: string;
  stats: {
    kept: number;
    reviewed: number;
    rejected: number;
  };
  showHeader?: boolean;
  title?: string;
}

const props = withDefaults(defineProps<Props>(), {
  showHeader: true,
  title: "",
});

const emit = defineEmits(["committed"]);

const { getPreset, lastSelectedPresetId } = usePresets();

const committing = ref(false);
const commitResult = ref<{
  success: boolean;
  written: number;
  failed: number;
  errors: string[];
} | null>(null);

const writeActionsBuffer = ref<{
  keepRating?: number;
  pendingRating?: number;
  rejectRating?: number;
}>({});

const selectedPreset = computed(() => {
  return lastSelectedPresetId.value
    ? getPreset(lastSelectedPresetId.value)
    : undefined;
});

const writeActions = computed({
  get() {
    return {
      keepRating:
        writeActionsBuffer.value.keepRating ??
        selectedPreset.value?.writeActions.keepRating ??
        4,
      pendingRating:
        writeActionsBuffer.value.pendingRating ??
        selectedPreset.value?.writeActions.pendingRating ??
        0,
      rejectRating:
        writeActionsBuffer.value.rejectRating ??
        selectedPreset.value?.writeActions.rejectRating ??
        2,
    };
  },
  set(v) {
    writeActionsBuffer.value = { ...v };
  },
});

async function commit() {
  committing.value = true;

  try {
    const { data } = await mutate(CommitChangesDocument, {
      variables: {
        input: {
          sessionId: props.sessionId,
          writeActions: writeActions.value,
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
  committing,
  commitResult,
});
</script>
