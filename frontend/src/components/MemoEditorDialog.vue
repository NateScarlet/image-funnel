<template>
  <Transition
    enter-active-class="transition duration-300 ease-out"
    enter-from-class="opacity-0 backdrop-blur-0"
    enter-to-class="opacity-100 backdrop-blur-md"
    leave-active-class="transition duration-200 ease-in"
    leave-from-class="opacity-100 backdrop-blur-md"
    leave-to-class="opacity-0 backdrop-blur-0"
  >
    <div
      v-if="modelValue"
      class="fixed inset-0 z-100 flex items-center justify-center p-4 bg-black/60"
      @click.self="modelValue = false"
    >
      <div
        class="w-full max-w-lg bg-primary-900 border border-primary-700 rounded-2xl shadow-2xl overflow-hidden"
        @click.stop
      >
        <div
          class="p-4 border-b border-primary-700 flex items-center justify-between bg-primary-800/50"
        >
          <h3
            class="text-lg font-bold text-primary-100 flex items-center gap-2"
          >
            <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiNoteTextOutline" fill="currentColor" />
            </svg>
            图片备注
          </h3>
          <button
            class="p-2 hover:bg-primary-700 rounded-lg text-primary-400 transition-colors"
            @click="modelValue = false"
          >
            <svg class="w-6 h-6" viewBox="0 0 24 24">
              <path :d="mdiClose" fill="currentColor" />
            </svg>
          </button>
        </div>

        <div class="p-6">
          <MemoEditor ref="editor" :memo="memo" @saved="onSaved" />
          <p class="mt-4 text-xs text-primary-500 italic">
            备注信息将保存为同名的 .md 文件。内容为空时将自动删除备注文件。
          </p>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { mdiNoteTextOutline, mdiClose } from "@mdi/js";
import MemoEditor from "./MemoEditor.vue";
import type { MemoFragment as Memo } from "../graphql/generated";
import { useTemplateRef, watch } from "vue";

const modelValue = defineModel<boolean>({ required: true });

defineProps<{
  memo: Memo;
}>();

const editor = useTemplateRef<InstanceType<typeof MemoEditor>>("editor");

watch(modelValue, (val) => {
  if (!val) {
    editor.value?.flush();
  }
});

function onSaved() {
  // 可以根据需要添加保存后的处理
}
</script>
