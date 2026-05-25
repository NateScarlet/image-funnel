<template>
  <div class="flex flex-col h-full">
    <!-- 对话框头部 -->
    <div
      class="px-4 sm:px-8 py-3 sm:py-6 short:py-1 border-b border-primary-700 flex items-center justify-between bg-primary-800/50 shrink-0 text-left"
    >
      <h3
        class="text-base sm:text-2xl short:text-sm font-bold text-primary-100 flex items-center gap-3"
      >
        <svg
          class="w-4 sm:w-8 h-4 sm:h-8 short:w-4 short:h-4 text-secondary-400"
          viewBox="0 0 24 24"
        >
          <path :d="mdiNoteTextOutline" fill="currentColor" />
        </svg>
        <span class="short:text-xs truncate">新建笔记</span>
      </h3>
      <button
        class="p-2 sm:p-3 short:p-1 hover:bg-primary-700 rounded-lg text-primary-400 transition-colors active:scale-95 cursor-pointer"
        type="button"
        @click="emit('close')"
      >
        <svg
          class="w-5 sm:w-8 h-5 sm:h-8 short:w-4 short:h-4"
          viewBox="0 0 24 24"
        >
          <path :d="mdiClose" fill="currentColor" />
        </svg>
      </button>
    </div>

    <!-- 对话框主体内容区 -->
    <div
      class="px-4 sm:px-10 py-4 sm:py-10 short:px-2 short:py-1 overflow-y-auto flex-1 min-h-0 text-left"
    >
      <!-- 文件名输入区 -->
      <div class="mb-4 sm:mb-6">
        <label
          class="block text-xs sm:text-sm font-bold text-primary-400 mb-2 select-none uppercase tracking-wider"
        >
          文件名
        </label>
        <div
          class="flex items-center bg-primary-800/50 focus-within:bg-primary-800 border border-primary-700 focus-within:border-secondary-500/50 rounded-xl px-4 py-2 sm:py-3 transition-all duration-300"
        >
          <input
            ref="filenameInput"
            v-model="filename"
            type="text"
            class="flex-1 bg-transparent border-none text-sm sm:text-base text-primary-100 placeholder-primary-500 outline-none leading-relaxed min-w-0"
            placeholder="README"
            @keydown.enter.prevent="focusContent"
          />
          <span
            class="text-xs sm:text-sm text-primary-400 select-none font-mono font-medium ml-1"
            >.md</span
          >
        </div>
      </div>

      <!-- 内容输入区 -->
      <div class="relative w-full group">
        <label
          class="block text-xs sm:text-sm font-bold text-primary-400 mb-2 select-none uppercase tracking-wider"
        >
          笔记内容
        </label>
        <textarea
          ref="textarea"
          v-model="content"
          class="w-full bg-primary-800/50 hover:bg-primary-800 focus:bg-primary-800 border border-primary-700 focus:border-secondary-500/50 rounded-xl px-4 py-3 sm:px-8 sm:py-6 short:py-1 text-sm sm:text-xl text-primary-100 placeholder-primary-500 outline-none transition-all duration-300 resize-none leading-relaxed min-h-30 sm:min-h-60 short:min-h-10 max-h-[50vh] short:max-h-none overflow-y-auto"
          placeholder="输入备注内容..."
          data-no-gesture
        ></textarea>
      </div>

      <!-- 操作按钮区 -->
      <div class="mt-6 flex justify-end gap-3 shrink-0">
        <button
          type="button"
          class="px-4 py-2 text-sm font-medium rounded-xl border border-primary-700 hover:border-primary-600 bg-primary-800/40 hover:bg-primary-800 text-primary-300 hover:text-white transition-all active:scale-95 cursor-pointer"
          @click="emit('close')"
        >
          取消
        </button>
        <button
          type="button"
          class="flex items-center gap-1.5 px-5 py-2 text-sm font-semibold rounded-xl bg-secondary-500 hover:bg-secondary-600 text-white shadow-lg shadow-secondary-500/20 active:scale-95 transition-all cursor-pointer"
          :disabled="isSaving"
          @click="saveMemo"
        >
          <svg v-if="isSaving" class="w-4 h-4 animate-spin" viewBox="0 0 24 24">
            <path :d="mdiLoading" fill="currentColor" />
          </svg>
          <span>保存</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, useTemplateRef, onMounted } from "vue";
import type { MemoFragment } from "@/graphql/generated";
import { CreateMemoDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import { mdiNoteTextOutline, mdiClose, mdiLoading } from "@mdi/js";
import useNotification from "@/composables/useNotification";
import useTextAreaAutoHeight from "@/composables/useTextAreaAutoHeight";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
  existingMemos: MemoFragment[];
}>();

const emit = defineEmits<(e: "close" | "saved") => void>();
// #endregion

const filename = ref("README");
const content = ref("");

const filenameInput = useTemplateRef<HTMLInputElement>("filenameInput");
const textarea = useTemplateRef<HTMLTextAreaElement>("textarea");
const isSaving = ref(false);

const { showError, showSuccess } = useNotification();

// 自动调整高度
useTextAreaAutoHeight(textarea, content);

// 组件挂载后默认聚焦在文件名输入框上
onMounted(() => {
  filenameInput.value?.focus();
});

// 在文件名输入框按回车时自动聚焦到内容框
function focusContent() {
  textarea.value?.focus();
}

async function saveMemo() {
  if (isSaving.value) return;

  const cleanName = filename.value.trim().replace(/\.md$/i, "");
  if (!cleanName) {
    showError("文件名不能为空");
    return;
  }

  // 校验文件名中是否含有非法字符
  if (/[\\/:*?"<>|]/.test(cleanName)) {
    showError('文件名包含非法字符 (\\ / : * ? " < > |)');
    return;
  }

  // 计算目录相对路径并查重
  const dirRelPath = props.directoryId
    ? props.directoryId.replace(/^dir:/, "")
    : "";
  const finalRelPath = dirRelPath
    ? `${dirRelPath}/${cleanName}.md`
    : `${cleanName}.md`;
  const isDuplicate = props.existingMemos.some(
    (m) => m.relPath.toLowerCase() === finalRelPath.toLowerCase(),
  );

  if (isDuplicate) {
    showError("该目录下已存在同名笔记");
    return;
  }

  if (!content.value.trim()) {
    showError("笔记内容不能为空");
    return;
  }

  isSaving.value = true;
  try {
    await mutate(CreateMemoDocument, {
      variables: {
        input: {
          directoryId: props.directoryId,
          name: cleanName,
          content: content.value,
        },
      },
    });

    showSuccess("新建笔记成功");
    emit("saved");
  } catch (err) {
    console.error("Failed to create memo:", err);
    const msg = err instanceof Error ? err.message : "创建笔记失败";
    showError(msg);
  } finally {
    isSaving.value = false;
  }
}

// 暴露 focus 方法以供外部通过 ref 调用
function focus() {
  filenameInput.value?.focus();
}

defineExpose({ focus });
</script>
