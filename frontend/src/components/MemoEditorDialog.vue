<template>
  <Teleport :to="rendererEl">
    <div v-if="isVisible" class="fixed inset-0 z-100 pointer-events-none">
      <!-- 背景遮罩：独立于内容容器，确保始终全屏 -->
      <Transition
        enter-active-class="transition duration-300 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-200 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div
          v-if="modelValue"
          class="absolute inset-0 bg-black/60 backdrop-blur-md pointer-events-auto"
          @click="close"
        ></div>
      </Transition>

      <!-- 内容容器：底部对齐，会自动被键盘顶起 -->
      <div
        class="absolute inset-x-0 bottom-0 flex flex-col justify-end sm:items-center sm:justify-center short:justify-start overflow-hidden max-h-full"
      >
        <Transition
          appear
          enter-active-class="transition duration-300 ease-out"
          enter-from-class="translate-y-full"
          enter-to-class="translate-y-0"
          leave-active-class="transition duration-200 ease-in"
          leave-from-class="translate-y-0"
          leave-to-class="translate-y-full"
          @after-enter="onAfterEnter"
          @leave="onLeave"
          @after-leave="onAfterLeave"
        >
          <div
            v-if="modelValue"
            class="relative w-full sm:max-w-3xl short:max-w-none sm:rounded-2xl rounded-t-3xl short:rounded-none bg-primary-900 border border-primary-700 shadow-2xl overflow-hidden flex flex-col max-h-[95dvh] sm:max-h-[85vh] short:max-h-dvh pointer-events-auto"
            @click.stop
          >
            <div
              class="px-4 sm:px-8 py-3 sm:py-6 short:py-1 border-b border-primary-700 flex items-center justify-between bg-primary-800/50 shrink-0"
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
                <span class="short:text-xs">图片备注</span>
              </h3>
              <button
                class="p-2 sm:p-3 short:p-1 hover:bg-primary-700 rounded-lg text-primary-400 transition-colors active:scale-95"
                @click="close"
              >
                <svg
                  class="w-5 sm:w-8 h-5 sm:h-8 short:w-4 short:h-4"
                  viewBox="0 0 24 24"
                >
                  <path :d="mdiClose" fill="currentColor" />
                </svg>
              </button>
            </div>

            <div
              class="px-4 sm:px-10 py-4 sm:py-10 short:px-2 short:py-1 overflow-y-auto flex-1 min-h-0"
            >
              <MemoEditor ref="editor" :memo="memo" @saved="onSaved" />
              <p
                class="mt-3 sm:mt-8 short:hidden text-xs sm:text-base text-primary-500 italic leading-relaxed"
              >
                备注信息将保存为同名的 .md 文件。内容为空时将自动删除备注文件。
              </p>
            </div>
          </div>
        </Transition>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { mdiNoteTextOutline, mdiClose } from "@mdi/js";
import MemoEditor from "./MemoEditor.vue";
import type { MemoFragment as Memo } from "../graphql/generated";
import { ref, useTemplateRef, watch, nextTick, onUnmounted } from "vue";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";

const rendererEl = useFullscreenRendererElement();
const modelValue = defineModel<boolean>({ required: true });
const isVisible = ref(false);

defineProps<{
  memo: Memo;
}>();

const editor = useTemplateRef<InstanceType<typeof MemoEditor>>("editor");

watch(modelValue, (val) => {
  if (val) {
    document.body.style.overflow = "hidden";
    isVisible.value = true;
  } else {
    document.body.style.overflow = "";
  }
});

function close() {
  editor.value?.flush();
  modelValue.value = false;
}

function onAfterEnter() {
  nextTick(() => {
    editor.value?.focus();
  });
}

function onLeave() {
  // Leave animation started
}

function onAfterLeave() {
  isVisible.value = false;
}

function onSaved() {
  // 可以根据需要添加保存后的处理
}

onUnmounted(() => {
  document.body.style.overflow = "";
});
</script>
