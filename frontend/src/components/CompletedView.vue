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

    <div v-if="nextDirectoryId" class="mt-6">
      <p class="text-sm text-slate-400 mb-3">下一个目录</p>
      <div
        class="p-4 rounded-lg transition-all border-2 bg-slate-600 border-slate-500"
      >
        <div class="flex items-start gap-3">
          <div class="flex-shrink-0 rounded overflow-hidden">
            <img
              v-if="nextDirectoryStats?.latestImage"
              :src="nextDirectoryStats.latestImage.url256"
              :alt="nextDirectoryPath"
              class="w-20 bg-slate-700 object-cover"
            />
            <div
              v-else
              class="w-20 h-20 flex-shrink-0 bg-slate-700 rounded overflow-hidden"
            >
              <div class="w-full h-full animate-pulse bg-slate-600"></div>
            </div>
          </div>
          <div class="flex-1 min-w-0">
            <h3 class="font-semibold text-lg mb-1">
              <span class="flex-1 break-all">{{ nextDirectoryPath }}</span>
            </h3>
            <div class="text-xs text-slate-300 space-y-1">
              <div v-if="nextDirectoryStats">
                <div v-if="nextDirectoryStats.latestImage?.modTime">
                  {{ formatDate(nextDirectoryStats.latestImage.modTime) }}
                </div>
                <div
                  v-if="nextDirectoryStats.ratingCounts.length > 0"
                  class="flex flex-wrap gap-2 mt-2"
                >
                  <div
                    v-for="rc in sortedRatingCounts(
                      nextDirectoryStats.ratingCounts,
                    )"
                    :key="rc.rating"
                    class="flex items-center gap-1 px-2 py-1 rounded bg-slate-700/50"
                  >
                    <RatingIcon :rating="rc.rating" />
                    <span class="text-xs">{{ rc.count }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  GetSessionDocument,
  CreateSessionDocument,
  GetDirectoryStatsDocument,
} from "../graphql/generated";
import CommitForm from "./CommitForm.vue";
import useDirectoryProgress from "../composables/useDirectoryProgress";
import RatingIcon from "./RatingIcon.vue";
import { formatDate } from "../utils/date";
import { sortBy } from "es-toolkit";
import type { RatingCountFragment } from "../graphql/generated";

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

const nextDirectoryId = computed(() => {
  if (!session.value) return undefined;
  return getNextDirectory(
    session.value.directory.parentId ?? "",
    session.value.directory.id,
  );
});

const loadingCount = ref(0);
const { data: nextDirectoryData } = useQuery(GetDirectoryStatsDocument, {
  variables: () =>
    nextDirectoryId.value ? { id: nextDirectoryId.value } : undefined,
  loadingCount,
});

const nextDirectoryStats = computed(
  () => nextDirectoryData.value?.directory?.stats,
);
const nextDirectoryPath = computed(
  () => nextDirectoryData.value?.directory?.path ?? "",
);

function sortedRatingCounts(
  ratingCounts: RatingCountFragment[],
): RatingCountFragment[] {
  return sortBy(ratingCounts, [(rc) => rc.rating]);
}

async function handleCommitted() {
  if (!session.value) {
    router.push("/");
    return;
  }

  const { directory, filter, targetKeep } = session.value;
  const nextDirectoryIdValue = getNextDirectory(
    directory.parentId ?? "",
    directory.id,
  );

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
