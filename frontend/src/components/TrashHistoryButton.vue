<template>
  <AppDropdown v-if="trashHistoryNodes.length > 0" placement="bottom-end" content-class="w-80">
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

        <!-- 历史条目列表（滚动到底部自动加载下一页） -->
        <div ref="listContainerRef" class="max-h-64 overflow-y-auto space-y-2.5 pr-1 text-xs">
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
                :width="item.coverImage.width"
                :height="item.coverImage.height"
                class="w-full h-full object-cover"
                alt="封面"
              />
              <svg v-else class="w-5 h-5 text-primary-600" viewBox="0 0 24 24">
                <path :d="mdiFileImage" fill="currentColor" />
              </svg>
            </div>

            <!-- 详细信息 -->
            <div class="flex-1 min-w-0 space-y-0.5">
              <div
                class="text-primary-200 font-medium leading-relaxed break-words"
                :title="item.message || item.srcRelPath"
              >
                {{ item.message || getDirName(item.srcRelPath) }}
              </div>
              <div class="flex items-center gap-1 text-primary-400 text-xs">
                <span>{{ item.imageCount }} 张图片</span>
                <span
                  v-if="item.associatedFileCount > 0"
                  class="text-primary-500 font-normal shrink-0"
                >
                  (+{{ item.associatedFileCount }} 伴随)
                </span>
                <span class="text-primary-600 mx-1">·</span>
                <span>{{ formatTrashSize(item.totalFileSize) }}</span>
              </div>
              <div class="text-primary-500 font-mono text-xs">
                {{ formatTime(item.trashedAt) }}
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

        <!-- 加载更多时底部提示 -->
        <div v-if="loading" class="flex justify-center pt-1">
          <svg class="w-4 h-4 animate-spin text-secondary-500" viewBox="0 0 24 24" fill="none">
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
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
            :class="{ 'pointer-events-none opacity-40 cursor-not-allowed': !canClean }"
            @click="emptyTrashHistory"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
              <path :d="mdiDeleteSweep" fill="currentColor" />
            </svg>
            <span>{{ cleanupButtonText }}</span>
          </button>
        </div>
      </div>
    </template>
  </AppDropdown>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from "vue";
import { mdiDelete, mdiDeleteSweep, mdiFileImage, mdiLoading } from "@mdi/js";
import AppDropdown from "./AppDropdown.vue";
import useTrash from "@/composables/domain/useTrash";
import useInfiniteScroll from "@/composables/useInfiniteScroll";
import useNotification from "@/composables/useNotification";
import useStorage from "@/composables/useStorage";
import useCurrentTime from "@/composables/useCurrentTime";
import Duration from "@/utils/Duration";
import { formatSize } from "@/utils/formatSize";
import basename from "@/utils/basename";
import type { TrashHistoryQuery } from "@/graphql/generated";

type TrashHistoryItem = TrashHistoryQuery["trashHistory"]["nodes"][number];

const { model: trashMinAge, flush: saveMinAge } = useStorage<string>(
  localStorage,
  "trash_min_age_duration_t4g7k9",
  () => "P7D",
);

const { showSuccess } = useNotification();
const loadingCount = ref(0);
const loading = computed(() => loadingCount.value > 0);
const {
  nodes: trashHistoryNodes,
  pageInfo,
  fetchMore,
  undo: domainUndo,
  empty: domainEmpty,
} = useTrash({ loadingCount });

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

const totalTrashSize = computed(() => {
  return trashHistoryNodes.value.reduce(
    (acc: number, item: TrashHistoryItem) => acc + item.totalFileSize,
    0,
  );
});

const { currentTime, refreshOn } = useCurrentTime();

// 将 ISO 8601 保留期转为毫秒数，解析失败时返回极大值防止误判全部过期
const minAgeMs = computed(() => {
  const dur = Duration.parse(trashMinAge.value);
  return dur.valid ? dur.toMilliseconds() : Number.MAX_SAFE_INTEGER;
});

// 每个条目的过期时间戳（trashedAt + minAgeMs）
const itemExpiryTimes = computed(() => {
  const cutoff = minAgeMs.value;
  return trashHistoryNodes.value.map((item) => ({
    size: item.totalFileSize,
    expiryTime: new Date(item.trashedAt).getTime() + cutoff,
  }));
});

// 当前已加载数据中超过保留期的条目总大小
const expiredSize = computed(() => {
  const now = currentTime.value.getTime();
  return itemExpiryTimes.value
    .filter((item) => item.expiryTime <= now)
    .reduce((acc, item) => acc + item.size, 0);
});

// 是否还有下一页数据
const hasNextPage = computed(() => pageInfo.value.hasNextPage);

// 有下一页时后续分页可能存在已过期项，不应仅凭当前已加载部分禁用清理按钮
const canClean = computed(() => expiredSize.value > 0 || hasNextPage.value);

// 清理按钮文本
const cleanupButtonText = computed(() => {
  const size = formatSize(expiredSize.value);
  if (hasNextPage.value) {
    return `清理 ≥${size}`;
  }
  return `清理 ${size}`;
});

const listContainerRef = useTemplateRef<HTMLElement>("listContainerRef");

// 列表滚动到底部时自动加载下一页
useInfiniteScroll(listContainerRef, async () => {
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});

// 监听下一个条目超过保留期的时间点，到达时自动刷新按钮文本
refreshOn(() => {
  const now = currentTime.value.getTime();
  const futureTimes = itemExpiryTimes.value.map((item) => item.expiryTime).filter((t) => t > now);
  if (futureTimes.length === 0) {
    return undefined;
  }
  return Math.min(...futureTimes);
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

function getDirName(relPath: string) {
  return basename(relPath) || "/";
}
</script>
