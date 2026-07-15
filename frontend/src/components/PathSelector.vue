<template>
  <div class="relative">
    <label class="mb-2 block text-xs font-semibold text-primary-300"> 目标目录路径 </label>

    <!-- 路径输入模式选择 -->
    <div
      class="mb-2 flex bg-primary-900/50 rounded-lg p-0.5 text-xs w-fit border border-primary-800"
    >
      <button
        type="button"
        class="px-2.5 py-1 rounded-md transition-all cursor-pointer font-medium"
        :class="
          pathMode === 'current'
            ? 'bg-primary-700 text-white shadow'
            : 'text-primary-400 hover:text-primary-200'
        "
        :disabled="disabled"
        @click="pathMode = 'current'"
      >
        相对当前目录
      </button>
      <button
        type="button"
        class="px-2.5 py-1 rounded-md transition-all cursor-pointer font-medium"
        :class="
          pathMode === 'root'
            ? 'bg-primary-700 text-white shadow'
            : 'text-primary-400 hover:text-primary-200'
        "
        :disabled="disabled"
        @click="pathMode = 'root'"
      >
        相对项目根目录
      </button>
    </div>

    <div class="relative">
      <input
        ref="inputEl"
        v-model="targetDirInput"
        name="path"
        autocomplete="off"
        type="text"
        :placeholder="
          pathMode === 'current'
            ? '例如：selected 或 ../sibling'
            : pathMode === 'root'
              ? '例如：folder/subfolder'
              : '例如：C:\\Workspaces\\image-funnel\\dest'
        "
        class="w-full rounded-xl border border-primary-700 hover:border-primary-600 bg-primary-800 px-4 py-2 text-xs text-white placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 focus:border-secondary-500 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="disabled"
        @focus="onFocus"
        @blur="onBlur"
        @keydown.down.prevent="onKeyDownDown"
        @keydown.up.prevent="onKeyDownUp"
        @keydown.enter.prevent="onKeyDownEnter"
      />

      <!-- 联想建议气泡列表 -->
      <Teleport to="body">
        <div
          v-if="showSuggestions && suggestions.length > 0"
          ref="floatingEl"
          :style="floatingStyles"
          class="z-[100] max-h-56 overflow-y-auto rounded-xl bg-primary-800 border border-primary-700 shadow-2xl py-1 divide-y divide-primary-700/40"
        >
          <button
            v-for="(item, idx) in suggestions"
            :key="item.id"
            type="button"
            class="w-full text-left px-3 py-2 flex items-center gap-3 transition-colors hover:bg-primary-700/60 cursor-pointer text-primary-200 group"
            :class="{
              'bg-primary-700/80 text-white': idx === activeSuggestionIndex,
            }"
            @mousedown="selectSuggestion(item)"
          >
            <!-- 左侧：封面/文件夹图标 -->
            <div
              class="relative w-10 h-10 rounded-lg overflow-hidden bg-primary-900/60 border border-primary-700 shrink-0 flex items-center justify-center"
            >
              <template v-if="getCachedStats(item.id)?.latestImage">
                <img
                  :src="getCachedStats(item.id)!.latestImage!.url256"
                  :width="getCachedStats(item.id)!.latestImage!.width"
                  :height="getCachedStats(item.id)!.latestImage!.height"
                  class="w-full h-full object-cover"
                  alt="Cover"
                />
              </template>
              <template v-else>
                <!-- 默认文件夹 SVG 图标 -->
                <svg
                  class="w-5 h-5 text-primary-400 group-hover:text-secondary-400 transition-colors"
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M19,20H5V8H19M19,6H12L10,4H4C2.89,4 2,4.89 2,6V18A2,2 0 0,0 4,20H19A2,2 0 0,0 21,18V8A2,2 0 0,0 19,6Z"
                    fill="currentColor"
                  />
                </svg>
              </template>

              <!-- 如果正在加载，且数据还没返回，在图标上加一个加载小圆圈 -->
              <div
                v-if="isStatsLoading(item.id) && !getCachedStats(item.id)"
                class="absolute inset-0 bg-primary-800/80 flex items-center justify-center"
              >
                <div
                  class="w-4 h-4 rounded-full border-2 border-secondary-500 border-t-transparent animate-spin"
                ></div>
              </div>
            </div>

            <!-- 中右侧：路径与统计信息 -->
            <div class="flex-1 min-w-0 flex flex-col justify-between py-0.5">
              <div
                class="truncate text-xs font-medium text-primary-100 group-hover:text-white transition-colors"
              >
                {{ item.relPath }}
              </div>
              <div class="flex items-center gap-2 mt-0.5 text-xs">
                <template v-if="getCachedStats(item.id)">
                  <span class="text-primary-300">
                    {{ getCachedStats(item.id)!.imageCount }} 张图片
                  </span>
                  <span
                    v-if="getCachedStats(item.id)!.subdirectoryCount > 0"
                    class="text-primary-400"
                  >
                    • {{ getCachedStats(item.id)!.subdirectoryCount }} 个子目录
                  </span>
                </template>
                <template v-else>
                  <span class="text-primary-500 animate-pulse">计算中…</span>
                </template>
              </div>
            </div>
          </button>
        </div>
      </Teleport>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { debounce } from "es-toolkit";
