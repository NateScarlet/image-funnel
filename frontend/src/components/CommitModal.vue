<template>
  <div
    class="fixed inset-0 bg-black/75 flex items-center justify-center z-50 p-4"
    @click.self="$emit('close')"
  >
    <div class="bg-primary-800 rounded-xl max-w-md w-full p-6 shadow-2xl">
      <CommitForm
        :session-id="sessionId"
        :stats="stats"
        title="提交更改"
        @committed="$emit('committed')"
      >
        <template #actions="{ committing, commitResult, commit }">
          <button
            :disabled="committing"
            class="flex-1 px-4 py-2 bg-primary-700 hover:bg-primary-600 disabled:bg-primary-800 disabled:cursor-not-allowed rounded-lg transition-colors"
            @click="$emit('close')"
          >
            取消
          </button>
          <button
            v-if="!commitResult"
            :disabled="committing"
            class="flex-2 px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg flex items-center justify-center gap-2 transition-colors font-bold"
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
            class="flex-2 px-4 py-2 bg-green-600 hover:bg-green-700 rounded-lg transition-colors font-bold"
            @click="$emit('committed')"
          >
            完成
          </button>
        </template>
      </CommitForm>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import useQuery from "../graphql/utils/useQuery";
import { SessionDocument } from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import { mdiLoading } from "@mdi/js";

const { sessionId } = defineProps<{
  sessionId: string;
}>();
defineEmits<(e: "close" | "committed") => void>();

const { data: sessionData } = useQuery(SessionDocument, {
  variables: () => ({ id: sessionId }),
});

const stats = computed(() => sessionData.value?.session?.stats);
</script>
