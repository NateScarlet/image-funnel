<template>
  <div
    class="min-h-screen bg-primary-900 text-primary-100 flex flex-col font-sans"
  >
    <!-- 顶部导航栏 -->
    <header
      class="flex-none bg-primary-900/80 backdrop-blur-md border-b border-primary-700/50 px-4 py-3 sticky top-0 z-30"
    >
      <div class="max-w-[1600px] mx-auto flex items-center gap-3">
        <!-- 路径面包屑与返回上级 -->
        <div class="flex items-center gap-3 overflow-hidden flex-1 min-w-0">
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
            class="flex items-center gap-1.5 px-3 py-1.5 bg-black/20 rounded-lg border border-white/5 overflow-hidden text-sm"
          >
            <!-- 递归路径面包屑 -->
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

          <!-- 上次会话按钮，显示文本与最后更新时间 -->
          <button
            v-if="lastSession"
            class="flex items-center gap-2 px-3 py-1.5 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none text-sm font-medium"
            title="返回最近会话"
            @click="goToLastSession"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiHistory" fill="currentColor" />
            </svg>
            <span>上次会话：{{ formatDate(lastSession.updatedAt) }}</span>
          </button>
        </div>

        <!-- 同级目录导航按钮组，固定在右上角，不可用时显示禁用样式 -->
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
    </header>

    <main
      class="flex-1 w-full max-w-[1600px] mx-auto p-4 md:p-6 space-y-6 overflow-x-hidden"
    >
      <!-- 子目录容器区 -->
      <SubdirectoryGrid
        v-if="subDirectories.length > 0"
        :directories="subDirectories"
        :filter-rating="filterRating"
        @navigate="navigateToDir"
      />

      <!-- 笔记列表容器区 -->
      <section
        class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
      >
        <div
          class="flex items-center justify-between border-b border-primary-700/50 pb-3"
        >
          <h2
            class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
          >
            <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiNoteTextOutline" fill="currentColor" />
            </svg>
            笔记列表 ({{ memos.length }} 个)
          </h2>
          <!-- 显示隐藏笔记切换开关 -->
          <ToggleSwitch v-model="showHiddenMemos" label="显示隐藏笔记" />
        </div>

        <!-- 暂无笔记状态 -->
        <div
          v-if="memos.length === 0"
          class="py-6 text-center text-primary-500 text-sm italic"
        >
          暂无任何笔记
        </div>

        <!-- 笔记列表项 -->
        <div v-else class="space-y-2 max-h-60 overflow-y-auto pr-1">
          <div
            v-for="memoItem in memos"
            :key="memoItem.id"
            class="flex items-center justify-between p-3 rounded-xl bg-primary-800/20 hover:bg-primary-800/60 border border-primary-800/40 hover:border-secondary-500/30 transition-all duration-200 group cursor-pointer"
            @click="editMemo(memoItem)"
          >
            <div class="flex items-center gap-3 min-w-0 flex-1">
              <svg
                class="w-4 h-4 text-primary-400 group-hover:text-secondary-400 transition-colors shrink-0"
                viewBox="0 0 24 24"
              >
                <path :d="mdiNoteTextOutline" fill="currentColor" />
              </svg>
              <!-- 列表显示笔记关联的文件名与正文内容，单行 truncate -->
              <span
                class="text-[10px] text-primary-400 shrink-0 bg-primary-800/60 px-1.5 py-0.5 rounded border border-primary-700/50 font-mono select-none"
              >
                {{ memoItem.title }}
              </span>
              <span
                class="text-sm text-primary-200 group-hover:text-white transition-colors truncate font-medium"
              >
                {{ memoItem.content || "（空白笔记内容，点击编辑）" }}
              </span>
              <!-- 如果是隐藏笔记，显示标记 -->
              <span
                v-if="memoItem.hidden"
                class="px-1.5 py-0.5 text-[10px] bg-red-950/40 border border-red-900/50 text-red-400 rounded-md shrink-0 flex items-center gap-0.5"
                title="此笔记已通过 frontmatter 隐藏"
              >
                <svg class="w-3 h-3" viewBox="0 0 24 24">
                  <path :d="mdiEyeOff" fill="currentColor" />
                </svg>
                已隐藏
              </span>
            </div>
            <div class="flex items-center gap-3 shrink-0">
              <!-- 一键切换隐藏状态按钮 -->
              <button
                class="p-1.5 rounded-lg bg-primary-800/40 hover:bg-primary-700/60 border border-primary-700/50 text-primary-400 hover:text-white transition-all active:scale-95 flex items-center justify-center opacity-0 group-hover:opacity-100"
                :title="memoItem.hidden ? '取消隐藏此笔记' : '隐藏此笔记'"
                @click.stop="toggleMemoHidden(memoItem)"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
                  <path
                    :d="memoItem.hidden ? mdiEye : mdiEyeOff"
                    fill="currentColor"
                  />
                </svg>
              </button>
              <div
                class="text-xs text-primary-500 opacity-0 group-hover:opacity-100 transition-opacity duration-200 select-none flex items-center gap-1"
              >
                <span>编辑</span>
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
                  <path :d="mdiChevronRight" fill="currentColor" />
                </svg>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 图片网格展示与筛选区 -->
      <ImageGrid :directory-id="currentDirectoryId" />
    </main>

    <!-- 备忘录/笔记编辑对话框 -->
    <MemoEditorDialog
      v-if="selectedMemo"
      v-model="isMemoEditorOpen"
      :memo="selectedMemo"
    />
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
  mdiNoteTextOutline,
  mdiEye,
  mdiEyeOff,
  mdiHome,
  mdiHistory,
  mdiOpenInNew,
  mdiArrowUp,
} from "@mdi/js";
import useQuery from "../graphql/utils/useQuery";
import { formatDate } from "@/utils/date";
import useBrowseMemos from "../composables/useBrowseMemos";
import {
  DirectoriesDocument,
  MetaDocument,
  type MemoFragment,
  type BrowseMemosQueryVariables,
  type MemoFiltersInput,
} from "../graphql/generated";
import MemoEditorDialog from "../components/MemoEditorDialog.vue";
import SubdirectoryGrid from "../components/SubdirectoryGrid.vue";
import ToggleSwitch from "../components/ToggleSwitch.vue";
import DirectoryBreadcrumb from "../components/DirectoryBreadcrumb.vue";
import ImageGrid from "../components/ImageGrid.vue";

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
const { filterRating, showHiddenMemos, lastSession } =
  useDirectoryState(currentDirectoryId);

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

