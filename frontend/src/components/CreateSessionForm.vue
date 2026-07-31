<template>
  <div class="space-y-6">
    <div>
      <label class="block text-sm font-medium text-primary-300 mb-4"> 选择评分预设 </label>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div
          v-for="preset in presets"
          :key="preset.id"
          :class="[
            'p-4 rounded-lg cursor-pointer transition-all border-2',
            selectedPresetId === preset.id
              ? 'bg-secondary-600 border-secondary-500 shadow-lg shadow-secondary-500/30'
              : 'bg-primary-700 border-primary-600 hover:border-primary-500 hover:bg-primary-650',
          ]"
          @click="selectedPresetId = preset.id"
        >
          <h3 class="font-semibold text-lg mb-2">{{ preset.name }}</h3>
          <p class="text-sm opacity-80">{{ preset.description }}</p>
        </div>
      </div>
    </div>

    <div class="bg-primary-700 rounded-lg p-4">
      <h3 class="font-medium mb-4">筛选条件</h3>
      <div class="mb-4">
        <label class="block text-sm text-primary-400 mb-2">评分（多选）</label>
        <RatingSelector v-model="filterRating" />
      </div>
    </div>

    <div>
      <label class="block text-sm font-medium text-primary-300 mb-2"> 保留目标数量 </label>
      <NumberInput
        v-model="targetKeep"
        :min="1"
        :max="100"
        :step="1"
        class="w-full px-4 py-2 bg-primary-700 border border-primary-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-secondary-500 text-white"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import NumberInput from "./NumberInput.vue";
import RatingSelector from "./RatingSelector.vue";
import { useSessionConfig } from "../composables/useSessionConfig";

const {
  presets,
  selectedPresetId,
  selectedPreset,
  targetKeep,
  rating: filterRating,
} = useSessionConfig();

// 验证是否可以创建会话
const canCreate = computed(() => {
  return (filterRating.value?.length || 0) > 0 && (targetKeep.value || 0) > 0;
});

// 暴露配置值供父组件（HomeView 模态框）读取
defineExpose({
  filterRating,
  targetKeep,
  selectedPreset,
  canCreate,
});
</script>
