<template>
  <div
    class="h-1 w-full bg-black/20 pointer-events-none relative overflow-hidden"
  >
    <!-- 背景层 -->
    <div
      class="absolute inset-0 transition-all duration-700"
      :class="isTargetMet ? 'bg-green-500/30' : 'bg-white/10'"
    ></div>
    <!-- 操作历史层 -->
    <TransitionGroup
      tag="div"
      class="absolute inset-0 grid"
      :style="{ gridTemplateColumns: `repeat(${currentSize}, 1fr)` }"
      :enter-from-class="isBatchChange ? 'opacity-0' : 'scale-x-0 opacity-0'"
      :leave-to-class="isBatchChange ? 'opacity-0' : 'scale-x-0 opacity-0'"
    >
      <div
        v-for="(action, index) in queueActions"
        :key="index"
        class="h-full transition-all duration-300 ease-in-out origin-left"
        :class="getActionClass(action)"
      ></div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ImageAction, type SessionFragment } from "@/graphql/generated";

const props = defineProps<{
  session: SessionFragment;
}>();

const queueActions = computed(() => props.session.queueActions);
const currentSize = computed(() => props.session.currentSize);
const isTargetMet = computed(
  () => props.session.stats.kept >= props.session.targetKeep,
);

const isBatchChange = ref(false);
watch(
  () => queueActions.value.length,
  (newLen, oldLen) => {
    isBatchChange.value = Math.abs(newLen - (oldLen ?? 0)) > 1;
  },
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
