<template>
  <div
    class="min-h-screen bg-primary-900 text-primary-100 flex flex-col font-sans"
  >
    <!-- 顶部导航栏 -->
    <header
      class="flex-none bg-primary-900/80 backdrop-blur-md border-b border-primary-700/50 px-4 py-3 sticky top-0 z-10"
    >
      <!-- 大屏布局：一行显示所有内容 -->
      <div class="hidden md:flex max-w-400 mx-auto items-center gap-3">
        <!-- 路径面包屑与返回上级 -->
        <div class="flex items-center gap-3 flex-1 min-w-0">
          <!-- 返回主页 -->
          <button
            class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none flex items-center justify-center cursor-pointer"
            title="返回主页"
            @click="navigateToHome"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiHome" fill="currentColor" />
            </svg>
          </button>

          <!-- 返回上一级 -->
          <button
            :disabled="!canGoToParent"
            class="p-2 rounded-lg border transition-all flex-none flex items-center justify-center"
            :class="
              canGoToParent
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white cursor-pointer'
                : 'bg-primary-800/40 border-primary-700/50 text-primary-500 cursor-not-allowed'
            "
            title="返回上一级"
            @click="goToParent"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiArrowUp" fill="currentColor" />
            </svg>
          </button>

          <!-- 磨砂面包屑路径 -->
          <div
            class="flex flex-wrap items-center gap-2 px-3 py-1 bg-black/20 rounded-lg border border-white/5 text-sm break-all"
          >
            <DirectoryBreadcrumb
              v-if="currentDirectoryId"
              :directory-id="currentDirectoryId"
              :is-current="true"
              @navigate="navigateToDir"
            />
          </div>

          <!-- 打开当前目录按钮 -->
          <button
            v-if="fullDirectoryPath"
            class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none flex items-center justify-center"
            title="在资源管理器中打开当前目录"
            @click="revealInExplorer(fullDirectoryPath)"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiOpenInNew" fill="currentColor" />
            </svg>
          </button>

          <!-- 上次会话按钮 -->
          <button
            v-if="lastSession"
            class="flex items-center gap-2 px-3 py-1 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none text-sm font-medium"
            title="返回最近会话"
            @click="goToLastSession"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiHistory" fill="currentColor" />
            </svg>
            <span>上次会话：{{ formatDate(lastSession.updatedAt) }}</span>
          </button>
        </div>

        <!-- 同级目录导航按钮组 -->
        <div class="flex items-center gap-1 flex-none">
          <button
            :disabled="!prevSibling"
            class="p-2 rounded-lg border transition-all flex items-center justify-center"
            :class="
              prevSibling
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40'
            "
            :title="
              prevSibling
                ? `上一个目录 ([): ${getDirName(prevSibling.relPath)}`
                : '没有上一个目录'
            "
            @click="prevSibling && navigateToDir(prevSibling.id)"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiChevronLeft" fill="currentColor" />
            </svg>
          </button>
          <button
            :disabled="!nextSibling"
            class="p-2 rounded-lg border transition-all flex items-center justify-center"
            :class="
              nextSibling
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40'
            "
            :title="
              nextSibling
                ? `下一个目录 (]): ${getDirName(nextSibling.relPath)}`
                : '没有下一个目录'
            "
            @click="nextSibling && navigateToDir(nextSibling.id)"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiChevronRight" fill="currentColor" />
            </svg>
          </button>
        </div>
      </div>

      <!-- 小屏布局：两行显示 -->
      <div class="md:hidden max-w-400 mx-auto flex flex-col gap-3">
        <!-- 第一行：所有按钮 -->
        <div class="flex flex-wrap items-center justify-between gap-2">
          <!-- 左侧按钮组 -->
          <div class="flex items-center gap-2">
            <!-- 返回主页 -->
            <button
              class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex items-center justify-center cursor-pointer"
              title="返回主页"
              @click="navigateToHome"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiHome" fill="currentColor" />
              </svg>
            </button>

            <!-- 返回上一级 -->
            <button
              :disabled="!canGoToParent"
              class="p-2 rounded-lg border transition-all flex items-center justify-center"
              :class="
                canGoToParent
                  ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white cursor-pointer'
                  : 'bg-primary-800/40 border-primary-700/50 text-primary-500 cursor-not-allowed'
              "
              title="返回上一级"
              @click="goToParent"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiArrowUp" fill="currentColor" />
              </svg>
            </button>

            <!-- 上次会话按钮 - 仅显示图标 -->
            <button
              v-if="lastSession"
              class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex items-center justify-center"
              title="返回最近会话"
              @click="goToLastSession"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path :d="mdiHistory" fill="currentColor" />
              </svg>
            </button>
          </div>

          <!-- 右侧同级目录导航按钮组 -->
          <div class="flex items-center gap-1">
            <button
              :disabled="!prevSibling"
              class="p-2 rounded-lg border transition-all flex items-center justify-center"
              :class="
                prevSibling
                  ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                  : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40'
              "
              :title="
                prevSibling
                  ? `上一个目录 ([): ${getDirName(prevSibling.relPath)}`
                  : '没有上一个目录'
              "
              @click="prevSibling && navigateToDir(prevSibling.id)"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiChevronLeft" fill="currentColor" />
              </svg>
            </button>
            <button
              :disabled="!nextSibling"
              class="p-2 rounded-lg border transition-all flex items-center justify-center"
              :class="
                nextSibling
                  ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                  : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40'
              "
              :title="
                nextSibling
                  ? `下一个目录 (]): ${getDirName(nextSibling.relPath)}`
                  : '没有下一个目录'
              "
              @click="nextSibling && navigateToDir(nextSibling.id)"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiChevronRight" fill="currentColor" />
              </svg>
            </button>
          </div>
        </div>

        <!-- 第二行：面包屑路径 -->
        <div
          class="flex flex-wrap items-center gap-2 px-3 py-1 bg-black/20 rounded-lg border border-white/5 text-sm break-all"
        >
          <DirectoryBreadcrumb
            v-if="currentDirectoryId"
            :directory-id="currentDirectoryId"
            :is-current="true"
            @navigate="navigateToDir"
          />
        </div>
      </div>
    </header>

    <main
      class="flex-1 w-full max-w-400 mx-auto p-4 md:p-6 space-y-6 overflow-x-hidden"
    >
      <!-- 子目录容器区 -->
      <SubdirectoryGrid
        v-if="subDirectories.length > 0"
        :directories="subDirectories"
        :filter-rating="filterRating"
        @navigate="navigateToDir"
      />

      <!-- 笔记列表容器区 -->
      <MemoList :directory-id="currentDirectoryId" />

      <!-- 图片网格展示与筛选区 -->
      <ImageGrid :directory-id="currentDirectoryId" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import useHotkey from "@/composables/useHotkey";
