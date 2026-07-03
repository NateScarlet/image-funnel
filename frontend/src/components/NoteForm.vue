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
        <span class="short:text-xs truncate" :title="displayTitle">
          编辑笔记 ({{ displayTitle }})
        </span>
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
      <div class="relative w-full group">
        <NoteEditor
          ref="noteEditor"
          v-model="content"
          placeholder="输入笔记... (自动保存)"
          :note-id="props.note.id"
          :on-before-dispatch="flush"
          @input="handleInput"
        />
        <div
          class="absolute bottom-2 sm:bottom-6 right-3 sm:right-8 text-xs sm:text-sm uppercase tracking-wider font-bold transition-all duration-300 flex items-center gap-2"
          :class="{
            'text-secondary-400 opacity-100':
              currentStatus === SaveStatus.SAVING,
            'text-success-400 opacity-100': currentStatus === SaveStatus.SAVED,
            'text-error-400 opacity-100': currentStatus === SaveStatus.ERROR,
            'opacity-0': currentStatus === SaveStatus.IDLE,
          }"
        >
          <template v-if="currentStatus === SaveStatus.SAVING">
            <svg class="w-3 h-3 sm:w-5 sm:h-5 animate-spin" viewBox="0 0 24 24">
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            保存中
          </template>
          <template v-else-if="currentStatus === SaveStatus.SAVED">
            <svg class="w-3 h-3 sm:w-5 sm:h-5" viewBox="0 0 24 24">
              <path :d="mdiCheck" fill="currentColor" />
            </svg>
            已保存
          </template>
          <template v-else-if="currentStatus === SaveStatus.ERROR">
            <svg class="w-3 h-3 sm:w-5 sm:h-5" viewBox="0 0 24 24">
              <path :d="mdiAlertCircleOutline" fill="currentColor" />
            </svg>
            失败
          </template>
        </div>
      </div>
      <p
        class="mt-3 sm:mt-8 short:hidden text-xs sm:text-base text-primary-500 italic leading-relaxed"
      >
        修改内容将在关闭对话框时自动保存。笔记信息将保存为同名的 .md
        文件。内容为空时将自动删除笔记文件。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted, useTemplateRef } from "vue";
import type { NoteFragment as Note } from "../graphql/generated";
import useNote from "@/composables/domain/useNote";
import NoteEditor from "./NoteEditor.vue";
import {
  mdiNoteTextOutline,
  mdiClose,
  mdiLoading,
  mdiCheck,
  mdiAlertCircleOutline,
} from "@mdi/js";
import useCurrentTime from "@/composables/useCurrentTime";

// #region 属性与事件定义
const props = defineProps<{
  note: Note;
}>();

const emit = defineEmits<(e: "close" | "saved") => void>();
// #endregion

const displayTitle = computed(() => {
  const basename = props.note.relPath.split("/").pop() ?? props.note.relPath;
  return basename.replace(/\.md$/, "");
});

const noteEditorRef = useTemplateRef("noteEditor");

function focus() {
  noteEditorRef.value?.focus();
}

const { note: serverNote, updateNote } = useNote(() => props.note.id);
const { currentTime, refreshOn } = useCurrentTime();

const contentBuffer = ref<{ id: string; content: string }>();
const content = computed({
  get: () => {
    if (contentBuffer.value && contentBuffer.value.id === props.note.id) {
      return contentBuffer.value.content;
    }
    return serverNote.value?.rawContent ?? props.note.rawContent;
  },
  set: (v) => {
    contentBuffer.value = { id: props.note.id, content: v };
  },
});

enum SaveStatus {
  IDLE,
  SAVING,
  SAVED,
  ERROR,
}

const isSaving = ref(false);
const lastSaved = ref<{ id: string; at: number }>();
const lastError = ref<{ id: string; at: number }>();
const isPending = computed(() => {
  const currentVal = content.value;
  const actualVal = serverNote.value?.rawContent ?? props.note.rawContent;
  return currentVal !== actualVal;
});

// 声明式调度刷新，确保状态在超时后自动更新
refreshOn(() => [lastSaved.value ? lastSaved.value.at + 2000 : undefined]);

const currentStatus = computed(() => {
  if (isSaving.value) return SaveStatus.SAVING;

  // 检查最近是否有针对当前 ID 的成功保存
  if (lastSaved.value && lastSaved.value.id === props.note.id) {
    const elapsed = currentTime.value.getTime() - lastSaved.value.at;
    if (elapsed < 2000) return SaveStatus.SAVED;
  }

  // 检查最近是否有针对当前 ID 的保存错误（持续显示直到成功）
  if (lastError.value && lastError.value.id === props.note.id) {
    return SaveStatus.ERROR;
  }

  return SaveStatus.IDLE;
});

const performSave = async (newContent: string, targetId: string) => {
  if (targetId !== props.note.id) return;

  if (newContent === (serverNote.value?.rawContent ?? props.note.rawContent)) {
    if (contentBuffer.value?.id === targetId) {
      contentBuffer.value = undefined;
    }
    return;
  }

  isSaving.value = true;
  try {
    await updateNote(newContent);
    if (
      contentBuffer.value?.id === targetId &&
      contentBuffer.value?.content === newContent
    ) {
      contentBuffer.value = undefined;
    }
    lastSaved.value = { id: targetId, at: Date.now() };
    lastError.value = undefined;
    emit("saved");
  } catch (err) {
    console.error("Failed to save note:", err);
    lastError.value = { id: targetId, at: Date.now() };
  } finally {
    isSaving.value = false;
  }
};

function handleInput() {
  // 输入时清除成功状态，但保留错误状态直到保存成功
  lastSaved.value = undefined;
}

async function flush() {
  if (isPending.value) {
    await performSave(content.value, props.note.id);
  }
  contentBuffer.value = undefined;
}

defineExpose({ flush, focus });

onUnmounted(() => {
  flush();
});
</script>
