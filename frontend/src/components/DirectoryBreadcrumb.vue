<template>
  <div class="flex flex-wrap items-center gap-1">
    <DirectoryBreadcrumbContent
      :directory-id="directoryId"
      :is-current="isCurrent"
      @rename="emit('rename')"
    />
  </div>
</template>

<script setup lang="ts">
import DirectoryBreadcrumbContent from "./DirectoryBreadcrumbContent.vue";

// #region 组件属性与事件定义
defineProps<{
  directoryId: string;
  isCurrent?: boolean;
}>();

// 透传末级目录的重命名意图，递归渲染的父级节点不会触发该事件
const emit = defineEmits<(e: "rename") => void>();
// #endregion
</script>
