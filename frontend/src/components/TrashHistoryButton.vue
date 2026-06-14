<template>
  <div v-if="trashHistoryNodes.length > 0" class="relative group/trash-history">
    <button
      class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1.5 bg-primary-800/80 hover:bg-primary-700/80 border-primary-700 text-primary-200 cursor-pointer select-none hover:border-red-500/30"
    >
      <svg class="w-4 h-4 text-red-400 animate-pulse" viewBox="0 0 24 24">
        <path :d="mdiDelete" fill="currentColor" />
      </svg>
      <span>回收站历史 ({{ formatTrashSize(totalTrashSize) }})</span>
    </button>

    <!-- 气泡内容面板 -->
    <div
      class="absolute top-full right-0 mt-2 invisible group-hover/trash-history:visible opacity-0 group-hover/trash-history:opacity-100 transition-all duration-300 bg-primary-900/95 backdrop-blur-xl border border-primary-700/80 p-4 rounded-2xl shadow-[0_10px_30px_rgba(0,0,0,0.8)] z-60 w-80 space-y-3"
    >
      <div
        class="flex justify-between items-center border-b border-primary-800/50 pb-2"
      >
        <h4 class="text-xs font-bold text-primary-200">回收站</h4>
        <span class="text-[10px] text-primary-500 font-mono">
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
          <div class="flex-1 min-w-0">
            <div
              class="text-primary-200 font-medium truncate flex items-center gap-1"
            >
              <span>{{ item.imageCount }} 张图片</span>
              <span
                v-if="item.associatedFileCount > 0"
                class="text-[10px] text-primary-400 font-normal shrink-0"
              >
                (+{{ item.associatedFileCount }} 伴随)
              </span>
            </div>
            <div
              class="flex items-center justify-between text-[10px] text-primary-500 mt-0.5 font-mono"
            >
              <span>{{ formatTrashSize(item.totalFileSize) }}</span>
              <span>{{ formatTime(item.trashedAt) }}</span>
            </div>
          </div>

          <!-- 撤销按钮 -->
          <button
            class="px-2.5 py-1 text-[10px] font-semibold bg-secondary-600 hover:bg-secondary-700 text-white rounded-lg transition-colors cursor-pointer shrink-0"
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
          <label class="text-[10px] text-primary-400 select-none"
            >清空保留期:</label
          >
          <select
            v-model="trashMinAge"
            class="bg-primary-800 border border-primary-700 text-[10px] text-primary-200 rounded-lg px-2 py-0.5 focus:outline-none focus:ring-1 focus:ring-secondary-500"
            @change="saveMinAgeSetting"
          >
            <option value="PT0S">不保留（立即）</option>
            <option value="PT5M">5 分钟</option>
            <option value="PT1H">1 小时</option>
            <option value="P1D">1 天</option>
            <option value="P7D">7 天</option>
            <option value="P30D">30 天</option>
          </select>
        </div>

        <button
          class="w-full py-1.5 text-[10px] font-bold bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 rounded-xl transition-colors cursor-pointer flex items-center justify-center gap-1"
          @click="emptyTrashHistory"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
            <path :d="mdiDeleteSweep" fill="currentColor" />
          </svg>
          <span>立即清空回收站</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { mdiDelete, mdiDeleteSweep, mdiFileImage } from "@mdi/js";
import useTrashHistory, {
  trashMinAge,
  saveMinAge,
} from "@/composables/useTrashHistory";
import { formatSize } from "@/utils/formatSize";
import type { TrashHistoryQuery } from "@/graphql/generated";

type TrashHistoryItem = TrashHistoryQuery["trashHistory"]["nodes"][number];

const {
  data: trashHistoryData,
  undo: undoTrashHistory,
  empty: emptyTrashHistory,
} = useTrashHistory();

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
