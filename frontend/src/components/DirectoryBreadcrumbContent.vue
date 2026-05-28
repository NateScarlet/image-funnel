<template>
  <!-- #region 递归渲染父级目录 -->
  <template v-if="parentID">
    <DirectoryBreadcrumbContent
      :directory-id="parentID"
      @navigate="emit('navigate', $event)"
    />
    <!-- 仅当当前节点不是 Root 且父节点不是 Root 时渲染分隔符 -->
    <span
      v-if="needsSeparatorBefore"
      class="text-primary-600 select-none mx-0.5"
      >/</span
    >
  </template>
  <!-- #endregion -->

  <!-- #region 渲染当前目录节点 -->
  <!-- 根目录渲染为文件夹图标按钮，后面紧跟分隔符 -->
  <template v-if="isRoot">
    <button
      class="px-1 py-0.5 rounded transition-all flex items-center shrink-0"
      :class="[
        isCurrent
          ? 'pointer-events-none text-primary-400'
          : 'text-primary-300 hover:text-white hover:bg-white/10 cursor-pointer',
      ]"
      :disabled="loading"
      title="Root"
      @click="myDirectory && emit('navigate', myDirectory.id)"
    >
      <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24">
        <path :d="mdiFolderOpen" fill="currentColor" />
      </svg>
    </button>
    <span class="text-primary-600 select-none mx-0.5">/</span>
  </template>
  <!-- 子目录展示最后一级目录名 -->
  <button
    v-else
    class="px-1 py-0.5 rounded transition-all flex items-center gap-1 shrink-0 select-all"
    :class="[
      isCurrent
        ? 'text-white font-semibold'
        : 'text-primary-300 hover:text-white hover:bg-white/10 cursor-pointer',
    ]"
    :disabled="loading"
    :title="myDirectory?.relPath || '加载中...'"
    @click="myDirectory && emit('navigate', myDirectory.id)"
  >
    {{ displayName }}
  </button>
  <!-- #endregion -->
</template>

<script setup lang="ts">
import { computed } from "vue";
import { mdiFolderOpen } from "@mdi/js";
import useDirectories from "@/composables/useDirectories";

// #region 组件属性与事件定义
const props = defineProps<{
  directoryId: string;
  isCurrent?: boolean;
}>();

const emit = defineEmits<(e: "navigate", id: string) => void>();
// #endregion

// #region 目录数据查询与解析
// 查询当前目录自身元数据
const { currentDirectory: myDirectory } = useDirectories(() => ({
  id: props.directoryId,
  first: 0,
}));

// 计算当前加载中的状态
const loading = computed(() => {
  return myDirectory.value === undefined;
});

// 是否是相对路径根目录
const isRoot = computed(() => {
  return myDirectory.value?.root ?? false;
});

// 父级目录 ID，用于上级面包屑递归
const parentID = computed(() => {
  return myDirectory.value?.parentId || undefined;
});

// 是否需要在当前目录前渲染分隔符
// 如果是根目录，或者它的父级是根目录（即相对路径不包含斜杠/反斜杠），则不需要分隔符
const needsSeparatorBefore = computed(() => {
  if (isRoot.value) return false;
  const path = myDirectory.value?.relPath || "";
  return path.includes("/") || path.includes("\\");
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
