<template>
  <div
    class="fixed inset-0 bg-black bg-opacity-75 flex items-center justify-center z-50 p-4"
  >
    <div class="bg-slate-800 rounded-lg max-w-md w-full p-6">
      <h2 class="text-xl font-bold mb-4">提交更改</h2>

      <div class="mb-4">
        <p class="text-slate-300 mb-2">
          将 {{ stats?.processed || 0 }} 个操作写入 XMP 文件
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

      <div v-if="!commitResult" class="mb-4 bg-slate-700 rounded-lg p-4">
        <h3 class="font-medium mb-4">写入操作设置</h3>
        <div class="space-y-4">
          <div>
            <label class="block text-sm text-slate-400 mb-2"
              >保留图片评分</label
            >
            <StarSelector v-model="writeActions.keepRating" mode="single" />
          </div>
          <div>
            <label class="block text-sm text-slate-400 mb-2"
              >稍后图片评分</label
            >
            <StarSelector v-model="writeActions.pendingRating" mode="single" />
          </div>
          <div>
            <label class="block text-sm text-slate-400 mb-2"
              >排除图片评分</label
            >
            <StarSelector v-model="writeActions.rejectRating" mode="single" />
          </div>
        </div>
      </div>

      <div v-if="commitResult" class="mb-4">
        <div :class="commitResult.success ? 'text-green-400' : 'text-red-400'">
          {{ commitResult.success ? "✓ 提交成功" : "✗ 提交失败" }}
        </div>
        <div class="text-sm text-slate-400">
          写入: {{ commitResult.written }} | 失败: {{ commitResult.failed }}
        </div>
        <div
          v-if="commitResult.errors.length > 0"
          class="mt-2 text-sm text-red-300"
        >
          <div v-for="(err, i) in commitResult.errors" :key="i">{{ err }}</div>
        </div>
      </div>

      <div class="flex gap-3">
        <button
          :disabled="committing"
          class="flex-1 px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg"
          @click="$emit('close')"
        >
          取消
        </button>
        <button
          v-if="!commitResult"
          :disabled="committing"
          class="flex-1 px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg flex items-center justify-center gap-2"
          @click="commit"
        >
          <svg
            v-if="committing"
            class="w-5 h-5 animate-spin"
            viewBox="0 0 24 24"
          >
            <path :d="mdiLoading" fill="currentColor" />
          </svg>
          <span>{{ committing ? "提交中..." : "确认提交" }}</span>
        </button>
        <button
          v-else
          class="flex-1 px-4 py-2 bg-green-600 hover:bg-green-700 rounded-lg"
          @click="$emit('committed')"
        >
          完成
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  GetSessionDocument,
  CommitChangesDocument,
} from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import StarSelector from "./StarSelector.vue";
import { mdiLoading } from "@mdi/js";

interface Props {
  sessionId: string;
}

interface CommitResult {
  success: boolean;
  written: number;
  failed: number;
  errors: string[];
}

const props = defineProps<Props>();
const emit = defineEmits(["close", "committed"]);

const { getPreset } = usePresets();

const { data: sessionData } = useQuery(GetSessionDocument, {
  variables: () => ({ id: props.sessionId }),
});

const stats = computed(() => sessionData.value?.session?.stats);
const committing = ref(false);
const commitResult = ref<CommitResult | null>(null);

const writeActions = ref({
  keepRating: 4,
  pendingRating: 0,
  rejectRating: 2,
});

onMounted(() => {
  const lastPresetId = localStorage.getItem("lastSelectedPresetId");
  if (lastPresetId) {
    const preset = getPreset(lastPresetId);
    if (preset) {
      writeActions.value = { ...preset.writeActions };
    }
  }
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
</script>
