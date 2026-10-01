<template>
  <div>
    <!-- 头部标题区域 -->
    <div class="mb-6 flex justify-between items-start">
      <h2 class="text-lg font-bold text-primary-50 flex items-center gap-2">
        <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
          <path :d="mdiFolderEdit" fill="currentColor" />
        </svg>
        重命名目录
      </h2>
      <button
        class="text-primary-400 hover:text-primary-200 transition-colors p-2 rounded-lg hover:bg-primary-700/50 cursor-pointer shrink-0"
        type="button"
        title="关闭"
        :disabled="renaming"
        @click="emit('close')"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24">
          <path :d="mdiClose" fill="currentColor" />
        </svg>
      </button>
    </div>

    <!-- 表单输入区 -->
    <div class="space-y-4">
      <div>
        <label
          for="rename-directory-name"
          class="mb-2 block text-xs font-semibold text-primary-300"
        >
          目录名
        </label>
        <input
          id="rename-directory-name"
          ref="nameInput"
          v-model="newName"
          type="text"
          class="w-full bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-sm text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all px-3 py-2"
          placeholder="输入新的目录名"
          autocomplete="off"
          :disabled="renaming"
          @keydown.enter.prevent="submit"
        />
      </div>

      <p class="text-xs text-primary-500 leading-relaxed">
        只能修改当前目录的名字，目录内容与所在层级不会改变。名字里不能有
        <code class="text-primary-400">/</code> 或 <code class="text-primary-400">\</code>，也不能是
        <code class="text-primary-400">.</code> 或
        <code class="text-primary-400">..</code>；与同级已有条目重名时会失败。
      </p>
    </div>

    <!-- 操作按钮区 -->
    <div class="mt-6 flex justify-end gap-3 shrink-0">
      <button
        class="rounded-xl bg-primary-700 px-4 py-2 text-xs text-primary-200 hover:text-white transition-colors hover:bg-primary-600 cursor-pointer"
        type="button"
        :disabled="renaming"
        @click="emit('close')"
      >
        取消
      </button>
      <button
        class="rounded-xl bg-secondary-600 hover:bg-secondary-700 px-5 py-2 text-xs text-white transition-colors disabled:cursor-not-allowed disabled:bg-primary-700 flex items-center gap-2 cursor-pointer font-semibold"
        type="button"
        :disabled="renaming || !canSubmit"
        @click="submit"
      >
        <svg
          v-if="renaming"
          class="w-4 h-4 animate-spin text-white"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          stroke-linecap="round"
        >
          <path :d="mdiLoading" />
        </svg>
        <span>{{ renaming ? "正在重命名…" : "确认重命名" }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from "vue";
import { mdiClose, mdiFolderEdit, mdiLoading } from "@mdi/js";
import type { DirectoryFragment } from "@/graphql/generated";
import { RenameDirectoryDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";
import basename from "@/utils/basename";

// #region 属性与事件定义
const props = defineProps<{
  directory: DirectoryFragment;
}>();

const emit = defineEmits<{
  close: [];
  renamed: [directory: DirectoryFragment];
}>();
// #endregion

// #region 内部状态管理
const newName = ref("");
const renaming = ref(false);

const nameInput = useTemplateRef<HTMLInputElement>("nameInput");

const { showError, showSuccess } = useNotification();

// 当前目录名，用于识别"没有真正改名"的空提交
const currentName = computed(() => basename(props.directory.relPath));
// 去掉首尾空白后的输入值，前端先做一轮校验只为给出即时反馈，后端仍会重复校验
const trimmedName = computed(() => newName.value.trim());
// 只拦截改名无效的空提交（与当前同名），空名等非法输入交给 submit() 给出明确提示
const canSubmit = computed(() => trimmedName.value !== currentName.value);
// #endregion

// #region 提交重命名
async function submit() {
  if (renaming.value) return;
  // 与当前同名属于空提交，回车触发时也要拦下，避免无谓的请求往返
  if (trimmedName.value === currentName.value) return;

  if (trimmedName.value === "") {
    showError("目录名不能为空");
    return;
  }
  if (/[\\/]/.test(trimmedName.value)) {
    showError("目录名不能包含路径分隔符");
    return;
  }
  if (trimmedName.value === "." || trimmedName.value === "..") {
    showError("目录名不能是 . 或 ..");
    return;
  }

  renaming.value = true;
  try {
    const result = await mutate(RenameDirectoryDocument, {
      variables: {
        input: {
          directoryId: props.directory.id,
          newName: trimmedName.value,
        },
      },
    });
    const renamed = result.data?.renameDirectory.directory;
    if (!renamed) return;

    showSuccess(`已重命名为 ${basename(renamed.relPath)}`);
    emit("renamed", renamed);
  } finally {
    renaming.value = false;
  }
}
// #endregion

// 每次打开弹窗时把输入重置为当前目录名并全选，便于直接覆盖输入
function reset() {
  newName.value = currentName.value;
  nameInput.value?.focus();
  nameInput.value?.select();
}

defineExpose({ reset });
</script>
