<template>
  <div class="min-h-screen bg-primary-900 text-primary-100 flex flex-col font-sans">
    <!-- 顶部导航栏 -->
    <header
      class="flex-none bg-primary-900/80 backdrop-blur-md border-b border-primary-700/50 px-4 py-3 sticky top-0 z-10 relative"
    >
      <!-- 大屏布局：一行显示所有内容 -->
      <div class="hidden md:flex max-w-400 mx-auto items-center gap-3">
        <!-- 路径面包屑与返回上级 -->
        <div class="flex items-center gap-3 flex-1 min-w-0">
          <!-- 返回主页 -->
          <RouterLink
            :to="homeTo"
            class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none flex items-center justify-center cursor-pointer no-underline"
            title="返回主页"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiHome" fill="currentColor" />
            </svg>
          </RouterLink>

          <!-- 返回上一级 -->
          <RouterLink
            :to="parentTo"
            class="p-2 rounded-lg border transition-all flex-none flex items-center justify-center no-underline"
            :class="
              canGoToParent
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white cursor-pointer'
                : 'bg-primary-800/40 border-primary-700/50 text-primary-500 cursor-not-allowed pointer-events-none'
            "
            title="返回上一级"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiArrowUp" fill="currentColor" />
            </svg>
          </RouterLink>

          <!-- 磨砂面包屑路径 -->
          <div
            class="flex flex-wrap items-center gap-2 px-3 py-1 bg-black/20 rounded-lg border border-white/5 text-sm break-all"
          >
            <DirectoryBreadcrumb
              v-if="currentDirectoryId"
              :directory-id="currentDirectoryId"
              :is-current="true"
              @rename="renameDirectoryDialog.open"
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
          <RouterLink
            v-if="lastSession"
            :to="{
              name: 'session',
              params: { id: lastSession.id },
            }"
            class="flex items-center gap-2 px-3 py-1 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none text-sm font-medium no-underline"
            title="返回最近会话"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiHistory" fill="currentColor" />
            </svg>
            <span>上次会话：{{ formatDate(lastSession.updatedAt) }}</span>
          </RouterLink>
        </div>

        <!-- 同级目录导航按钮组 -->
        <div class="flex items-center gap-1 flex-none">
          <RouterLink
            :to="prevSiblingTo"
            class="p-2 rounded-lg border transition-all flex items-center justify-center no-underline"
            :class="
              prevSibling
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40 pointer-events-none'
            "
            :title="
              prevSibling ? `上一个目录 ([): ${getDirName(prevSibling.relPath)}` : '没有上一个目录'
            "
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiChevronLeft" fill="currentColor" />
            </svg>
          </RouterLink>
          <RouterLink
            :to="nextSiblingTo"
            class="p-2 rounded-lg border transition-all flex items-center justify-center no-underline"
            :class="
              nextSibling
                ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40 pointer-events-none'
            "
            :title="
              nextSibling ? `下一个目录 (]): ${getDirName(nextSibling.relPath)}` : '没有下一个目录'
            "
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiChevronRight" fill="currentColor" />
            </svg>
          </RouterLink>
        </div>

        <!-- 回收站与设备管理 -->
        <div class="flex items-center gap-3 flex-none">
          <NotificationCenterButton
            button-class="relative flex items-center gap-2 rounded-lg border border-primary-700 bg-primary-800/80 px-4 py-2 text-sm font-medium text-primary-300 transition-all hover:border-primary-600 hover:bg-primary-700 hover:text-white active:scale-95 cursor-pointer select-none"
          >
            <span class="hidden sm:inline">通知</span>
          </NotificationCenterButton>
          <TrashHistoryButton />
          <DeviceManagerButton />
        </div>
      </div>

      <!-- 小屏布局：两行显示 -->
      <div class="md:hidden max-w-400 mx-auto flex flex-col gap-3">
        <!-- 第一行：所有按钮 -->
        <div class="flex flex-wrap items-center justify-between gap-2">
          <!-- 左侧按钮组 -->
          <div class="flex items-center gap-2">
            <!-- 返回主页 -->
            <RouterLink
              :to="homeTo"
              class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex items-center justify-center cursor-pointer no-underline"
              title="返回主页"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiHome" fill="currentColor" />
              </svg>
            </RouterLink>

            <!-- 返回上一级 -->
            <RouterLink
              :to="parentTo"
              class="p-2 rounded-lg border transition-all flex items-center justify-center no-underline"
              :class="
                canGoToParent
                  ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white cursor-pointer'
                  : 'bg-primary-800/40 border-primary-700/50 text-primary-500 cursor-not-allowed pointer-events-none'
              "
              title="返回上一级"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiArrowUp" fill="currentColor" />
              </svg>
            </RouterLink>

            <!-- 上次会话按钮 - 仅显示图标 -->
            <RouterLink
              v-if="lastSession"
              :to="{
                name: 'session',
                params: { id: lastSession.id },
              }"
              class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex items-center justify-center no-underline"
              title="返回最近会话"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path :d="mdiHistory" fill="currentColor" />
              </svg>
            </RouterLink>
          </div>

          <!-- 右侧同级目录导航按钮组、回收站与更多操作 -->
          <div class="flex items-center gap-2">
            <TrashHistoryButton />

            <div class="flex items-center gap-1">
              <RouterLink
                :to="prevSiblingTo"
                class="p-2 rounded-lg border transition-all flex items-center justify-center no-underline"
                :class="
                  prevSibling
                    ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                    : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40 pointer-events-none'
                "
                :title="
                  prevSibling
                    ? `上一个目录 ([): ${getDirName(prevSibling.relPath)}`
                    : '没有上一个目录'
                "
              >
                <svg class="w-5 h-5" viewBox="0 0 24 24">
                  <path :d="mdiChevronLeft" fill="currentColor" />
                </svg>
              </RouterLink>
              <RouterLink
                :to="nextSiblingTo"
                class="p-2 rounded-lg border transition-all flex items-center justify-center no-underline"
                :class="
                  nextSibling
                    ? 'bg-primary-800 hover:bg-primary-700 border-primary-700 hover:border-primary-600 text-primary-300 hover:text-white'
                    : 'bg-primary-900 border-primary-800 text-primary-700 cursor-not-allowed opacity-40 pointer-events-none'
                "
                :title="
                  nextSibling
                    ? `下一个目录 (]): ${getDirName(nextSibling.relPath)}`
                    : '没有下一个目录'
                "
              >
                <svg class="w-5 h-5" viewBox="0 0 24 24">
                  <path :d="mdiChevronRight" fill="currentColor" />
                </svg>
              </RouterLink>
            </div>

            <!-- 更多操作按钮 -->
            <button
              class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex items-center justify-center cursor-pointer relative"
              title="更多操作"
              @click="moreMenuDialog.open"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiMenu" fill="currentColor" />
              </svg>
              <span
                v-if="pairingRequests.length > 0"
                class="absolute -right-1 -top-1 flex h-2.5 w-2.5 rounded-full bg-red-500 animate-pulse"
              ></span>
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
            @rename="renameDirectoryDialog.open"
          />
        </div>
      </div>
    </header>

    <main class="flex-1 w-full max-w-400 mx-auto p-4 md:p-6 space-y-6 overflow-x-hidden">
      <!-- 子目录容器区 -->
      <SubdirectoryGrid :directory-id="currentDirectoryId" :filter-rating="filterRating" />

      <!-- 笔记列表容器区 -->
      <NoteList :directory-id="currentDirectoryId" />

      <!-- 图片网格展示与筛选区 -->
      <ImageGrid :directory-id="currentDirectoryId" />
    </main>

    <moreMenuDialog.component container-class="p-6 sm:max-w-sm">
      <div class="space-y-3">
        <NotificationCenterButton
          button-class="relative w-full py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-all flex items-center gap-3 text-primary-200 hover:text-white active:scale-95 cursor-pointer select-none"
          @click="moreMenuDialog.close()"
        >
          <span class="flex-1 text-left">通知中心</span>
        </NotificationCenterButton>
        <DeviceManagerButton variant="menu-item" @click="moreMenuDialog.close()" />

        <!-- 在资源管理器中打开当前目录 -->
        <button
          v-if="fullDirectoryPath"
          class="w-full py-3 px-4 bg-primary-700 hover:bg-primary-600 rounded-lg font-medium transition-colors flex items-center gap-3 text-primary-200 hover:text-white cursor-pointer"
          @click="
            revealInExplorer(fullDirectoryPath);
            moreMenuDialog.close();
          "
        >
          <svg class="w-5 h-5 shrink-0" viewBox="0 0 24 24">
            <path :d="mdiOpenInNew" fill="currentColor" />
          </svg>
          <span class="text-left">在资源管理器中打开</span>
        </button>
      </div>
    </moreMenuDialog.component>

    <renameDirectoryDialog.component
      v-if="currentDirectory && !currentDirectory.root"
      container-class="sm:max-w-md p-6"
    >
      <RenameDirectoryForm
        ref="renameDirectoryFormRef"
        :directory="currentDirectory"
        @close="renameDirectoryDialog.close()"
        @renamed="handleDirectoryRenamed"
      />
    </renameDirectoryDialog.component>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, shallowRef, useTemplateRef, watch, watchEffect } from "vue";
