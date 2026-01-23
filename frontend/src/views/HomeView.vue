<template>
  <div class="min-h-screen bg-primary-900 text-primary-100 p-4 md:p-8">
    <div class="max-w-4xl mx-auto">
      <header class="mb-8">
        <h1 class="text-3xl md:text-4xl font-bold text-center mb-2">
          ImageFunnel
          <span
            v-if="version"
            class="text-lg md:text-xl text-primary-400 font-normal ml-2"
          >
            {{ version }}
          </span>
        </h1>
        <p class="text-primary-400 text-center">图片筛选工具</p>
      </header>

      <div class="bg-primary-800 rounded-lg p-6 mb-6">
        <h2 class="text-xl font-semibold mb-4">创建新会话</h2>

        <CreateSessionForm />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import useQuery from "../graphql/utils/useQuery";
import { MetaDocument } from "../graphql/generated";
import CreateSessionForm from "../components/CreateSessionForm.vue";

const loadingCount = ref(0);

const { data: metaData } = useQuery(MetaDocument, {
  loadingCount,
});

const version = computed(() => metaData.value?.meta?.version || "");
</script>