import { useDirectoryState } from "../composables/useDirectoryState";
import { useRoute, useRouter } from "vue-router";
import {
  mdiChevronLeft,
  mdiChevronRight,
  mdiHome,
  mdiHistory,
  mdiOpenInNew,
  mdiArrowUp,
} from "@mdi/js";
import useQuery from "../graphql/utils/useQuery";
import { formatDate } from "@/utils/date";
import { DirectoriesDocument, MetaDocument } from "../graphql/generated";
import SubdirectoryGrid from "../components/SubdirectoryGrid.vue";
import DirectoryBreadcrumb from "../components/DirectoryBreadcrumb.vue";
import ImageGrid from "../components/ImageGrid.vue";
import MemoList from "../components/MemoList.vue";

// #region 路由参数与导航
const route = useRoute();
const router = useRouter();

// 目录ID，默认从路由 query 中获取，否则为空字符串（即代表根目录）
const currentDirectoryId = computed(() => (route.query.dir as string) || "");

function navigateToDir(id: string) {
  router.push({
    path: "/browse",
    query: id ? { dir: id } : {},
  });
}

function navigateToHome() {
  router.push("/");
}

function goToLastSession() {
  if (lastSession.value) {
    router.push({
      name: "session",
      params: { id: lastSession.value.id },
    });
  }
}
// #endregion

// #region 过滤器与多选状态
const { filterRating, lastSession } = useDirectoryState(currentDirectoryId);