import { useFloating, offset, flip, shift, autoUpdate, size } from "@floating-ui/vue";
import useQuery from "@/graphql/utils/useQuery";
import { SuggestDirectoriesDocument, type PathInput } from "@/graphql/generated";
import { useDirectoryStats } from "@/composables/domain/useDirectoryBrowse";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
  disabled?: boolean;
  modelValue?: PathInput | null;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", val: PathInput | null): void;
  (e: "submit"): void;
}>();
// #endregion

// #region 状态管理与声明式缓存区
const showSuggestions = ref(false);
const activeSuggestionIndex = ref(-1);
const isFocused = ref(false);

// 定位相关的 DOM 引用与 useFloating 初始化
const inputEl = ref<HTMLInputElement | null>(null);
const floatingEl = ref<HTMLElement | null>(null);

const { floatingStyles } = useFloating(inputEl, floatingEl, {
  placement: "bottom-start",
  strategy: "fixed",
  whileElementsMounted: autoUpdate,
  middleware: [
    offset({ mainAxis: 4 }),
    flip({
      fallbackPlacements: ["top-start", "bottom-start"],
    }),
    shift(),
    size({
      apply({ rects, elements }) {
        Object.assign(elements.floating.style, {
          width: `${rects.reference.width}px`,
        });
      },
    }),
  ],
});

const { getCachedStats, refetchStats, isStatsLoading } = useDirectoryStats();

// 声明式局部缓冲区：通过对比 base 和 props.modelValue 来自动清空/复位缓存，零副作用
const queryBuffer = ref<{
  base: PathInput | null | undefined;
  current: PathInput | null | undefined;
}>({
  base: undefined,
  current: undefined,
});

// 辅助函数：只读比较两个 PathInput 的内容是否完全相同，防止多余更新与回流
function isSamePath(a: PathInput | null | undefined, b: PathInput | null | undefined): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return (
    a.absolute === b.absolute &&
    a.relativeToRoot === b.relativeToRoot &&
    a.relativeToCurrent === b.relativeToCurrent
  );
}

const activePathInput = computed<PathInput | null>(() => {
  // 如果当前缓存的 base 与外部 modelValue 是同一个值，返回 current
  if (isSamePath(queryBuffer.value.base, props.modelValue)) {
    return queryBuffer.value.current ?? null;
  }
  // 否则，说明外部 props 发生了变动，直接退回 props 驱动
  return props.modelValue ?? null;
});

const targetDirInput = computed({
  get: () => {
    const p = activePathInput.value;
    if (!p) return "";
    return p.absolute ?? p.relativeToRoot ?? p.relativeToCurrent ?? "";
  },
  set: (val) => {
    const mode = pathMode.value;
    const nextPath: PathInput =
      mode === "absolute"
        ? { absolute: val }
        : mode === "root"
          ? { relativeToRoot: val }
          : { relativeToCurrent: val };

    queryBuffer.value = {
      base: props.modelValue,
      current: nextPath,
    };

    // 触发 emit 传回父组件
    const trimmed = val.trim();
    const emitPath = trimmed ? nextPath : null;
    if (!isSamePath(emitPath, props.modelValue)) {
      emit("update:modelValue", emitPath);
    }

    // 触发建议检索变量更新
    if (!trimmed) {
      debouncedPathInput.value = undefined;
    } else {
      updateDebouncedInput(nextPath);
    }
  },
});

