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
      <CommitForm
        :session-id="sessionId"
        :stats="stats"
        title=""
        @committed="handleCommitted"
      />
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
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import { SessionDocument, CreateSessionDocument } from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import DirectoryDisplay from "./DirectoryDisplay.vue";

interface SessionStats {
  kept: number;
  reviewed: number;
  rejected: number;
}

const { sessionId, stats } = defineProps<{
  sessionId: string;
  stats?: SessionStats;
}>();

const router = useRouter();
const { getNextDirectory } = useDirectoryProgress();

const { data: sessionData } = useQuery(SessionDocument, {
  variables: () => ({ id: sessionId }),
});

const session = computed(() => sessionData.value?.session);

const nextDirectoryId = computed(() => {
  if (!session.value) return undefined;
  return getNextDirectory(
    session.value.directory.parentId ?? "",
    session.value.directory.id,
  );
});

async function handleCommitted() {
  if (!session.value) {
    router.push("/");
    return;
  }

  const { filter, targetKeep } = session.value;
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
      router.push(`/session/${data.createSession.session.id}`);
    }
  } else {
    router.push("/");
  }
}
</script>
