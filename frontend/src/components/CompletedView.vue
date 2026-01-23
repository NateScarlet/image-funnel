<template>
  <div class="max-w-md w-full mx-auto">
    <div class="text-center mb-8">
      <div class="text-6xl mb-4">🎉</div>
      <h2 class="text-3xl font-bold mb-2 text-white">筛选完成！</h2>
      <p class="text-primary-400">已处理目录中的所有图片</p>
    </div>

    <div
      class="bg-primary-800/50 rounded-2xl p-6 border border-primary-700/50 shadow-xl backdrop-blur-sm"
    >
      <CommitForm :session title="" @committed="handleCommitted" />
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
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  type SessionFragment,
} from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import DirectoryDisplay from "./DirectoryDisplay.vue";

const { session } = defineProps<{
  session: SessionFragment;
}>();

const router = useRouter();
const { getNextDirectory } = useDirectoryProgress();

const nextDirectoryId = computed(() => {
  return getNextDirectory(
    session.directory.parentId ?? "",
    session.directory.id,
  );
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