const subDirectories = computed(() => {
  return currentDirectory.value?.directories || [];
});

// 判断当前是否可以返回上一级目录（存在当前目录且不是根目录）
const canGoToParent = computed(() => {
  return !!currentDirectory.value && !currentDirectory.value.root;
});

function goToParent() {
  if (currentDirectory.value?.parentId) {
    navigateToDir(currentDirectory.value.parentId);
  } else {
    navigateToDir("");
  }
}
// #endregion

// #region 目录与子目录查询
const loadingCount = ref(0);

const { data: directoriesData } = useQuery(DirectoriesDocument, {
  variables: () => ({
    id: currentDirectoryId.value,
  }),
  loadingCount,
});

const currentDirectory = computed(() => {
  const node = directoriesData.value?.node;
  return node?.__typename === "Directory" ? node : undefined;
});

// #region 同级目录切换逻辑
import useFilteredDirectories from "@/composables/useFilteredDirectories";

// 推导父目录 ID。若当前已经在根目录，则不加载父目录
const parentDirectoryId = computed(() => {
  if (!currentDirectoryId.value) return undefined;
  return currentDirectory.value?.parentId || "";
});

// 构建查询父目录下所有子目录的 GraphQL 变量，若 parentDirectoryId 未就绪则跳过查询
const parentDirectoriesVariables = computed(() => {
  if (parentDirectoryId.value === undefined) {
    return undefined;
  }
  return {
    id: parentDirectoryId.value,
  };
});

const { data: parentDirectoriesData } = useQuery(DirectoriesDocument, {
  variables: parentDirectoriesVariables,
});

// 父目录下的所有原始子目录列表
const parentSubDirectories = computed(() => {
  const node = parentDirectoriesData.value?.node;
  return node?.__typename === "Directory" ? node.directories || [] : [];
});

// 排序后的同级目录
const { sortedDirectories: sortedSiblings } =
  useFilteredDirectories(parentSubDirectories);

// 当前目录在排序后同级目录中的索引位置
const currentSiblingIndex = computed(() => {
  return sortedSiblings.value.findIndex(
    (dir) => dir.id === currentDirectoryId.value,
  );
});

// 上一个同级目录
const prevSibling = computed(() => {
  const idx = currentSiblingIndex.value;
  if (idx > 0) {
    return sortedSiblings.value[idx - 1];
  }
  return undefined;
});

// 下一个同级目录
const nextSibling = computed(() => {
  const idx = currentSiblingIndex.value;
  if (idx !== -1 && idx < sortedSiblings.value.length - 1) {
    return sortedSiblings.value[idx + 1];
  }
  return undefined;
});

// 提取目录名用于 UI 展示
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
// #endregion

// #region 绝对物理路径与资源管理器打开
import { useOpenDir } from "../composables/useOpenDir";
const { revealInExplorer } = useOpenDir();

const metaLoadingCount = ref(0);
const { data: metaData } = useQuery(MetaDocument, {
  loadingCount: metaLoadingCount,
});

const fullDirectoryPath = computed(() => {
  const rootPath = metaData.value?.meta?.rootAbsPath;
  const relPath = currentDirectory.value?.relPath;
  if (!rootPath) {
    return "";
  }
  if (!relPath) {
    return rootPath;
  }
  if (rootPath.includes("\\")) {
    return rootPath + "\\" + relPath.replace(/\//g, "\\");
  }
  return rootPath + "/" + relPath.replace(/\\/g, "/");
});
// #endregion

// #region 同级目录切换快捷键

// 返回上一级目录
useHotkey(
  "backspace",
  () => {
    if (canGoToParent.value) {
      goToParent();
    }
  },
  { description: "返回上一级目录", category: "目录导航" },
);

// 切换到上一个同级目录
useHotkey(
  "[",
  () => {
    if (prevSibling.value) navigateToDir(prevSibling.value.id);
  },
  { description: "上一个目录", category: "目录导航" },
);

// 切换到下一个同级目录
useHotkey(
  "]",
  () => {
    if (nextSibling.value) navigateToDir(nextSibling.value.id);
  },
  { description: "下一个目录", category: "目录导航" },
);
// #endregion
</script>
