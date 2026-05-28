<template>
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
        <ToggleSwitch v-model="showHiddenMemos" label="显示隐藏笔记" />
      </div>
    </div>

    <!-- 暂无笔记状态 -->
    <div
      v-if="memos.length === 0"
      class="py-6 text-center text-primary-500 text-sm italic"
    >
      暂无任何笔记
    </div>

    <!-- 笔记列表项 -->
    <div v-else class="space-y-2 max-h-[40vh] overflow-y-auto pr-1">
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
          <!-- 文件名按钮，点击打开对应图片查看器 -->
          <span
            class="text-[10px] text-primary-400 shrink-0 bg-primary-800/60 px-2 py-1 rounded border border-primary-700/50 font-mono select-none cursor-pointer hover:text-secondary-400 hover:border-secondary-500/50 transition-colors"
            title="打开关联图片"
            @click.stop="openImageViewerForMemo(memoItem)"
          >
            {{ memoDisplayName(memoItem) }}
          </span>
          <span
            class="text-sm text-primary-200 group-hover:text-white transition-colors truncate font-medium"
          >
            {{ memoItem.content || "（空白笔记内容，点击编辑）" }}
          </span>
          <!-- 如果是隐藏笔记，显示标记 -->
          <span
            v-if="memoItem.hidden"
            class="px-2 py-1 text-[10px] bg-red-950/40 border border-red-900/50 text-red-400 rounded-md shrink-0 flex items-center gap-0.5"
            title="此笔记已通过 frontmatter 隐藏"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiEyeOff" fill="currentColor" />
            </svg>
            已隐藏
          </span>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <!-- 一键切换隐藏状态按钮 -->
          <button
            class="p-2 rounded-lg bg-primary-800/40 hover:bg-primary-700/60 border border-primary-700/50 text-primary-400 hover:text-white transition-all active:scale-95 flex items-center justify-center opacity-0 group-hover:opacity-100"
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
            class="text-xs text-primary-500 opacity-0 group-hover:opacity-100 transition-opacity duration-200 select-none flex items-center gap-1 cursor-pointer"
          >
            <span>编辑</span>
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiChevronRight" fill="currentColor" />
            </svg>
          </div>
        </div>
      </div>
    </div>

    <!-- 备忘录/笔记编辑对话框 -->
    <memoDialog.component container-class="sm:max-w-3xl short:max-w-none">
      <MemoForm
        v-if="selectedMemo"
        ref="memoDialogRef"
        :memo="selectedMemo"
        @close="memoDialog.close"
      />
    </memoDialog.component>

    <!-- 新建笔记对话框 -->
    <createDialog.component container-class="sm:max-w-3xl short:max-w-none">
      <CreateMemoForm
        ref="createDialogRef"
        :directory-id="props.directoryId"
        :existing-memos="memos"
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
} from "@mdi/js";
import CreateMemoForm from "./CreateMemoForm.vue";
import useBrowseMemos from "@/composables/useBrowseMemos";
import { useDirectoryState } from "@/composables/useDirectoryState";
import type {
  BrowseMemosQueryVariables,
  MemoFiltersInput,
  MemoFragment,
} from "@/graphql/generated";
import MemoForm from "./MemoForm.vue";
import ToggleSwitch from "./ToggleSwitch.vue";
import { openImageViewerByFilename } from "@/events";

// #region 组件属性定义
interface Props {
  directoryId: string;
}

const props = defineProps<Props>();
// #endregion

function memoDisplayName(memo: MemoFragment) {
  const basename = memo.relPath.split("/").pop() ?? memo.relPath;
  return basename.replace(/\.md$/, "");
}

// #region 数据查询与过滤状态处理
const directoryIdRef = computed(() => props.directoryId);

// 使用 useDirectoryState 独立管理当前目录的笔记过滤配置（包括显示隐藏状态）
const { showHiddenMemos } = useDirectoryState(directoryIdRef);

// 构建笔记查询参数变量，支持响应式更新
const memosVariables = computed<BrowseMemosQueryVariables>(() => {
  const filterBy: MemoFiltersInput = {
    directoryId: [directoryIdRef.value],
  };
  if (!showHiddenMemos.value) {
    filterBy.hidden = false;
  }
  return {
    id: directoryIdRef.value,
    filterBy,
    first: 100,
    after: null,
  };
});

// 调用 composable 查询并订阅笔记数据的变化
const { memos, toggleMemoHidden } = useBrowseMemos(memosVariables);
// #endregion

// #region 笔记编辑弹出框管理
const selectedMemo = ref<MemoFragment | null>(null);
const memoDialogRef =
  useTemplateRef<InstanceType<typeof MemoForm>>("memoDialogRef");

const memoDialog = useModalDialog({
  onDidOpen() {
    document.body.style.overflow = "hidden";
    nextTick(() => {
      memoDialogRef.value?.focus();
    });
  },
  onWillClose() {
    document.body.style.overflow = "";
    memoDialogRef.value?.flush();
  },
});

function editMemo(memoItem: MemoFragment) {
  selectedMemo.value = memoItem;
  nextTick(() => {
    memoDialog.open();
  });
}

function openImageViewerForMemo(memoItem: MemoFragment) {
  openImageViewerByFilename.dispatch({
    detail: { filename: memoDisplayName(memoItem) },
  });
}

// #region 新建笔记弹出框管理
const createDialogRef =
  useTemplateRef<InstanceType<typeof CreateMemoForm>>("createDialogRef");

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
