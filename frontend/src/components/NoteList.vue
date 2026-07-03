<template>
  <!-- 笔记列表容器区 -->
  <section
    class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
  >
    <div class="flex items-center justify-between border-b border-primary-700/50 pb-3">
      <h2
        class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
      >
        <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
          <path :d="mdiNoteTextOutline" fill="currentColor" />
        </svg>
        笔记列表 ({{ notes.length }} 个)
      </h2>
      <!-- 按钮区，包含新建按钮和切换开关 -->
      <div class="flex items-center gap-4">
        <button
          class="flex items-center gap-1 px-3 py-1 text-xs font-semibold rounded-lg border border-secondary-500/30 hover:border-secondary-500/60 bg-secondary-500/10 hover:bg-secondary-500/20 text-secondary-400 hover:text-secondary-300 transition-all duration-200 active:scale-95 cursor-pointer"
          title="新建笔记"
          @click="openCreateDialog"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24">
            <path :d="mdiPlus" fill="currentColor" />
          </svg>
          <span>新建</span>
        </button>
        <!-- 显示隐藏笔记切换开关 -->
        <ToggleSwitch v-model="showHiddenNotes" label="显示隐藏笔记" />
      </div>
    </div>

    <!-- 暂无笔记状态 -->
    <div v-if="notes.length === 0" class="py-6 text-center text-primary-500 text-sm italic">
      暂无任何笔记
    </div>

    <!-- 笔记列表项 -->
    <div v-else ref="containerRef" class="max-h-[40vh] overflow-y-auto pr-1">
      <div class="space-y-2 p-4">
        <div
          v-for="noteItem in notes"
          :key="noteItem.id"
          class="flex items-center flex-wrap gap-x-3 gap-y-2 p-3 rounded-xl bg-primary-800/20 hover:bg-primary-800/60 border border-primary-800/40 hover:border-secondary-500/30 transition-all duration-200 group cursor-pointer"
          @click="editNote(noteItem)"
        >
          <!-- 图标始终在最前 -->
          <svg
            class="order-1 w-4 h-4 text-primary-400 group-hover:text-secondary-400 transition-colors shrink-0"
            viewBox="0 0 24 24"
          >
            <path :d="mdiNoteTextOutline" fill="currentColor" />
          </svg>
          <!-- 文件名按钮：移动端在第二行，桌面端紧随图标 -->
          <span
            class="order-3 sm:order-2 text-xs text-primary-400 shrink-0 bg-primary-800/60 px-2 py-1 rounded border border-primary-700/50 font-mono select-none cursor-pointer hover:text-secondary-400 hover:border-secondary-500/50 transition-colors"
            title="打开关联图片"
            @click.stop="openImageViewerForNote(noteItem)"
          >
            {{ noteDisplayName(noteItem) }}
          </span>
          <!-- 正文内容：移动端独占第一行，桌面端在文件名之后 -->
          <span
            class="order-2 sm:order-3 text-sm text-primary-200 group-hover:text-white transition-colors truncate font-medium min-w-0 flex-1"
          >
            {{ noteItem.content || "（空白笔记内容，点击编辑）" }}
          </span>
          <!-- 如果是隐藏笔记，显示标记 -->
          <span
            v-if="noteItem.hidden"
            class="order-4 px-2 py-1 text-xs bg-red-950/40 border border-red-900/50 text-red-400 rounded-md shrink-0 flex items-center gap-1"
            title="此笔记已通过 frontmatter 隐藏"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiEyeOff" fill="currentColor" />
            </svg>
            已隐藏
          </span>
          <!-- 一键切换隐藏状态按钮：触屏设备始终可见，hover 设备悬停时显示 -->
          <button
            class="order-5 ml-auto p-2 rounded-lg bg-primary-800/40 hover:bg-primary-700/60 border border-primary-700/50 text-primary-400 hover:text-white transition-all active:scale-95 flex items-center justify-center [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100"
            :title="noteItem.hidden ? '取消隐藏此笔记' : '隐藏此笔记'"
            @click.stop="toggleNoteHidden(noteItem)"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="noteItem.hidden ? mdiEye : mdiEyeOff" fill="currentColor" />
            </svg>
          </button>
          <!-- 编辑提示：触屏设备始终可见，hover 设备悬停时显示 -->
          <div
            class="order-6 text-xs text-primary-500 transition-opacity duration-200 select-none flex items-center gap-1 cursor-pointer [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100"
          >
            <span>编辑</span>
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiChevronRight" fill="currentColor" />
            </svg>
          </div>
        </div>
      </div>

      <!-- 加载更多分页控制 -->
      <div
        v-if="hasNextPage"
        class="mt-4 flex justify-center border-t border-primary-700/30 pt-3 pb-1"
      >
        <button
          :disabled="loading"
          class="px-4 py-1.5 bg-primary-800 hover:bg-primary-700 rounded-lg text-xs text-primary-300 hover:text-white border border-primary-700 transition-colors cursor-pointer flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
          @click="fetchMore"
        >
          <!-- 加载中动画 -->
          <svg
            v-if="loading"
            class="w-3.5 h-3.5 animate-spin text-secondary-500"
            viewBox="0 0 24 24"
            fill="none"
          >
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
          <span>{{ loading ? "正在加载..." : "加载更多笔记" }}</span>
        </button>
      </div>
    </div>

    <!-- 笔记编辑对话框 -->
    <noteDialog.component container-class="sm:max-w-3xl short:max-w-none">
      <NoteForm
        v-if="selectedNote"
        ref="noteDialogRef"
        :note="selectedNote"
        @close="noteDialog.close"
      />
    </noteDialog.component>

    <!-- 新建笔记对话框 -->
    <createDialog.component container-class="sm:max-w-3xl short:max-w-none">
      <CreateNoteForm
        ref="createDialogRef"
        :directory-id="props.directoryId"
        :existing-notes="notes"
        @close="createDialog.close"
        @saved="createDialog.close"
      />
    </createDialog.component>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, useTemplateRef, nextTick } from "vue";
