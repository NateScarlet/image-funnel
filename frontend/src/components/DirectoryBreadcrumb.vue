<template>
  <div class="flex flex-wrap items-center gap-1">
    <!-- #region 递归渲染父级目录 -->
    <template v-if="!isRoot && parentID">
      <DirectoryBreadcrumb
        :directory-id="parentID"
        @navigate="emit('navigate', $event)"
      />
      <span class="text-primary-600 select-none mx-0.5">/</span>
    </template>
    <!-- #endregion -->

    <!-- #region 渲染当前目录节点 -->
    <button
      class="px-1 py-0.5 rounded transition-all flex items-center gap-1 shrink-0"
      :class="[
        isCurrent
          ? 'text-white font-semibold'
          : 'text-primary-300 hover:text-white hover:bg-white/10 cursor-pointer',
      ]"
      :disabled="loading"
      :title="myDirectory?.relPath || '加载中...'"
      @click="myDirectory && emit('navigate', myDirectory.id)"
    >
      <!-- 根目录展示文件夹图标与 Root 标识 -->
      <template v-if="isRoot">
        <svg class="w-4 h-4 shrink-0 text-primary-400" viewBox="0 0 24 24">
          <path :d="mdiFolderOpen" fill="currentColor" />
        </svg>
        <span>Root</span>
      </template>
      <!-- 子目录展示最后一级目录名 -->
      <template v-else>
        {{ displayName }}
      </template>
    </button>
    <!-- #endregion -->
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { mdiFolderOpen } from "@mdi/js";
import useQuery from "../graphql/utils/useQuery";
import { DirectoriesDocument } from "../graphql/generated";

// #region 组件属性与事件定义
const props = defineProps<{
  directoryId: string;
  isCurrent?: boolean;
}>();

const emit = defineEmits<(e: "navigate", id: string) => void>();
// #endregion

// #region 目录数据查询与解析
// 仅在 directoryId 有效时发起 GraphQL 查询
const { data } = useQuery(DirectoriesDocument, {
  variables: () => {
    if (!props.directoryId) return undefined;
    return {
      id: props.directoryId,
    };
  },
});

// 计算当前加载中的状态
const loading = computed(() => {
  // 根据 apollo client 的查询结果状态来确定是否加载中
  return data.value === undefined;
});

// 提取当前级目录对象
const myDirectory = computed(() => {
  const node = data.value?.node;
  return node?.__typename === "Directory" ? node : undefined;
});

// 是否是相对路径根目录
const isRoot = computed(() => {
  return myDirectory.value?.root ?? false;
});

// 父级目录 ID，用于上级面包屑递归
const parentID = computed(() => {
  return myDirectory.value?.parentId || undefined;
});

// 解析显示名称，未就绪时显示为省略号
const displayName = computed(() => {
  if (!myDirectory.value) {
    return "...";
  }
  return getDirName(myDirectory.value.relPath);
});

// 从相对路径中提取最后一级目录的名称
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
</script>
