<template>
  <!-- 背景与内容容器 -->
  <Transition
    appear
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0 scale-95"
    enter-to-class="opacity-100 scale-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100 scale-100"
    leave-to-class="opacity-0 scale-95"
    @after-leave="emit('afterLeave')"
  >
    <template v-if="visibleModel">
      <div
        class="absolute inset-0 bg-black/95 backdrop-blur-sm select-none flex items-center justify-center"
      >
        <slot></slot>
      </div>
    </template>
  </Transition>
</template>

<script setup lang="ts">
import useHotkey from "@/composables/useHotkey";

const visibleModel = defineModel<boolean>("visible", { required: true });

const emit = defineEmits<(e: "afterLeave") => void>();

const close = () => {
  visibleModel.value = false;
};

// 全屏弹窗在激活时，拦截 Escape 键执行关闭操作
useHotkey("Escape", close, {
  enabled: visibleModel,
  description: "关闭查看器",
  category: "查看器",
});
</script>