import { useHotkeys } from "@/composables/useHotkeys";
import { useDirectoryState } from "../composables/useDirectoryState";
import { useRoute, useRouter } from "vue-router";
import {
  mdiChevronLeft,
  mdiChevronRight,
  mdiHome,
  mdiHistory,
  mdiOpenInNew,
  mdiArrowUp,
  mdiMenu,
} from "@mdi/js";
import useAsyncTask from "@/composables/useAsyncTask";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import { formatDate } from "@/utils/date";
import type { DirectoryFragment } from "../graphql/generated";
import useDirectories, {
  maxUnratedCount,
  showLargeUnrated,
  showUncompletedDirectories,
} from "@/composables/useDirectories";
import { evaluateDirectoryVisibility } from "@/utils/directoryVisibility";
import SubdirectoryGrid from "../components/SubdirectoryGrid.vue";
import DirectoryBreadcrumb from "../components/DirectoryBreadcrumb.vue";
import ImageGrid from "../components/ImageGrid.vue";
import NoteList from "../components/NoteList.vue";
import RenameDirectoryForm from "../components/RenameDirectoryForm.vue";
import useModalDialog from "@/composables/useModalDialog";
import DeviceManagerButton from "../components/DeviceManagerButton.vue";
import NotificationCenterButton from "../components/NotificationCenterButton.vue";
import TrashHistoryButton from "../components/TrashHistoryButton.vue";
import { useDevices } from "@/composables/useDevices.ts";
import { useDirectoryStats } from "@/composables/domain/useDirectoryBrowse";
import {
  createDirectoryCursor,
  findNextDirectory,
  findPreviousDirectory,
  sortDirectoryOrder,
} from "@/utils/directoryOrder";
import { getBrowseCursor, setBrowseCursor } from "@/composables/useDirectoryCursor";