// #region 获取并管理实时更新的备忘录/笔记列表
const selectedMemo = ref<MemoFragment | null>(null);
const isMemoEditorOpen = ref(false);

// 构建备忘录查询 variables
const memosVariables = computed<BrowseMemosQueryVariables>(() => {
  const filterBy: MemoFiltersInput = {
    directoryId: [currentDirectoryId.value],
  };
  if (!showHiddenMemos.value) {
    filterBy.hidden = false;
  }
  return {
    id: currentDirectoryId.value,
    filterBy,
    first: 100,
    after: null,
  };
});

// 调用 useBrowseMemos 获取备忘录列表与隐藏切换操作
const { memos, toggleMemoHidden } = useBrowseMemos(memosVariables, {
  loadingCount,
});

function editMemo(memoItem: MemoFragment) {
  selectedMemo.value = memoItem;
  isMemoEditorOpen.value = true;
}
// #endregion

// #region 同级目录切换快捷键

// 切换到上一个同级目录
useHotkey(
  "[",
  () => {
    if (prevSibling.value) navigateToDir(prevSibling.value.id);
  },
  { description: "上一个目录" },
);

// 切换到下一个同级目录
useHotkey(
  "]",
  () => {
    if (nextSibling.value) navigateToDir(nextSibling.value.id);
  },
  { description: "下一个目录" },
);
// #endregion
</script>
