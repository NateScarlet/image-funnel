<template>
  <!-- 背景遮罩层 -->
  <Transition
    appear
    enter-from-class="opacity-0"
    leave-to-class="opacity-0"
    enter-active-class="transition duration-300 ease-out"
    leave-active-class="transition duration-200 ease-in"
  >
    <template v-if="visibleModel">
      <div
        class="absolute inset-0 bg-black/60 backdrop-blur-md cursor-pointer z-40"
        @click="close()"
      />
    </template>
  </Transition>

  <!-- 内容卡片容器层：移动端在底部，宽屏居中 -->
  <Transition
    appear
    enter-from-class="translate-y-full sm:translate-y-4 sm:opacity-0 sm:scale-95"
    leave-to-class="translate-y-full sm:translate-y-4 sm:opacity-0 sm:scale-95"
    enter-active-class="transition duration-300 ease-out"
    leave-active-class="transition duration-200 ease-in"
    @after-leave="emit('afterLeave')"
  >
    <template v-if="visibleModel">
      <div
        class="absolute inset-x-0 bottom-0 flex flex-col justify-end sm:items-center sm:justify-center short:justify-start overflow-hidden max-h-full pointer-events-none p-0 sm:p-4 z-50"
      >
        <div
          class="relative w-full pointer-events-auto bg-primary-900 border-t border-x sm:border border-primary-700 rounded-t-3xl sm:rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[95dvh] sm:max-h-[85vh] short:max-h-dvh text-left"
          :class="containerClass"
        >
          <slot></slot>
        </div>
      </div>
    </template>
  </Transition>
</template>

<script setup lang="ts">
import useHotkey from "@/composables/useHotkey";

// #region 属性与事件定义
withDefaults(
  defineProps<{
    containerClass?: string | undefined;
  }>(),
  {
    containerClass: () => "sm:max-w-md",
  },
);

const emit = defineEmits<(e: "afterLeave") => void>();

const visibleModel = defineModel<boolean>("visible", { required: true });

const close = () => {
  visibleModel.value = false;
};
// #endregion

// #region 快捷键监听
useHotkey("Escape", close, {
  enabled: visibleModel,
  description: "关闭对话框",
  category: "对话框",
});
// #endregion
</script>
