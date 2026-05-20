<template>
  <div class="w-full flex flex-col items-center">
    <div class="max-w-md w-full mx-auto text-primary-100">
      <div class="text-center mb-8">
        <div class="text-6xl mb-4">🎉</div>
        <h2 class="text-3xl font-bold mb-2 text-white">筛选完成！</h2>
        <p class="text-primary-400">已处理目录中的所有图片</p>
        <button
          v-if="session.canUndo"
          class="mt-4 px-4 py-2 text-primary-300 hover:text-white hover:bg-white/10 rounded-lg transition-colors flex items-center justify-center gap-2 mx-auto"
          @click="$emit('undo')"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
            <path :d="mdiUndo" />
          </svg>
          撤销上一张
        </button>
      </div>

      <div
        class="bg-primary-800/50 rounded-2xl p-6 border border-primary-700/50 shadow-xl backdrop-blur-sm"
      >
        <CommitForm
          ref="commitForm"
          :session
          title=""
          @committed="handleCommitted"
        >
          <template #actions="{ committing, commitResult, commit }">
            <button
              v-if="!commitResult"
              :disabled="committing"
              class="flex-1 px-4 py-3 bg-secondary-600 hover:bg-secondary-700 disabled:bg-primary-600 disabled:cursor-not-allowed rounded-lg font-bold flex items-center justify-center gap-2 transition-colors"
              type="button"
              @click="interceptCommit(commit)"
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
              type="button"
              @click="handleCommitted"
            >
              完成
            </button>
          </template>
        </CommitForm>
      </div>

      <div v-if="nextDirectoryId" class="mt-6">
        <p class="text-sm text-primary-400 mb-3">下一个目录</p>
        <div
          class="p-4 rounded-lg transition-all border-2 bg-primary-600 border-primary-500"
        >
          <DirectoryDisplay :directory="{ id: nextDirectoryId }" />
        </div>
      </div>
    </div>

    <div v-if="session.stats.totalKept > 0" class="w-full max-w-6xl px-4 mt-8">
      <KeptImagesGrid
        :session-id="session.id"
        :total-kept="session.stats.totalKept"
      />
    </div>

    <!-- 确认对话框 -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div
          v-if="showConfirm"
          class="fixed inset-0 bg-black/80 flex items-center justify-center z-60 p-4 backdrop-blur-md"
          @click.self="showConfirm = false"
        >
          <div
            class="bg-primary-800 rounded-2xl max-w-sm w-full p-8 shadow-2xl border border-primary-700 transform transition-all"
            :class="showConfirm ? 'scale-100' : 'scale-95'"
          >
            <div class="text-center">
              <div
                class="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mx-auto mb-4"
              >
                <svg class="w-8 h-8 text-red-500" viewBox="0 0 24 24">
                  <path :d="mdiAlertOutline" fill="currentColor" />
                </svg>
              </div>
              <h3 class="text-xl font-bold mb-2 text-white">
                确认排除所有图片？
              </h3>
              <p class="text-primary-300 mb-8 leading-relaxed">
                您当前没有保留任何图片，这将会把目录下的所有图片标记为排除并更新
                XMP 文件。提交后，如果需要找回这些图片，您可能需要重新开始筛选。
              </p>
              <div class="flex gap-3">
                <button
                  class="flex-1 px-4 py-3 bg-primary-700 hover:bg-primary-600 rounded-xl transition-colors font-medium text-primary-100"
                  @click="showConfirm = false"
                >
                  取消
                </button>
                <button
                  class="flex-1 px-4 py-3 bg-red-600 hover:bg-red-700 rounded-xl font-bold transition-colors text-white shadow-lg shadow-red-900/20"
                  @click="handleConfirm"
                >
                  确定排除
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  type SessionFragment,
} from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import DirectoryDisplay from "./DirectoryDisplay.vue";
import KeptImagesGrid from "./KeptImagesGrid.vue";
import { mdiUndo, mdiLoading, mdiAlertOutline } from "@mdi/js";

const { session } = defineProps<{
  session: SessionFragment;
}>();

defineEmits<(e: "undo") => void>();

const router = useRouter();
const { getNextDirectory } = useDirectoryProgress();

const nextDirectoryId = computed(() => {
  return getNextDirectory(
    session.directory.parentId ?? "",
    session.directory.id,
  );
});

const commitForm =
  useTemplateRef<InstanceType<typeof CommitForm>>("commitForm");

const showConfirm = ref(false);
let pendingCommit: (() => void) | null = null;

function interceptCommit(commitFn: () => void) {
  if (session.stats.totalKept === 0) {
    showConfirm.value = true;
    pendingCommit = commitFn;
  } else {
    commitFn();
  }
}

function handleConfirm() {
  showConfirm.value = false;
  if (pendingCommit) {
    pendingCommit();
    pendingCommit = null;
  }
}

function submit() {
  interceptCommit(() => commitForm.value?.commit());
}

defineExpose({
  submit,
});

async function handleCommitted() {
  const { filter, targetKeep } = session;
  const nextDirectoryIdValue = nextDirectoryId.value;

  if (nextDirectoryIdValue) {
    const { data } = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          filter: {
            rating: filter.rating ?? [],
          },
          targetKeep,
          directoryId: nextDirectoryIdValue,
        },
      },
    });

    if (data?.createSession) {
      await router.push({
        name: "session",
        params: {
          id: data.createSession.session.id,
        },
      });
    }
  } else {
    await router.push("/");
  }
}
</script>