const { pairingRequests } = useDevices();
const moreMenuDialog = useModalDialog();

// #region 路由参数与导航
const route = useRoute();
const router = useRouter();

// 目录ID，默认从路由 query 中获取，否则为空字符串（即代表根目录）
const currentDirectoryId = computed(() => (route.query.dir as string) || "");

// 返回主页时携带当前目录，使主页自动选中来源目录
const homeTo = computed(() => ({
  path: "/",
  query: currentDirectoryId.value ? { dir: currentDirectoryId.value } : {},
}));

const parentTo = computed(() => {
  if (currentDirectory.value?.parentId) {
    return { path: "/browse", query: { dir: currentDirectory.value.parentId } };
  }
  return { path: "/browse", query: {} };
});

const prevSiblingTo = computed(() => {
  if (prevSibling.value) {
    return { path: "/browse", query: { dir: prevSibling.value.id } };
  }
  return {};
});

const nextSiblingTo = computed(() => {
  if (nextSibling.value) {
    return { path: "/browse", query: { dir: nextSibling.value.id } };
  }
  return {};
});

function navigateToDir(id: string) {
  router.push({
    path: "/browse",
    query: id ? { dir: id } : {},
  });
}
// #endregion

// #region 目录重命名
const renameDirectoryFormRef =
  useTemplateRef<InstanceType<typeof RenameDirectoryForm>>("renameDirectoryFormRef");

