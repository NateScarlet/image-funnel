<template>
  <div class="max-w-md w-full mx-auto">
    <div class="text-center mb-8">
      <div class="text-6xl mb-4">🎉</div>
      <h2 class="text-3xl font-bold mb-2 text-white">筛选完成！</h2>
      <p class="text-slate-400">已处理目录中的所有图片</p>
    </div>

    <div
      class="bg-slate-800/50 rounded-2xl p-6 border border-slate-700/50 shadow-xl backdrop-blur-sm"
    >
      <CommitForm
        :session-id="sessionId"
        :stats="stats"
        title=""
        @committed="handleCommitted"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  GetSessionDocument,
  CreateSessionDocument,
} from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import useDirectoryProgress from "../composables/useDirectoryProgress";

interface SessionStats {
  kept: number;
  reviewed: number;
  rejected: number;
}

interface Props {
  sessionId: string;
  stats?: SessionStats;
}

const props = defineProps<Props>();

const router = useRouter();
const { getNextDirectory } = useDirectoryProgress();

const { data: sessionData } = useQuery(GetSessionDocument, {
  variables: () => ({ id: props.sessionId }),
});

const session = computed(() => sessionData.value?.session);

async function handleCommitted() {
  if (!session.value) {
    router.push("/");
    return;
  }

  const { directory, filter, targetKeep } = session.value;
  const nextDirectoryId = getNextDirectory(
    directory.parentId ?? "",
    directory.id,
  );

  if (nextDirectoryId) {
    const { data } = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          filter: {
            rating: filter.rating ?? [],
          },
          targetKeep,
          directoryId: nextDirectoryId,
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
