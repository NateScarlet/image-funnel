<template>
  <!-- 背景遮罩层 -->
  <Transition
    appear
    enter-from-class="opacity-0"
    leave-to-class="opacity-0"
    enter-active-class="transition-all duration-300 ease-in-out"
    leave-active-class="transition-all duration-300 ease-in-out"
  >
    <div
      v-if="visibleModel"
      class="absolute inset-0 bg-black/75 backdrop-blur-xs cursor-pointer z-40"
      @click="close()"
    ></div>
  </Transition>

  <!-- 抽屉容器层 -->
  <Transition
    appear
    enter-from-class="transform translate-x-full"
    leave-to-class="transform translate-x-full"
    enter-active-class="transition-all duration-300 ease-in-out"
    leave-active-class="transition-all duration-300 ease-in-out"
    @after-leave="emit('afterLeave')"
  >
    <div
      v-if="visibleModel"
      class="absolute inset-y-0 right-0 lg:max-w-[90vw] z-50 pointer-events-auto"
      :class="containerClass"
    >
      <!-- 移动端顶部关闭按钮 -->
      <div class="w-full sticky p-1 top-0 text-right lg:hidden">
        <button
          class="text-primary-400 hover:text-primary-200 transition-colors p-1.5 rounded-lg hover:bg-primary-700/50 cursor-pointer"
          type="button"
          @click="close()"
        >
          <svg class="inline-block fill-current w-6 h-6" viewBox="0 0 24 24">
            <path :d="mdiClose" fill="currentColor"></path>
          </svg>
        </button>
      </div>
      <slot></slot>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import useHotkey from "@/composables/useHotkey";
import { mdiClose } from "@mdi/js";

// #region 属性与事件定义
withDefaults(
  defineProps<{
    visible?: boolean;
    containerClass?: string;
  }>(),
  {
    containerClass: () =>
      "bg-primary-800 border-l border-primary-700 p-6 overflow-y-auto overflow-x-hidden shadow-2xl flex flex-col h-full w-full max-w-md text-left",
  },
);

const emit = defineEmits<{
  (e: "afterLeave"): void;
  (e: "update:visible", v: boolean): void;
}>();

const visibleModel = defineModel<boolean>("visible", { required: true });

const close = () => {
  visibleModel.value = false;
};
// #endregion

// #region 快捷键监听
useHotkey("Escape", close, {
  enabled: visibleModel,
  description: "关闭抽屉",
  category: "抽屉",
});
// #endregion
</script>
