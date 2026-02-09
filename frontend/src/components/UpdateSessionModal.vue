<template>
  <div
    class="fixed inset-0 z-50 overflow-y-auto bg-black/75"
    data-no-gesture
    @click.self="$emit('close')"
  >
    <div class="flex min-h-full items-center justify-center p-4">
      <div class="w-full max-w-md rounded-xl bg-primary-800 p-6 shadow-2xl">
        <div class="mb-6">
          <h2 class="text-xl font-bold">修改筛选配置</h2>
          <p class="mt-1 text-sm text-primary-400">
            调整目标保留数量和筛选条件
          </p>
        </div>

        <div class="space-y-4">
          <!-- 预设选择 -->
          <div>
            <label class="mb-2 block text-sm font-medium text-primary-300">
              选择预设
            </label>
            <div class="grid grid-cols-2 gap-3">
              <div
                v-for="preset in presets"
                :key="preset.id"
                :class="[
                  'p-3 rounded-lg cursor-pointer transition-all border-2',
                  selectedPresetId === preset.id
                    ? 'bg-secondary-600 border-secondary-500 shadow-md shadow-secondary-500/20'
                    : 'bg-primary-700 border-primary-600 hover:border-primary-500 hover:bg-primary-650',
                ]"
                @click="selectedPresetId = preset.id"
              >
                <h3 class="font-semibold text-sm">{{ preset.name }}</h3>
                <p class="text-xs opacity-70 line-clamp-2">
                  {{ preset.description }}
                </p>
              </div>
            </div>
          </div>

          <!-- 目标保留数量 -->
          <div class="rounded-lg bg-primary-700/50 p-4">
            <label class="mb-2 block text-sm font-medium text-primary-300">
              目标保留数量
              <span class="ml-2 text-xs text-primary-400"
                >({{ session.stats?.kept ?? 0 }} / {{ targetKeep }})</span
              >
            </label>
            <input
              v-model.number="targetKeep"
              type="number"
              min="1"
              class="w-full rounded-lg border border-primary-600 bg-primary-700 px-4 py-2 text-white focus:border-transparent focus:ring-2 focus:ring-secondary-500"
              placeholder="输入要保留的图片数量"
            />
          </div>

          <!-- 筛选条件 -->
          <div class="rounded-lg bg-primary-700/50 p-4">
            <div class="mb-2 flex items-center justify-between">
              <span class="block text-sm font-medium text-primary-300">
                筛选条件
              </span>
            </div>
            <RatingSelector v-model="rating" />
          </div>
        </div>

        <div class="mt-6 flex justify-end gap-3">
          <button
            class="rounded-lg bg-primary-700 px-4 py-2 text-sm transition-colors hover:bg-primary-600"
            @click="$emit('close')"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-secondary-600 px-4 py-2 text-sm transition-colors hover:bg-secondary-700 disabled:cursor-not-allowed disabled:bg-primary-600"
            :disabled="updating"
            @click="update"
          >
            <span>保存</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import type { SessionFragment } from "../graphql/generated";
import mutate from "../graphql/utils/mutate";
import { UpdateSessionDocument } from "../graphql/generated";
import { useSessionConfig } from "../composables/useSessionConfig";
import RatingSelector from "./RatingSelector.vue";

interface Props {
  visible?: boolean;
  session: SessionFragment;
}

const props = defineProps<Props>();

const emit = defineEmits<(e: "close" | "updated") => void>();

const { presets, selectedPresetId, targetKeep, rating } = useSessionConfig(
  () => props.session,
);

// 触发更新事件
const updating = ref<boolean>(false);

async function update() {
  if (updating.value) {
    return;
  }
  updating.value = true;

  try {
    await mutate(UpdateSessionDocument, {
      variables: {
        input: {
          sessionId: props.session.id,
          targetKeep: targetKeep.value,
          filter: {
            rating: rating.value,
          },
        },
      },
    });

    emit("updated");
    emit("close");
  } finally {
    updating.value = false;
  }
}
</script>
