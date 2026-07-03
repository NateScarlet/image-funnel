<template>
  <div class="flex flex-wrap items-center gap-1">
    <DirectoryBreadcrumbContent :directory-id="directoryId" :is-current="isCurrent" />
  </div>
</template>

<script setup lang="ts">
import DirectoryBreadcrumbContent from "./DirectoryBreadcrumbContent.vue";

// #region 组件属性与事件定义
defineProps<{
  directoryId: string;
  isCurrent?: boolean;
}>();
// #endregion
</script>
