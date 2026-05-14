<script setup lang="ts">
import { ref, computed, onUnmounted, useTemplateRef } from "vue";
import type { MemoFragment as Memo } from "../graphql/generated";
import { debounce } from "es-toolkit";
import useMemo from "@/composables/useMemo";
import { mdiLoading, mdiCheck, mdiAlertCircleOutline } from "@mdi/js";
import useCurrentTime from "@/composables/useCurrentTime";
import useTextAreaAutoHeight from "@/composables/useTextAreaAutoHeight";

const textarea = useTemplateRef("textarea");

function focus() {
  textarea.value?.focus();
}

const props = defineProps<{
  memo: Memo;
}>();

const { memo, updateMemo } = useMemo(() => props.memo.id);
const { currentTime, refreshOn } = useCurrentTime();

const contentBuffer = ref<{ id: string; content: string }>();
const content = computed({
  get: () => {
    if (contentBuffer.value && contentBuffer.value.id === props.memo.id) {
      return contentBuffer.value.content;
    }
    return memo.value?.content ?? props.memo.content;
  },
  set: (v) => {
    contentBuffer.value = { id: props.memo.id, content: v };
  },
});

useTextAreaAutoHeight(textarea, content);

enum SaveStatus {
  IDLE,
  SAVING,
  SAVED,
  ERROR,
}

const isSaving = ref(false);
const lastSaved = ref<{ id: string; at: number }>();
const lastError = ref<{ id: string; at: number }>();
const isPending = ref(false);

// 声明式调度刷新，确保状态在超时后自动更新
refreshOn(() => [lastSaved.value ? lastSaved.value.at + 2000 : undefined]);

const currentStatus = computed(() => {
  if (isSaving.value) return SaveStatus.SAVING;

  // 检查最近是否有针对当前 ID 的成功保存
  if (lastSaved.value && lastSaved.value.id === props.memo.id) {
    const elapsed = currentTime.value.getTime() - lastSaved.value.at;
    if (elapsed < 2000) return SaveStatus.SAVED;
  }

  // 检查最近是否有针对当前 ID 的保存错误（持续显示直到成功）
  if (lastError.value && lastError.value.id === props.memo.id) {
    return SaveStatus.ERROR;
  }

  return SaveStatus.IDLE;
});

const performSave = async (newContent: string, targetId: string) => {
  if (targetId !== props.memo.id) return;

  if (newContent === (memo.value?.content ?? props.memo.content)) {
    if (contentBuffer.value?.id === targetId) {
      contentBuffer.value = undefined;
    }
    return;
  }

  isSaving.value = true;
  try {
    await updateMemo(newContent);
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
    console.error("Failed to save memo:", err);
    lastError.value = { id: targetId, at: Date.now() };
  } finally {
    isSaving.value = false;
    isPending.value = false;
  }
};

const save = debounce(performSave, 500);

function handleInput() {
  isPending.value = true;
  // 输入时清除成功状态，但保留错误状态直到保存成功
  lastSaved.value = undefined;
  save(content.value, props.memo.id);
}

function flush() {
  if (isPending.value) {
    save.cancel();
    performSave(content.value, props.memo.id);
  }
}

defineExpose({ flush, focus });

onUnmounted(() => {
  flush();
});

const emit = defineEmits<(e: "saved") => void>();
</script>

<template>
  <div class="relative w-full group">
    <textarea
      ref="textarea"
      v-model="content"
      class="w-full bg-primary-800/50 hover:bg-primary-800 focus:bg-primary-800 border border-primary-700 focus:border-secondary-500/50 rounded-xl px-4 py-3 sm:px-5 sm:py-4 text-sm sm:text-base text-primary-100 placeholder-primary-500 outline-none transition-all duration-300 resize-none leading-relaxed min-h-30 max-h-[50vh] overflow-y-auto"
      placeholder="输入备注信息... (自动保存)"
      data-no-gesture
      @input="handleInput"
    ></textarea>
    <div
      class="absolute bottom-2 sm:bottom-3 right-3 sm:right-4 text-[10px] uppercase tracking-wider font-bold transition-all duration-300 flex items-center gap-1"
      :class="{
        'text-secondary-400 opacity-100': currentStatus === SaveStatus.SAVING,
        'text-success-400 opacity-100': currentStatus === SaveStatus.SAVED,
        'text-error-400 opacity-100': currentStatus === SaveStatus.ERROR,
        'opacity-0': currentStatus === SaveStatus.IDLE,
      }"
    >
      <template v-if="currentStatus === SaveStatus.SAVING">
        <svg class="w-3 h-3 animate-spin" viewBox="0 0 24 24">
          <path :d="mdiLoading" fill="currentColor" />
        </svg>
        保存中
      </template>
      <template v-else-if="currentStatus === SaveStatus.SAVED">
        <svg class="w-3 h-3" viewBox="0 0 24 24">
          <path :d="mdiCheck" fill="currentColor" />
        </svg>
        已保存
      </template>
      <template v-else-if="currentStatus === SaveStatus.ERROR">
        <svg class="w-3 h-3" viewBox="0 0 24 24">
          <path :d="mdiAlertCircleOutline" fill="currentColor" />
        </svg>
        失败
      </template>
    </div>
  </div>
</template>
