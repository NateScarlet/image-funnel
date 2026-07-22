<template>
  <div>
    <!-- 头部标题区域 -->
    <div class="mb-6 flex justify-between items-center">
      <div>
        <h2 class="text-lg font-bold text-primary-50 flex items-center gap-2">
          <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiFolderMove" fill="currentColor" />
          </svg>
          移动匹配图片
        </h2>
        <p class="mt-2 text-xs text-primary-400">
          将当前筛选匹配的图片及其配套伴随文件移动到新目录
        </p>
      </div>
      <button
        class="text-primary-400 hover:text-primary-200 transition-colors p-2 rounded-lg hover:bg-primary-700/50 cursor-pointer"
        type="button"
        @click="$emit('close')"
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
        <span class="font-bold">{{ isApproximate ? "> " : "" }}{{ matchCount }} 张</span>
        <p class="mt-1 text-xs text-primary-400 leading-relaxed">
          提示：图片对应的配套伴随文件（如同名
          .txt，.json，或者带有当前图片完整名称及额外扩展名的文件）也将同步移动。
        </p>
      </div>

      <!-- 操作模式选择 -->
      <div>
        <label class="mb-2 block text-xs font-semibold text-primary-300"> 操作模式 </label>
        <div
          class="grid grid-cols-2 gap-2 bg-primary-900/50 p-1 rounded-xl border border-primary-800"
        >
          <button
            type="button"
            class="py-2 text-xs font-medium rounded-lg transition-all cursor-pointer flex items-center justify-center gap-2"
            :class="
              !toTrash
                ? 'bg-primary-700 text-white shadow'
                : 'text-primary-400 hover:text-primary-200'
            "
            :disabled="moving"
            @click="toTrash = false"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiFolderMove" fill="currentColor" />
            </svg>
            移动到新目录
          </button>
          <button
            type="button"
            class="py-2 text-xs font-medium rounded-lg transition-all cursor-pointer flex items-center justify-center gap-2"
            :class="
              toTrash
                ? 'bg-primary-700 text-white shadow'
                : 'text-primary-400 hover:text-primary-200'
            "
            :disabled="moving"
            @click="toTrash = true"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiDelete" fill="currentColor" />
            </svg>
            删除
          </button>
        </div>
      </div>

      <!-- 目标目录输入 -->
      <PathSelector
        v-if="!toTrash"
        v-model="pathInput"
        :directory-id="directoryId"
        :disabled="moving"
        @submit="handleMoveImages"
      />

      <!-- 移至回收站说明 -->
      <div v-else class="space-y-3">
        <div
          class="rounded-xl bg-primary-900/20 border border-primary-800/20 p-4 text-xs text-primary-300 leading-relaxed flex gap-2 items-start"
        >
          <svg class="w-4 h-4 text-secondary-400 shrink-0 mt-0.5" viewBox="0 0 24 24">
            <path :d="mdiDelete" fill="currentColor" />
          </svg>
          <div>
            <span class="font-medium text-primary-200">说明：</span>
            图片及其配套的伴随文件将被移动到根目录下的回收站目录中。
            此操作完全支持撤销，您也可以在回收站历史页面中随时将其永久清空。
          </div>
        </div>

        <!-- 删除消息输入 -->
        <div>
          <label class="mb-2 block text-xs font-semibold text-primary-300">
            删除说明（可选）
          </label>
          <textarea
            v-model="trashMessage"
            class="w-full bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-xs text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all px-3 py-2 resize-none"
            placeholder="添加删除说明，帮助理解删除上下文…"
            rows="2"
            :disabled="moving"
          ></textarea>
        </div>
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
        class="rounded-xl bg-primary-700 px-4 py-2 text-xs text-primary-200 hover:text-white transition-colors hover:bg-primary-600 cursor-pointer"
        type="button"
        :disabled="moving"
        @click="$emit('close')"
      >
        取消
      </button>
      <button
        class="rounded-xl bg-secondary-600 hover:bg-secondary-700 px-5 py-2 text-xs text-white transition-colors disabled:cursor-not-allowed disabled:bg-primary-700 flex items-center gap-2 cursor-pointer font-semibold"
        type="button"
        :disabled="moving || (!toTrash && !pathInput)"
        @click="handleMoveImages"
      >
        <svg v-if="moving" class="w-4 h-4 animate-spin text-white" viewBox="0 0 24 24" fill="none">
          <path
            :d="mdiLoading"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
            stroke-linecap="round"
          />
        </svg>
        <span>{{ moving ? "正在移动…" : "确认移动" }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { mdiFolderMove, mdiClose, mdiLoading, mdiDelete } from "@mdi/js";
import mutate from "@/graphql/utils/mutate";
import { MoveImagesDocument, type ImageFiltersInput, type PathInput } from "@/graphql/generated";
import { useOpenDir } from "@/composables/useOpenDir";
import useNotification from "@/composables/useNotification";
import useTrash from "@/composables/domain/useTrash";
import PathSelector from "./PathSelector.vue";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
  filterBy: ImageFiltersInput;
  matchCount: number;
  isApproximate?: boolean;
}>();

const emit = defineEmits<(e: "close") => void>();
// #endregion

// #region 内部状态管理
const pathInput = ref<PathInput | null>(null);
const toTrash = ref(false);
const moving = ref(false);
const moveError = ref("");
const trashMessage = ref("");

const { show: showNotification } = useNotification();
const { revealInExplorer } = useOpenDir();
const { trashImages } = useTrash();
// #endregion

// #region 执行移动图片操作
async function handleMoveImages() {
  if (!toTrash.value && !pathInput.value) return;
  if (moving.value) return;

  moving.value = true;
  moveError.value = "";

  try {
    if (toTrash.value) {
      await trashImages(props.directoryId, props.filterBy, trashMessage.value || undefined);
      emit("close");
    } else {
      const toDir = pathInput.value;
      if (!toDir) return;

      const result = await mutate(MoveImagesDocument, {
        variables: {
          input: {
            directoryId: props.directoryId,
            filterBy: props.filterBy,
            toDirectory: toDir,
          },
        },
      });

      const movedCount = result.data?.moveImages.movedCount ?? 0;
      const targetAbsoluteDirectory = result.data?.moveImages.targetAbsoluteDirectory;

      emit("close");

      // 弹出成功通知，带有触发用户手势的打开资源管理器按钮
      showNotification(
        `成功移动了 ${movedCount} 张图片及其配套文件`,
        "success",
        8000,
        targetAbsoluteDirectory
          ? [
              {
                text: "在资源管理器中打开",
                onClick: (closeNotification) => {
                  revealInExplorer(targetAbsoluteDirectory);
                  closeNotification();
                },
              },
            ]
          : undefined,
      );
    }
  } catch (err: unknown) {
    moveError.value = err instanceof Error ? err.message : String(err);
  } finally {
    moving.value = false;
  }
}
// #endregion
</script>
