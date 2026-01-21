<template>
  <Teleport :to="rendererEl">
    <div
      v-if="direction"
      class="absolute inset-x-0 bottom-0 h-1/2 flex items-center justify-center pointer-events-none transition-opacity duration-200"
      :class="{
        'bg-red-600/30': direction === 'DOWN',
        'bg-yellow-600/30': direction === 'UP',
        'bg-green-600/30': direction === 'LEFT',
        'bg-slate-600/30': direction === 'RIGHT',
      }"
    >
      <div class="text-6xl font-bold text-white drop-shadow-lg">
        <span v-if="direction === 'DOWN'">↓ 排除</span>
        <span v-else-if="direction === 'UP'">↑ 稍后再看</span>
        <span v-else-if="direction === 'RIGHT'">← 撤销</span>
        <span v-else-if="direction === 'LEFT'">→ 保留</span>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import type { RendererElement } from "vue";

interface Props {
  direction: "UP" | "DOWN" | "LEFT" | "RIGHT" | null;
  rendererEl: RendererElement;
}

defineProps<Props>();
</script>
