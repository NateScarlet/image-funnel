<template>
  <div>
    <div
      v-if="show"
      class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
      @click.self="show = false"
    >
      <div class="bg-slate-800 rounded-lg p-6 w-full max-w-sm">
        <div class="mb-6">
          <h3 class="text-lg font-bold mb-2">会话信息</h3>
          <div class="text-sm text-slate-400 mb-1">筛选条件</div>
          <div class="text-base">
            {{ session?.filter?.rating?.join(", ") || "无" }}
          </div>
        </div>

        <div class="space-y-3">
          <button
            :disabled="!canUndo"
            class="w-full py-3 px-4 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="handleUndo"
          >
            <svg
              v-if="undoing"
              class="w-5 h-5 animate-spin"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiUndo" fill="currentColor" />
            </svg>
            <span> 撤销</span>
          </button>

          <button
            class="w-full py-3 px-4 bg-slate-700 hover:bg-slate-600 rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="emit('showUpdateSessionModal')"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCogOutline" fill="currentColor" />
            </svg>
            修改预设
          </button>

          <button
            :disabled="!session?.canCommit"
            class="w-full py-3 px-4 bg-secondary-600 hover:bg-secondary-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="emit('showCommitModal')"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheck" fill="currentColor" />
            </svg>
            提交
          </button>

          <button
            class="w-full py-3 px-4 bg-red-600 hover:bg-red-700 rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="handleAbandon"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCloseCircleOutline" fill="currentColor" />
            </svg>
            放弃
          </button>
        </div>

        <button
          class="mt-4 w-full py-2 px-4 bg-slate-700 hover:bg-slate-600 rounded-lg text-sm transition-colors"
          @click="show = false"
        >
          关闭
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import {
  mdiUndo,
  mdiCloseCircleOutline,
  mdiCheck,
  mdiLoading,
  mdiCogOutline,
} from "@mdi/js";
import mutate from "../graphql/utils/mutate";
import { UndoDocument } from "../graphql/generated";

interface SessionFilter {
  rating?: number[] | null;
}

interface Session {
  filter?: SessionFilter;
  canCommit?: boolean;
  targetKeep?: number;
}

interface Props {
  show: boolean;
  session?: Session | null;
  canUndo?: boolean;
  undoing: boolean;
  sessionId: string;
  stats?: { kept?: number };
}

interface Emits {
  (e: "update:show", value: boolean): void;
  (e: "abandoned" | "showCommitModal" | "showUpdateSessionModal"): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();

const show = computed({
  get: () => props.show,
  set: (value: boolean) => emit("update:show", value),
});

const router = useRouter();

async function handleUndo() {
  await mutate(UndoDocument, {
    variables: { input: { sessionId: props.sessionId } },
  });
  show.value = false;
}

function handleAbandon() {
  show.value = false;
  if (confirm("确定要放弃当前会话吗？所有未提交的更改将会丢失。")) {
    emit("abandoned");
    router.push("/");
  }
}
</script>
