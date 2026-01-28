<template>
  <div
    class="flex items-center justify-center pointer-events-none transition-opacity duration-200"
    :class="{
      'bg-red-600/30': direction === 'DOWN',
      'bg-yellow-600/30': direction === 'UP',
      'bg-green-600/30': direction === 'LEFT',
      'bg-primary-600/30': direction === 'RIGHT',
    }"
  >
    <div class="text-6xl font-bold text-white drop-shadow-lg">
      <span v-if="direction === 'DOWN'">↓ 排除</span>
      <span v-else-if="direction === 'UP'">↑ 搁置</span>
      <span v-else-if="direction === 'RIGHT'">← 撤销</span>
      <span v-else-if="direction === 'LEFT'">→ 保留</span>
    </div>
  </div>
</template>

<script setup lang="ts">
const { direction } = defineProps<{
  direction: "UP" | "DOWN" | "LEFT" | "RIGHT";
}>();
</script>