const renameDirectoryDialog = useModalDialog({
  onDidOpen() {
    // 弹窗内容此时尚未挂载，等一次渲染后再回填输入框
    nextTick(() => renameDirectoryFormRef.value?.reset());
  },
});

// 目录 ID 由相对路径派生，重命名后 ID 会变化，因此跳转到新 ID 保持在原处浏览
function handleDirectoryRenamed(directory: DirectoryFragment) {
  renameDirectoryDialog.close();
  navigateToDir(directory.id);
}
// #endregion

// #region 过滤器与多选状态
const { filterRating, lastSession } = useDirectoryState(currentDirectoryId);

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

// 查询当前目录自身元数据
const { currentDirectory } = useDirectories(
  () => ({
    id: currentDirectoryId.value,
    first: 0,
  }),
  { loadingCount },
);

// #region 同级目录切换逻辑
// 推导父目录 ID。若当前已经在根目录，则不加载父目录
const parentDirectoryId = computed(() => {
  if (!currentDirectoryId.value) return undefined;
  return currentDirectory.value?.parentId || "";
});

// 查询并排序的同级目录列表（当 parentDirectoryId 为空或 undefined 时跳过）
const {
  sortedDirectories: sortedSiblings,
  liveDirectories: siblingDirectories,
  hasNextPage: siblingsHasNextPage,
  fetchMore: fetchMoreSiblings,
} = useDirectories(
  () => {
    if (!parentDirectoryId.value) {
      return { id: "" };
    }
    return { id: parentDirectoryId.value };
  },
  {
    maxUnratedCount: maxUnratedCount,
    showLargeUnrated,
  },
);

// 父目录的子目录可能超过单页大小，此时当前目录落在后续页上，
// 只消费首页会找不到它的相邻目录（索引为 -1），与子目录网格的无限滚动结果不一致，
// 因此把父目录的剩余分页全部加载完。
// 以父目录 id 为键单飞串行：并发加载会被 useRelayConnection 的重入保护直接跳过而空转，
// 换目录或组件卸载时随任务作用域中止。
const siblingsFlight = new SingleFlightGroup();

async function fetchAllSiblings(parentId: string, signal: AbortSignal): Promise<void> {
  await siblingsFlight.do(
    parentId,
    async (flightSignal) => {
      while (!flightSignal.aborted && siblingsHasNextPage.value) {
        const loadedCount = siblingDirectories.value.length;
        await fetchMoreSiblings();
        // 已加载数量没有增长说明分页已无法推进，避免变成无上限的请求循环
        if (siblingDirectories.value.length <= loadedCount) return;
      }
    },
    signal,
  );
}

// 无父目录（位于根目录）时不加载，父目录连接不存在时也不会发起额外请求
useAsyncTask({
  // 已加载数量只用于让任务在分页推进后重新就绪，实际加载由上面的循环串行完成
  args: () =>
    parentDirectoryId.value
      ? [parentDirectoryId.value, siblingDirectories.value.length]
      : undefined,
  task: (parentId, _loadedCount, ctx) => fetchAllSiblings(parentId, ctx.signal()),
});

