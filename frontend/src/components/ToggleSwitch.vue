<template>
  <!-- #region 开关容器 -->
  <div class="flex items-center gap-2 select-none">
    <!-- 纯文本 label 使用 label 并绑定 for -->
    <label v-if="label" :for="id" class="text-sm text-primary-400 cursor-pointer">
      {{ label }}
    </label>
    <!-- 自定义插槽使用 span 包裹，不绑定 for，避免内部交互元素冲突 -->
    <span v-else-if="$slots.default" class="text-sm text-primary-400">
      <slot></slot>
    </span>

    <div class="relative flex-none">
      <input :id="id" v-model="model" type="checkbox" class="sr-only peer" />
      <!-- 滑动背景与圆形滑块，使用 label 来触发 checkbox 的状态 -->
      <label
        :for="id"
        class="block w-11 h-6 bg-primary-600 peer-focus:ring-2 peer-focus:ring-secondary-500/50 peer-focus:ring-offset-2 peer-focus:ring-offset-primary-900 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-secondary-600 cursor-pointer"
      ></label>
    </div>
  </div>
  <!-- #endregion -->
</template>

<script setup lang="ts">
import { useId } from "vue";

// #region 属性与双向绑定定义
defineProps<{
  /** 开关旁的文字描述 */
  label?: string;
}>();

// 双向绑定开关的开启状态
const model = defineModel<boolean>();
// 自动生成唯一的 id 用于关联 label 和 input
const id = useId();
// #endregion
</script>