const pathMode = computed<"current" | "root" | "absolute">({
  get: () => {
    const p = activePathInput.value;
    if (!p) return "current";
    if (p.absolute !== undefined) return "absolute";
    if (p.relativeToRoot !== undefined) return "root";
    return "current";
  },
  set: (val) => {
    const input = targetDirInput.value;
    const nextPath: PathInput =
      val === "absolute"
        ? { absolute: input }
        : val === "root"
          ? { relativeToRoot: input }
          : { relativeToCurrent: input };

    queryBuffer.value = {
      base: props.modelValue,
      current: nextPath,
    };

    // 触发 emit 传回父组件
    const trimmed = input.trim();
    const emitPath = trimmed ? nextPath : null;
    if (!isSamePath(emitPath, props.modelValue)) {
      emit("update:modelValue", emitPath);
    }

    // 触发建议检索变量更新
    if (!trimmed) {
      debouncedPathInput.value = undefined;
    } else {
      updateDebouncedInput(nextPath);
    }
  },
});
// #endregion

// #region 声明式自动建议检索
const debouncedPathInput = ref<PathInput | undefined>(undefined);

const updateDebouncedInput = debounce((val: PathInput) => {
  if (isFocused.value) {
    debouncedPathInput.value = val;
  }
}, 300);

// 使用项目原装 useQuery 自动跟踪变量变化并发送请求，零手动 fetchSuggestions 逻辑
const { data: suggestionsData } = useQuery(SuggestDirectoriesDocument, {
  variables: () => {
    const input = debouncedPathInput.value;
    if (!input) return undefined;
    return {
      directoryId: props.directoryId,
      input,
    };
  },
  fetchPolicy: "network-only",
});

const suggestions = computed(() => {
  return suggestionsData.value?.suggestDirectories ?? [];
});

// 在联想建议返回时触发异步统计缓存拉取
watch(suggestions, (items) => {
  if (items.length > 0) {
    void refetchStats(items.map((item) => item.id));
  }
});
// #endregion

// #region 事件处理器
function onFocus() {
  isFocused.value = true;
  showSuggestions.value = true;
  if (targetDirInput.value.trim()) {
    // 聚焦时若有内容，立即更新检索变量触发首轮联想，免去防抖延迟
    const p = activePathInput.value;
    if (p) {
      debouncedPathInput.value = p;
    }
  }
}

// 延迟失焦以允许 mousedown 事件先触发
function onBlur() {
  setTimeout(() => {
    isFocused.value = false;
    showSuggestions.value = false;
    debouncedPathInput.value = undefined; // 释放数据请求状态
  }, 200);
}

function selectSuggestion(item: { id: string; relPath: string }) {
  // 选中后直接基于相对项目根的路径进行覆盖，这会自动完成外层 emit 与缓存绑定
  const emitPath = { relativeToRoot: item.relPath };
  queryBuffer.value = {
    base: props.modelValue,
    current: emitPath,
  };

  if (!isSamePath(emitPath, props.modelValue)) {
    emit("update:modelValue", emitPath);
  }

  showSuggestions.value = false;
  activeSuggestionIndex.value = -1;
  debouncedPathInput.value = undefined; // 释放建议查询
}

function onKeyDownDown() {
  if (!suggestions.value.length) return;
  activeSuggestionIndex.value = (activeSuggestionIndex.value + 1) % suggestions.value.length;
}

function onKeyDownUp() {
  if (!suggestions.value.length) return;
  activeSuggestionIndex.value =
    (activeSuggestionIndex.value - 1 + suggestions.value.length) % suggestions.value.length;
}

function onKeyDownEnter() {
  if (activeSuggestionIndex.value >= 0 && activeSuggestionIndex.value < suggestions.value.length) {
    selectSuggestion(suggestions.value[activeSuggestionIndex.value]);
  } else {
    emit("submit");
  }
}
// #endregion
</script>
