<template>
  <div class="mt-8">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-xl font-bold text-white">
        保留的图片
        <span class="text-primary-400 text-sm font-normal ml-2">
          (已加载 {{ images.length }} / {{ totalKept }})
        </span>
      </h3>
    </div>

    <!-- Skeleton Loader for initial load -->
    <div
      v-if="loading && images.length === 0"
      class="grid grid-cols-2 xs:grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-4"
    >
      <div
        v-for="i in 12"
        :key="i"
        class="aspect-square bg-primary-800 rounded-lg animate-pulse"
      ></div>
    </div>

    <div
      v-else
      class="grid grid-cols-2 xs:grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-4"
    >
      <div
        v-for="image in images"
        :key="image.id"
        class="group relative bg-primary-800 rounded-lg overflow-hidden border border-primary-700/50 hover:border-primary-500 transition-colors [@media(hover:hover)]:aspect-square"
      >
        <img
          :src="image.url256"
          :alt="image.filename"
          loading="lazy"
          class="w-full object-cover transition-transform duration-500 group-hover:scale-105 aspect-square [@media(hover:hover)]:h-full"
        />

        <!-- Info Overlay -->
        <div
          class="flex flex-col justify-end p-3 transition-opacity duration-200 opacity-100 [@media(hover:hover)]:absolute [@media(hover:hover)]:inset-0 [@media(hover:hover)]:bg-gradient-to-t [@media(hover:hover)]:from-black/90 [@media(hover:hover)]:via-black/40 [@media(hover:hover)]:to-transparent [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:pointer-events-none"
        >
          <div
            class="transform translate-y-0 transition-transform duration-200 pointer-events-auto [@media(hover:hover)]:translate-y-2 [@media(hover:hover)]:group-hover:translate-y-0"
          >
            <div
              class="text-white text-sm font-medium truncate mb-1"
              :title="image.filename"
            >
              {{ image.filename }}
            </div>

            <a
              :href="image.url"
              download
              class="flex items-center justify-center gap-2 w-full py-1.5 bg-white/10 hover:bg-white/20 active:bg-white/30 text-xs text-white rounded transition-colors backdrop-blur-sm"
              @click.stop
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                <path :d="mdiDownload" />
              </svg>
              <span>{{ formatSize(image.size) }}</span>
            </a>
          </div>
        </div>
      </div>
    </div>

    <!-- Load More Trigger / Loading State -->
    <div v-if="hasMore" class="mt-8 flex justify-center pb-8">
      <div v-if="loading" class="flex justify-center">
        <!-- Pure CSS Loader or SVG Spinner -->
        <svg
          class="animate-spin h-8 w-8 text-primary-400"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          ></circle>
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          ></path>
        </svg>
      </div>
      <button
        v-else
        class="p-2 rounded-full hover:bg-primary-700 text-primary-400 hover:text-white transition-colors"
        title="加载更多"
        @click="loadMore"
      >
        <svg class="w-6 h-6" viewBox="0 0 24 24" fill="currentColor">
          <path :d="mdiChevronDown" />
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { KeptImagesDocument, type ImageFragment } from "../graphql/generated";
import useQuery from "../graphql/utils/useQuery";
import { formatSize } from "../utils/formatSize";
import { mdiDownload, mdiChevronDown } from "@mdi/js";

const props = defineProps<{
  sessionId: string;
  totalKept: number;
}>();

const limit = 60;
const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);

const { data, query } = useQuery(KeptImagesDocument, {
  variables: {
    sessionId: props.sessionId,
    limit,
    offset: 0,
  },
  loadingCount,
  fetchPolicy: "network-only", // 确保获取最新数据
});

const images = computed(() => {
  return data.value?.session?.keptImages ?? ([] as ImageFragment[]);
});

const hasMore = computed(() => {
  return images.value.length < props.totalKept;
});

async function loadMore() {
  if (loading.value) return;

  await query.fetchMore({
    variables: {
      offset: images.value.length,
      limit,
    },
    updateQuery: (prev, { fetchMoreResult }) => {
      if (!fetchMoreResult?.session?.keptImages) return prev;
      if (!prev.session) return fetchMoreResult;

      return {
        ...prev,
        session: {
          ...prev.session,
          keptImages: [
            ...(prev.session.keptImages ?? []),
            ...fetchMoreResult.session.keptImages,
          ],
        },
      };
    },
  });
}
</script>
