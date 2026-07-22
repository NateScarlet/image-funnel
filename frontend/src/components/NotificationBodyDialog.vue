<template>
  <!-- 通知正文弹窗：展示 Hook 执行的 stderr 详情 -->
  <div class="flex flex-col">
    <!-- 标题栏 -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-primary-700">
      <h2 class="text-base font-bold text-primary-100 truncate">{{ title }}</h2>
      <button
        class="shrink-0 p-1 rounded-lg hover:bg-primary-700 transition-colors text-primary-400 hover:text-white cursor-pointer ml-2"
        @click="emit('close')"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24">
          <path :d="mdiClose" fill="currentColor" />
        </svg>
      </button>
    </div>

    <!-- 正文内容区 -->
    <div class="flex-1 overflow-auto p-4">
      <pre
        class="text-xs text-primary-200 font-mono whitespace-pre-wrap break-words bg-primary-900 rounded-lg p-3 max-h-96 overflow-auto"
        >{{ body }}</pre
      >
    </div>

    <!-- 操作按钮区 -->
    <div class="flex items-center justify-end gap-2 px-4 py-3 border-t border-primary-700">
      <button
        class="px-3 py-1 bg-primary-700 hover:bg-primary-600 text-primary-200 text-sm font-medium rounded-lg transition-colors cursor-pointer"
        @click="emit('close')"
      >
        关闭
      </button>
      <button
        class="px-3 py-1 bg-secondary-600 hover:bg-secondary-500 text-white text-sm font-medium rounded-lg transition-colors cursor-pointer"
        @click="handleCopy"
      >
        {{ copied ? "已复制" : "复制到剪贴板" }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { mdiClose } from "@mdi/js";

const props = defineProps<{
  title: string;
  body: string;
}>();

const emit = defineEmits<{
  close: [];
}>();

const copied = ref(false);

async function handleCopy() {
  await navigator.clipboard.writeText(props.body);
  copied.value = true;
  setTimeout(() => {
    copied.value = false;
  }, 2000);
}
</script>
