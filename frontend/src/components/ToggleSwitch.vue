<template>
  <!-- #region 开关容器 -->
  <label class="flex items-center gap-2 cursor-pointer select-none">
    <slot>
      <span class="text-sm text-primary-400">{{ label }}</span>
    </slot>
    <div class="relative flex-none">
      <input :id="id" v-model="model" type="checkbox" class="sr-only peer" />
      <!-- 滑动背景与圆形滑块 -->
      <div
        class="w-11 h-6 bg-primary-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-secondary-600"
      ></div>
    </div>
  </label>
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
