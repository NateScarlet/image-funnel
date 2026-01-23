<template>
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
    @click.self="$emit('close')"
  >
    <div class="bg-primary-800 rounded-lg p-6 w-full max-w-md">
      <div class="mb-4">
        <h2 class="text-xl font-bold">修改筛选配置</h2>
        <p class="text-primary-400 text-sm mt-1">调整目标保留数量和筛选条件</p>
      </div>

      <div class="space-y-4">
        <!-- 预设选择 -->
        <div>
          <label class="block text-sm font-medium text-primary-300 mb-2">
            选择预设
          </label>
          <select
            v-model="selectedPresetId"
            class="w-full bg-primary-700 border border-primary-600 rounded-lg px-4 py-2 text-white focus:ring-2 focus:ring-secondary-500 focus:border-transparent"
          >
            <option value="">自定义</option>
            <option
              v-for="preset in presets"
              :key="preset.id"
              :value="preset.id"
            >
              {{ preset.name }} - {{ preset.description }}
            </option>
          </select>
        </div>

        <!-- 目标保留数量 -->
        <div>
          <label class="block text-sm font-medium text-primary-300 mb-2">
            目标保留数量
            <span class="text-primary-400 ml-2 text-xs"
              >({{ session.stats?.kept ?? 0 }} / {{ targetKeep }})</span
            >
          </label>
          <input
            v-model.number="targetKeep"
            type="number"
            min="1"
            class="w-full bg-primary-700 border border-primary-600 rounded-lg px-4 py-2 text-white focus:ring-2 focus:border-transparent"
            placeholder="输入要保留的图片数量"
          />
        </div>

        <!-- 筛选条件 -->
        <div>
          <label class="block text-sm font-medium text-primary-300 mb-2">
            筛选条件
          </label>
          <RatingSelector v-model="rating" />
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <button
          class="px-4 py-2 bg-primary-700 hover:bg-primary-600 rounded-lg text-sm transition-colors"
          @click="$emit('close')"
        >
          取消
        </button>
        <button
          class="px-4 py-2 bg-secondary-600 hover:bg-secondary-700 rounded-lg text-sm transition-colors"
          :disabled="updating"
          @click="update"
        >
          <span>保存</span>
        </button>
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