import useModalDialog from "@/composables/useModalDialog";
import {
  mdiNoteTextOutline,
  mdiEye,
  mdiEyeOff,
  mdiChevronRight,
  mdiPlus,
  mdiLoading,
} from "@mdi/js";
import CreateNoteForm from "./CreateNoteForm.vue";
import useBrowseNotes from "@/composables/useBrowseNotes";
import useInfiniteScroll from "@/composables/useInfiniteScroll";
import { useDirectoryState } from "@/composables/useDirectoryState";
import type {
  BrowseNotesQueryVariables,
  NoteFiltersInput,
  NoteFragment,
} from "@/graphql/generated";
import NoteForm from "./NoteForm.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import { openImageViewerByFilename } from "@/events";

// #region 组件属性定义
interface Props {
  directoryId: string;
}

const props = defineProps<Props>();
// #endregion

function noteDisplayName(note: NoteFragment) {
  const basename = note.relPath.split("/").pop() ?? note.relPath;
  return basename.replace(/\.md$/, "");
}

// #region 数据查询与过滤状态处理
const directoryIdRef = computed(() => props.directoryId);

// 使用 useDirectoryState 独立管理当前目录的笔记过滤配置（包括显示隐藏状态）
const { showHiddenNotes } = useDirectoryState(directoryIdRef);

// 构建笔记查询参数变量，支持响应式更新
const notesVariables = computed<BrowseNotesQueryVariables>(() => {
  const filterBy: NoteFiltersInput = {
    directoryId: [directoryIdRef.value],
  };
  if (!showHiddenNotes.value) {
    filterBy.hidden = false;
  }
  return {
    id: directoryIdRef.value,
    filterBy,
    first: 20, // 设定较小的值以启用分页和无限加载
    after: null,
  };
});

// 定义加载状态
const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);

// 调用 composable 查询并订阅笔记数据的变化
const { notes, toggleNoteHidden, hasNextPage, fetchMore } = useBrowseNotes(notesVariables, {
  loadingCount,
});

const containerRef = useTemplateRef<HTMLElement>("containerRef");

useInfiniteScroll(containerRef, async () => {
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});
// #endregion

// #region 笔记编辑弹出框管理
const selectedNote = ref<NoteFragment | null>(null);
const noteDialogRef = useTemplateRef<InstanceType<typeof NoteForm>>("noteDialogRef");

const noteDialog = useModalDialog({
  onDidOpen() {
    document.body.style.overflow = "hidden";
    nextTick(() => {
      noteDialogRef.value?.focus();
    });
  },
  onWillClose() {
    document.body.style.overflow = "";
    noteDialogRef.value?.flush();
  },
});

function editNote(noteItem: NoteFragment) {
  selectedNote.value = noteItem;
  nextTick(() => {
    noteDialog.open();
  });
}

function openImageViewerForNote(noteItem: NoteFragment) {
  openImageViewerByFilename.dispatch({
    detail: { filename: noteDisplayName(noteItem) },
  });
}

// #region 新建笔记弹出框管理
const createDialogRef = useTemplateRef<InstanceType<typeof CreateNoteForm>>("createDialogRef");

const createDialog = useModalDialog({
  onDidOpen() {
    document.body.style.overflow = "hidden";
    nextTick(() => {
      createDialogRef.value?.focus();
    });
  },
  onWillClose() {
    document.body.style.overflow = "";
  },
});

function openCreateDialog() {
  createDialog.open();
}
// #endregion
// #endregion
</script>
