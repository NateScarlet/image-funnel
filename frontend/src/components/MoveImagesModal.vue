<template>
  <ModalDialogWrapper
    container-class="sm:max-w-md p-6"
    @after-leave="emit('afterLeave')"
  >
    <!-- 头部标题区域 -->
    <div class="mb-6 flex justify-between items-center">
      <div>
        <h2 class="text-lg font-bold text-primary-50 flex items-center gap-2">
          <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiFolderMove" fill="currentColor" />
          </svg>
          移动匹配图片
        </h2>
        <p class="mt-1.5 text-xs text-primary-400">
          将当前筛选匹配的图片及其配套伴随文件移动到新目录
        </p>
      </div>
      <button
        class="text-primary-400 hover:text-primary-200 transition-colors p-1.5 rounded-lg hover:bg-primary-700/50 cursor-pointer"
        type="button"
        @click="close"
      >
        <svg class="w-5 h-5" viewBox="0 0 24 24">
          <path :d="mdiClose" fill="currentColor" />
        </svg>
      </button>
    </div>

    <!-- 表单输入与数量显示区 -->
    <div class="space-y-4">
      <!-- 匹配图片数量展示 -->
      <div
        class="rounded-xl bg-primary-900/40 border border-primary-800/30 p-4 text-sm text-primary-200 leading-relaxed"
      >
        <span class="font-medium text-secondary-400">待移动图片：</span>
        <span class="font-bold">{{ matchCount }} 张</span>
        <p class="mt-1 text-xs text-primary-400 leading-relaxed">
          提示：图片对应的配套伴随文件（如同名
          .txt，.json，或者带有当前图片完整名称及额外扩展名的文件）也将同步移动。
        </p>
      </div>

      <!-- 目标目录输入 -->
      <div>
        <label class="mb-2 block text-xs font-semibold text-primary-300">
          目标目录名称（相对于当前目录）
        </label>
        <input
          v-model="targetDirInput"
          type="text"
          placeholder="例如：selected 或 ../sibling-dir"
          class="w-full rounded-xl border border-primary-700 hover:border-primary-600 bg-primary-850 px-4 py-2.5 text-xs text-white placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 focus:border-secondary-500 transition-all"
          :disabled="moving"
          @keyup.enter="handleMoveImages"
        />
      </div>

      <!-- 错误信息提示 -->
      <div
        v-if="moveError"
        class="text-xs text-red-400 bg-red-950/40 border border-red-900/50 p-3 rounded-xl leading-relaxed"
      >
        {{ moveError }}
      </div>
    </div>

    <!-- 操作按钮区 -->
    <div class="mt-6 flex justify-end gap-3 shrink-0">
      <button
        class="rounded-xl bg-primary-750 px-4 py-2 text-xs text-primary-200 hover:text-white transition-colors hover:bg-primary-700 cursor-pointer"
        type="button"
        :disabled="moving"
        @click="close"
      >
        取消
      </button>
      <button
        class="rounded-xl bg-secondary-600 hover:bg-secondary-700 px-5 py-2 text-xs text-white transition-colors disabled:cursor-not-allowed disabled:bg-primary-700 flex items-center gap-2 cursor-pointer font-semibold"
        type="button"
        :disabled="moving || !targetDirInput.trim()"
        @click="handleMoveImages"
      >
        <svg
          v-if="moving"
          class="w-4.5 h-4.5 animate-spin text-white"
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
        <span>{{ moving ? "正在移动..." : "确认移动" }}</span>
      </button>
    </div>
  </ModalDialogWrapper>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { mdiFolderMove, mdiClose, mdiLoading } from "@mdi/js";
import useModalDialog from "@/composables/useModalDialog";
import mutate from "@/graphql/utils/mutate";
import {
  MoveImagesDocument,
  type ImageFiltersInput,
} from "@/graphql/generated";
import { useOpenDir } from "@/composables/useOpenDir";
import useNotification from "@/composables/useNotification";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
  filterBy: ImageFiltersInput;
  matchCount: number;
}>();

const emit = defineEmits<(e: "afterLeave") => void>();
// #endregion

// #region 内部状态管理
const targetDirInput = ref("");
const moving = ref(false);
const moveError = ref("");

const { show: showNotification } = useNotification();
const { revealInExplorer } = useOpenDir();
// #endregion

// #region 使用 useModalDialog Composable 声明组件包装器
const {
  component: ModalDialogWrapper,
  open,
  close,
} = useModalDialog({
  onDidClose() {
    targetDirInput.value = "";
    moveError.value = "";
  },
});
// #endregion

// #region 执行移动图片操作
async function handleMoveImages() {
  const dirName = targetDirInput.value.trim();
  if (!dirName || moving.value) return;

  moving.value = true;
  moveError.value = "";

  try {
    const result = await mutate(MoveImagesDocument, {
      variables: {
        input: {
          directoryId: props.directoryId,
          filterBy: props.filterBy,
          toDirectoryRelPath: dirName,
        },
      },
    });

    const movedCount = result.data?.moveImages.movedCount ?? 0;
    const targetAbsoluteDirectory =
      result.data?.moveImages.targetAbsoluteDirectory;

    close();

    // 弹出成功通知，带有触发用户手势的打开资源管理器按钮
    showNotification(
      `成功移动了 ${movedCount} 张图片及其配套文件`,
      "success",
      8000,
      targetAbsoluteDirectory
        ? {
            text: "在资源管理器中打开",
            onClick: (closeNotification) => {
              revealInExplorer(targetAbsoluteDirectory);
              closeNotification();
            },
          }
        : undefined,
    );
  } catch (err: unknown) {
    moveError.value =
      err instanceof Error ? err.message : "移动图片失败，请检查路径或权限";
  } finally {
    moving.value = false;
  }
}
// #endregion

// #region 暴露方法供外部组件使用
defineExpose({
  open,
  close,
});
// #endregion
</script>
