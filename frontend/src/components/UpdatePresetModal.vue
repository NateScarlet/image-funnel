<template>
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
    @click.self="$emit('close')"
  >
    <div class="bg-slate-800 rounded-lg p-6 w-full max-w-md">
      <div class="mb-4">
        <h2 class="text-xl font-bold">修改筛选配置</h2>
        <p class="text-slate-400 text-sm mt-1">调整目标保留数量和筛选条件</p>
      </div>

      <div class="space-y-4">
        <!-- 预设选择 -->
        <div>
          <label class="block text-sm font-medium text-slate-300 mb-2">
            选择预设
          </label>
          <select
            v-model="selectedPresetId"
            class="w-full bg-slate-700 border border-slate-600 rounded-lg px-4 py-2 text-white focus:ring-2 focus:ring-secondary-500 focus:border-transparent"
            @change="onPresetChange"
          >
            <option value="custom">自定义</option>
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
          <label class="block text-sm font-medium text-slate-300 mb-2">
            目标保留数量
            <span class="text-slate-400 ml-2 text-xs"
              >({{ kept }} / {{ targetKeep }})</span
            >
          </label>
          <input
            v-model.number="targetKeep"
            type="number"
            min="1"
            class="w-full bg-slate-700 border border-slate-600 rounded-lg px-4 py-2 text-white focus:ring-2 focus:border-transparent"
            placeholder="输入要保留的图片数量"
          />
        </div>

        <!-- 筛选条件 -->
        <div>
          <label class="block text-sm font-medium text-slate-300 mb-2">
            筛选条件
          </label>
          <StarSelector v-model="rating" mode="multi" />
        </div>
      </div>

      <div class="mt-6 flex justify-end gap-3">
        <button
          class="px-4 py-2 bg-slate-700 hover:bg-slate-600 rounded-lg text-sm transition-colors"
          @click="$emit('close')"
        >
          取消
        </button>
        <button
          class="px-4 py-2 bg-secondary-600 hover:bg-secondary-700 rounded-lg text-sm transition-colors"
          :disabled="updating"
          @click="update"
        >
          <span v-if="updating" class="animate-pulse">保存中...</span>
          <span v-else>保存</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import mutate from "../graphql/utils/mutate";
import { UpdateSessionDocument } from "../graphql/generated";
import { usePresets } from "../composables/usePresets";
import StarSelector from "./StarSelector.vue";

interface Props {
  visible?: boolean;
  targetKeep?: number;
  filter?: {
    rating: number[];
  };
  kept?: number;
  sessionId: string;
}

const props = withDefaults(defineProps<Props>(), {
  visible: false,
  targetKeep: 4,
  filter: () => ({ rating: [0, 4] }),
  kept: 0,
  sessionId: "",
});

const emit = defineEmits<(e: "close" | "updated") => void>();

const { presets } = usePresets();
const selectedPresetId = ref<string>("custom");
const updating = ref<boolean>(false);

// 缓冲变量，用于存储用户主动修改的值
const targetKeepBuffer = ref<number>();
const ratingBuffer = ref<number[]>();

// 目标保留数量的computed属性
const targetKeep = computed({
  get: () => targetKeepBuffer.value ?? props.targetKeep,
  set: (value: number) => {
    targetKeepBuffer.value = value;
  },
});

// 筛选条件的rating属性
const rating = computed({
  get: () => ratingBuffer.value ?? [...props.filter.rating],
  set: (value: number[]) => {
    ratingBuffer.value = value;
  },
});

// 选择预设时更新表单
function onPresetChange() {
  if (selectedPresetId.value === "custom") {
    return;
  }

  const preset = presets.value.find((p) => p.id === selectedPresetId.value);
  if (preset) {
    targetKeepBuffer.value = preset.targetKeep;
    ratingBuffer.value = [...preset.filter.rating];
  }
}

// 触发更新事件
async function update() {
  updating.value = true;

  try {
    await mutate(UpdateSessionDocument, {
      variables: {
        input: {
          sessionId: props.sessionId,
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
