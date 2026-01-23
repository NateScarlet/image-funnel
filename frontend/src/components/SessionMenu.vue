<template>
  <div>
    <div
      v-if="show"
      class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
      @click.self="show = false"
    >
      <div class="bg-primary-800 rounded-lg p-6 w-full max-w-sm">
        <div class="space-y-3">
          <button
            class="w-full py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="
              emit('showUpdateSessionModal');
              show = false;
            "
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCogOutline" fill="currentColor" />
            </svg>
            修改预设
          </button>

          <button
            :disabled="!session?.canCommit"
            class="w-full py-3 px-4 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-3 whitespace-nowrap"
            @click="emit('showCommitModal')"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiCheck" fill="currentColor" />
            </svg>
            提交
          </button>
        </div>

        <button
          class="mt-4 w-full py-2 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg text-sm transition-colors"
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
import { mdiCheck, mdiCogOutline } from "@mdi/js";
import { SessionFragment } from "@/graphql/generated";

interface Props {
  show: boolean;
  session?: SessionFragment | null;
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
</script>
