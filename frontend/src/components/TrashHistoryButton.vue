<template>
  <AppDropdown
    v-if="trashHistoryNodes.length > 0"
    placement="bottom-end"
    content-class="w-80"
  >
    <template #trigger="{ isOpen, toggle }">
      <button
        class="px-4 py-2 text-sm font-medium border rounded-lg transition-all flex items-center gap-2 bg-primary-800/80 hover:bg-primary-700/80 border-primary-700 text-primary-200 hover:text-white cursor-pointer select-none hover:border-red-500/30 active:scale-95"
        :class="[isOpen ? 'border-red-500/30 text-white bg-primary-700' : '']"
        @click="toggle"
      >
        <svg class="w-4 h-4 text-red-400 animate-pulse" viewBox="0 0 24 24">
          <path :d="mdiDelete" fill="currentColor" />
        </svg>
        <span>回收站 ({{ formatTrashSize(totalTrashSize) }})</span>
      </button>
    </template>

    <template #content>
      <div class="space-y-3">
        <!-- 标题栏 -->
        <div class="flex justify-between items-center border-b border-primary-800/50 pb-2">
          <h4 class="text-xs font-bold text-primary-200">回收站</h4>
          <span class="text-xs text-primary-500 font-mono">
            {{ formatTrashSize(totalTrashSize) }}
          </span>
        </div>

        <!-- 历史条目列表 -->
        <div class="max-h-64 overflow-y-auto space-y-2.5 pr-1 text-xs">
          <div
            v-for="item in trashHistoryNodes"
            :key="item.id"
            class="flex items-center gap-3 p-2 bg-primary-800/40 rounded-xl border border-primary-800/30 hover:border-primary-700/30 transition-colors"
          >
            <!-- 图片封面 -->
            <div
              class="w-10 h-10 shrink-0 bg-primary-900/50 rounded-lg overflow-hidden border border-primary-700/30 flex items-center justify-center"
            >
              <img
                v-if="item.coverImage?.url"
                :src="item.coverImage.url"
                class="w-full h-full object-cover"
                alt="封面"
              />
              <svg v-else class="w-5 h-5 text-primary-600" viewBox="0 0 24 24">
                <path :d="mdiFileImage" fill="currentColor" />
              </svg>
            </div>

            <!-- 详细信息 -->
            <div class="flex-1 min-w-0 space-y-0.5">
              <div class="text-primary-200 font-medium truncate flex items-center gap-1">
                <span>{{ item.imageCount }} 张图片</span>
                <span
                  v-if="item.associatedFileCount > 0"
                  class="text-xs text-primary-400 font-normal shrink-0"
                >
                  (+{{ item.associatedFileCount }} 伴随)
                </span>
              </div>
              <!-- 原目录路径 -->
              <div
                class="text-primary-400 truncate flex items-center gap-1"
                :title="item.srcRelPath || '根目录'"
              >
                <svg class="w-3.5 h-3.5 text-primary-500 shrink-0" viewBox="0 0 24 24">
                  <path :d="mdiFolder" fill="currentColor" />
                </svg>
                <span class="truncate">{{ item.srcRelPath || "/" }}</span>
              </div>
              <div class="flex items-center justify-between text-primary-500 font-mono">
                <span>{{ formatTrashSize(item.totalFileSize) }}</span>
                <span>{{ formatTime(item.trashedAt) }}</span>
              </div>
            </div>

            <!-- 撤销按钮 -->
            <button
              class="px-2.5 py-1 text-xs font-semibold bg-secondary-600 hover:bg-secondary-700 text-white rounded-lg transition-colors cursor-pointer shrink-0"
              title="撤销删除，将文件恢复原位"
              @click="undoTrashHistory(item.id)"
            >
              撤销
            </button>
          </div>
        </div>

        <!-- 清空与期限配置区 -->
        <div class="pt-2 border-t border-primary-800/50 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <label class="text-xs text-primary-400 select-none">清空保留期:</label>
            <select
              v-model="trashMinAge"
              class="bg-primary-800 border border-primary-700 text-xs text-primary-200 rounded-lg px-2 py-0.5 focus:outline-none focus:ring-1 focus:ring-secondary-500"
              @change="saveMinAgeSetting"
            >
              <option value="PT5M">5 分钟</option>
              <option value="PT1H">1 小时</option>
              <option value="P1D">1 天</option>
              <option value="P7D">7 天</option>
              <option value="P30D">30 天</option>
            </select>
          </div>

          <button
            class="w-full py-1.5 text-xs font-bold bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 rounded-xl transition-colors cursor-pointer flex items-center justify-center gap-1"
            @click="emptyTrashHistory"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiDeleteSweep" fill="currentColor" />
            </svg>
            <span>清理超过保留期的文件</span>
          </button>
        </div>
      </div>
    </template>
  </AppDropdown>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { mdiDelete, mdiDeleteSweep, mdiFileImage, mdiFolder } from "@mdi/js";
import AppDropdown from "./AppDropdown.vue";
import useTrash from "@/composables/domain/useTrash";
import useNotification from "@/composables/useNotification";
import useStorage from "@/composables/useStorage";
import { formatSize } from "@/utils/formatSize";
import type { TrashHistoryQuery } from "@/graphql/generated";

type TrashHistoryItem = TrashHistoryQuery["trashHistory"]["nodes"][number];

const { model: trashMinAge, flush: saveMinAge } = useStorage<string>(
  localStorage,
  "trash_min_age_duration_t4g7k9",
  () => "P7D",
);

const { showSuccess } = useNotification();
const { data: trashHistoryData, undo: domainUndo, empty: domainEmpty } = useTrash();

async function undoTrashHistory(historyId: string) {
  const result = await domainUndo(historyId);
  if (result) {
    const { restoredCount, conflictCount, conflictDirName } = result;
    if (conflictCount > 0) {
      showSuccess(
        `成功还原了 ${restoredCount} 张图片，另有 ${conflictCount} 个文件存在冲突，已移入对应的 ${conflictDirName} 目录下，请手动处理`,
      );
    } else {
      showSuccess(`成功还原了 ${restoredCount} 张图片及其配套文件`);
    }
  }
}

async function emptyTrashHistory() {
  const result = await domainEmpty(trashMinAge.value);
  if (result) {
    const clearedCount = result.clearedCount;
    showSuccess(`已成功清理 ${clearedCount} 项历史图片及其伴随文件`);
  }
}

const trashHistoryNodes = computed(() => {
  return trashHistoryData.value?.trashHistory?.nodes || [];
});

const totalTrashSize = computed(() => {
  return trashHistoryNodes.value.reduce(
    (acc: number, item: TrashHistoryItem) => acc + item.totalFileSize,
    0,
  );
});

function formatTrashSize(size: number) {
  return formatSize(size);
}

function formatTime(val: string) {
  const d = new Date(val);
  return d.toLocaleString([], {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function saveMinAgeSetting() {
  saveMinAge();
}
</script>
