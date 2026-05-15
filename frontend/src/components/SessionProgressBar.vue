<template>
  <div
    class="flex h-1 w-full bg-black/20 overflow-hidden pointer-events-none relative"
  >
    <!-- 已处理的部分：为每个操作显示不同的颜色 -->
    <div
      v-for="(action, index) in queueActions"
      :key="index"
      class="h-full"
      :class="getActionClass(action)"
      :style="{ flex: 1 }"
    ></div>

    <!-- 未处理的部分：半透明背景 -->
    <div
      v-if="remainingSize > 0"
      class="h-full bg-white/10 transition-all duration-300"
      :style="{ flex: remainingSize }"
    ></div>

    <!-- 进度百分比提示（可选，这里保持简洁，仅显示进度条） -->
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { ImageAction, type SessionFragment } from "@/graphql/generated";

const props = defineProps<{
  session: SessionFragment;
}>();

const queueActions = computed(() => props.session.queueActions);
const currentSize = computed(() => props.session.currentSize);
const remainingSize = computed(() =>
  Math.max(0, currentSize.value - queueActions.value.length),
);

/**
 * 根据操作类型获取对应的背景颜色类名
 */
function getActionClass(action: ImageAction): string {
  switch (action) {
    case ImageAction.KEEP:
      return "bg-green-500";
    case ImageAction.SHELVE:
      return "bg-yellow-500";
    case ImageAction.REJECT:
      return "bg-red-500";
    default:
      return "bg-primary-500";
  }
}
</script>
