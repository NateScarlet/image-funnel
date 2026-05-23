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
        <span class="short:text-xs truncate" :title="memo.title">
          编辑笔记 ({{ memo.title }})
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
      <MemoEditor ref="editor" :memo="memo" @saved="onSaved" />
      <p
        class="mt-3 sm:mt-8 short:hidden text-xs sm:text-base text-primary-500 italic leading-relaxed"
      >
        备注信息将保存为同名的 .md 文件。内容为空时将自动删除备注文件。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { mdiNoteTextOutline, mdiClose } from "@mdi/js";
import MemoEditor from "./MemoEditor.vue";
import type { MemoFragment as Memo } from "../graphql/generated";
import { useTemplateRef } from "vue";

// #region 属性与事件定义
defineProps<{
  memo: Memo;
}>();

const emit = defineEmits<(e: "close") => void>();
// #endregion

const editor = useTemplateRef<InstanceType<typeof MemoEditor>>("editor");

// #region 暴露焦点和数据刷入方法供父组件控制
const focus = () => {
  editor.value?.focus();
};

const flush = () => {
  editor.value?.flush();
};

defineExpose({
  focus,
  flush,
});
// #endregion

function onSaved() {
  // 可以根据需要添加保存后的处理
}
</script>
