<template>
  <header
    class="bg-primary-800 border-b border-primary-700 p-2 md:p-4 shrink-0"
    data-no-gesture
  >
    <div class="max-w-7xl mx-auto flex items-center justify-between">
      <div
        class="cursor-pointer p-1 hover:bg-primary-700 rounded-lg transition-colors mr-2"
        @click="goHome"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24">
          <path :d="mdiHome" fill="currentColor" />
        </svg>
      </div>

      <div class="flex-1 min-w-0 mr-4">
        <div class="text-xs md:text-sm text-primary-400 truncate">
          {{ displayName }}
        </div>
        <div class="text-sm md:text-lg font-semibold truncate">
          {{ session?.currentIndex || 0 }} / {{ session?.currentSize || 0 }}
          <span class="text-green-400 ml-2"
            >保留: {{ session?.stats.kept || 0 }} /
            {{ session?.targetKeep || 0 }}</span
          >
        </div>
      </div>

      <slot name="extra" />

      <button
        class="md:hidden p-2 rounded-lg hover:bg-primary-700 transition-colors"
        @click="$emit('showMenu')"
      >
        <svg
          class="w-6 h-6"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path :d="mdiMenu" fill="currentColor" />
        </svg>
      </button>

      <div class="hidden md:flex items-center gap-4">
        <button
          :disabled="!session?.canUndo || undoing"
          class="px-4 py-2 bg-primary-700 hover:bg-primary-600 disabled:bg-primary-800 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('undo')"
        >
          <svg v-if="undoing" class="w-5 h-5 animate-spin" viewBox="0 0 24 24">
            <path :d="mdiLoading" fill="currentColor" />
          </svg>
          <svg v-else class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiUndo" fill="currentColor" />
          </svg>
          <span>撤销</span>
        </button>

        <button
          class="px-4 py-2 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('showUpdateSessionModal')"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiCogOutline" fill="currentColor" />
          </svg>
          修改预设
        </button>

        <button
          :disabled="!session?.canCommit"
          class="px-4 py-2 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors flex items-center gap-2 whitespace-nowrap"
          @click="$emit('showCommitModal')"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24">
            <path :d="mdiCheck" fill="currentColor" />
          </svg>
          提交
        </button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import {
  mdiMenu,
  mdiUndo,
  mdiCheck,
  mdiLoading,
  mdiCogOutline,
  mdiHome,
} from "@mdi/js";
import { computed } from "vue";
import basename from "@/utils/basename";
import useQuery from "@/graphql/utils/useQuery";
import { MetaDocument, SessionFragment } from "@/graphql/generated";

const router = useRouter();

function goHome() {
  router.push("/");
}

const { session, undoing } = defineProps<{
  session: SessionFragment | null | undefined;
  undoing: boolean;
}>();

defineEmits<
  (
    e: "showMenu" | "undo" | "showUpdateSessionModal" | "showCommitModal",
  ) => void
>();

const { data } = useQuery(MetaDocument);
const displayName = computed(() => {
  if (session?.directory?.root) {
    return data.value?.meta.rootPath ?? "/";
  }
  return basename(session?.directory?.path ?? "");
});
</script>