const { getCachedStats } = useDirectoryStats();

// 对同级目录进行与子目录列表一致的过滤和排序
const siblingEntries = computed(() => {
  const dirs = sortedSiblings.value;
  const visibility = {
    showUncompleted: showUncompletedDirectories.value,
    showLargeUnrated: showLargeUnrated.value,
    unratedLimit: maxUnratedCount.value,
  };

  return sortDirectoryOrder(
    dirs.map((dir) => {
      const stats = getCachedStats(dir.id);
      const { isFilteredOut, isForcedShown } = evaluateDirectoryVisibility(dir, stats, visibility);

      return {
        id: dir.id,
        dir,
        stats,
        isFilteredOut,
        isForcedShown,
      };
    }),
  );
});

const processedSiblings = computed(() =>
  siblingEntries.value.filter((item) => !item.isFilteredOut || item.isForcedShown),
);

// #region 游标锚定的同级导航
// 进入目录或筛选开关变化时重新生成游标；分页加载更多与统计数据更新不改变已锚定的位置
const browseCursor = shallowRef(getBrowseCursor());
const cursorRevision = ref(0);
const frozenCursorRevision = ref(-1);

watch(
  [currentDirectoryId, maxUnratedCount, showLargeUnrated, showUncompletedDirectories],
  () => {
    cursorRevision.value += 1;
    browseCursor.value = undefined;
    setBrowseCursor(undefined);
  },
  { immediate: true },
);

watchEffect(() => {
  const revision = cursorRevision.value;
  if (frozenCursorRevision.value === revision) return;

  const currentEntry = processedSiblings.value.find((item) => item.id === currentDirectoryId.value);
  if (!currentEntry) return;

  const cursor = createDirectoryCursor(currentEntry);
  browseCursor.value = cursor;
  setBrowseCursor(cursor);

  // 统计信息到达后重新生成一次排序键，随后冻结，避免筛选期间目录状态变化导致位置漂移
  if (currentEntry.stats) {
    frozenCursorRevision.value = revision;
  }
});

// 上一个同级目录
const prevSibling = computed(() => {
  const cursor = browseCursor.value;
  if (!cursor) return undefined;
  return findPreviousDirectory(processedSiblings.value, cursor, currentDirectoryId.value)?.dir;
});

// 下一个同级目录
const nextSibling = computed(() => {
  const cursor = browseCursor.value;
  if (!cursor) return undefined;
  return findNextDirectory(processedSiblings.value, cursor, currentDirectoryId.value)?.dir;
});
// #endregion

// 提取目录名用于 UI 展示
function getDirName(relPath: string): string {
  if (!relPath) return "";
  return relPath.split(/[/\\]/).pop() || "";
}
// #endregion
// #endregion

// #region 绝对物理路径与资源管理器打开
import { useOpenDir } from "../composables/useOpenDir";
import { useFileSystemContext } from "../composables/useFileSystemContext";
const { revealInExplorer } = useOpenDir();

const { absPath } = useFileSystemContext();

const fullDirectoryPath = computed(() => absPath(currentDirectory.value?.relPath));
// #endregion

// #region 同级目录切换快捷键

// 返回上一级目录
useHotkeys(
  {
    backspace: () => {
      if (canGoToParent.value) {
        goToParent();
      }
    },
  },
  { description: "返回上一级目录", category: "目录导航" },
);

// 切换到上一个同级目录
useHotkeys(
  {
    "[": () => {
      if (prevSibling.value) navigateToDir(prevSibling.value.id);
    },
  },
  { description: "上一个目录", category: "目录导航" },
);

// 切换到下一个同级目录
useHotkeys(
  {
    "]": () => {
      if (nextSibling.value) navigateToDir(nextSibling.value.id);
    },
  },
  { description: "下一个目录", category: "目录导航" },
);
// #endregion
</script>
