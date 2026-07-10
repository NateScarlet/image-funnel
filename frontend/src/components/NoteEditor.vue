<template>
  <div class="flex flex-col w-full text-left">
    <!-- 快捷指令按钮栏 + 派发按钮 -->
    <div
      v-if="directives.length || dispatchableHooks.length"
      class="flex flex-wrap gap-2 mb-3 items-center select-none text-left"
    >
      <template v-if="directives.length">
        <span
          class="text-xs font-bold text-primary-400 uppercase tracking-wider flex items-center gap-1"
        >
          <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiConsole" fill="currentColor" />
          </svg>
          常用指令:
        </span>
        <button
          v-for="h in directives"
          :key="h.id"
          type="button"
          class="px-2 py-1 text-xs font-semibold rounded-lg bg-primary-800 hover:bg-primary-700 border border-primary-700/60 hover:border-secondary-500/50 text-primary-300 hover:text-white transition-all active:scale-95 cursor-pointer flex items-center gap-1 group"
          :title="h.directive!.usage"
          @click="onInsertDirective(h.directive!.name)"
        >
          <span class="text-secondary-400 group-hover:text-secondary-300"
            >/{{ h.directive!.name }}</span
          >
          <span
            class="text-xs scale-90 origin-left font-normal text-primary-500 group-hover:text-primary-400"
            >({{ h.name }})</span
          >
        </button>
      </template>

      <!-- 派发按钮：可即时触发的笔记钩子，放入二级菜单防止误触 -->
      <div v-if="dispatchableHooks.length" ref="dispatchMenuRef" class="relative">
        <button
          type="button"
          class="px-2 py-1 text-xs font-semibold rounded-lg bg-secondary-900/60 hover:bg-secondary-800 border border-secondary-700/60 hover:border-secondary-500/50 text-secondary-300 hover:text-white transition-all active:scale-95 cursor-pointer flex items-center gap-1"
          :disabled="isDispatching"
          @click.stop="showDispatchMenu = !showDispatchMenu"
        >
          <svg class="w-3.5 h-3.5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiLightningBolt" fill="currentColor" />
          </svg>
          <span>动作 ({{ dispatchableHooks.length }})</span>
          <svg
            class="w-3 h-3 text-secondary-400 transition-transform"
            :class="{ 'rotate-180': showDispatchMenu }"
            viewBox="0 0 24 24"
          >
            <path :d="mdiChevronDown" fill="currentColor" />
          </svg>
        </button>

        <!-- 下拉菜单 -->
        <div
          v-if="showDispatchMenu"
          class="absolute top-full left-0 mt-2 z-50 bg-primary-900 border border-primary-700 rounded-xl shadow-2xl p-2 w-fit min-w-40 flex flex-col gap-1"
          @click.stop
        >
          <div
            class="px-3 py-2 text-xs font-bold text-primary-400 border-b border-primary-800 uppercase tracking-wider select-none"
          >
            选择动作
          </div>
          <button
            v-for="h in dispatchableHooks"
            :key="'dispatch-' + h.id"
            type="button"
            class="text-left px-3 py-2 rounded-lg flex items-center gap-2 transition-colors text-sm font-medium text-secondary-300 hover:bg-secondary-800 hover:text-white cursor-pointer whitespace-nowrap"
            :disabled="isDispatching"
            @click="
              triggerDispatch(h.id, h.name);
              showDispatchMenu = false;
            "
          >
            <svg class="w-4 h-4 text-secondary-400 shrink-0" viewBox="0 0 24 24">
              <path :d="mdiLightningBolt" fill="currentColor" />
            </svg>
            <span>{{ h.name }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 文本编辑区 -->
    <div class="relative w-full group">
      <!-- 自动完成浮层 -->
      <Teleport to="body">
        <div
          ref="floatingEl"
          :style="floatingStyles"
          v-if="autocompleteState?.show && (suggestions.length || isSearching)"
          class="fixed z-50 bg-primary-900 border border-primary-700 rounded-xl shadow-2xl p-2 max-h-56 sm:max-h-80 overflow-y-auto min-w-48 max-w-[calc(100vw-2rem)] backdrop-blur-md flex flex-col gap-1"
        >
          <div
            class="px-3 py-2 text-xs font-bold text-primary-400 border-b border-primary-800 uppercase tracking-wider select-none"
          >
            {{ autocompleteState?.type === "name" ? "选择笔记指令" : "指令参数建议" }}
          </div>
          <div
            v-if="isSearching"
            class="px-3 py-2 text-xs text-primary-400 flex items-center gap-2"
          >
            <svg class="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24">
              <path :d="mdiLoading" fill="currentColor" />
            </svg>
            加载中...
          </div>
          <button
            v-for="(sug, idx) in suggestions"
            :key="idx"
            type="button"
            class="w-full text-left px-3 py-2 rounded-lg flex flex-col transition-colors cursor-pointer"
            :class="
              idx === activeIndex
                ? 'bg-secondary-500 text-white'
                : 'hover:bg-primary-800 text-primary-200'
            "
            @click="onSelectSuggestion(sug)"
            @mouseenter="activeIndex = idx"
          >
            <div class="flex items-center gap-2 font-bold text-xs sm:text-sm">
              <span
                :class="[
                  idx === activeIndex ? 'text-white' : 'text-secondary-400',
                  sug.style === 'muted'
                    ? idx === activeIndex
                      ? 'line-through font-normal opacity-70'
                      : 'line-through font-normal opacity-40'
                    : '',
                ]"
              >
                {{ sug.displayText }}
              </span>
              <span
                v-if="sug.type"
                class="text-[10px] uppercase px-1 rounded font-normal"
                :class="
                  idx === activeIndex ? 'bg-white/20 text-white' : 'bg-primary-800 text-primary-400'
                "
              >
                {{ typeLabels[sug.type] || sug.type }}
              </span>
            </div>
            <div
              v-if="sug.description"
              class="text-xs opacity-80 wrap-break-word whitespace-pre-wrap"
              :class="idx === activeIndex ? 'text-white' : 'text-primary-400'"
            >
              {{ sug.description }}
            </div>
          </button>
        </div>
      </Teleport>

      <textarea
        ref="textarea"
        v-model="model"
        class="w-full bg-primary-800/50 hover:bg-primary-800 focus:bg-primary-800 border border-primary-700 focus:border-secondary-500/50 rounded-xl px-4 py-3 sm:px-8 sm:py-6 text-sm sm:text-xl text-primary-100 placeholder-primary-500 outline-none transition-all duration-300 resize-none leading-relaxed min-h-30 sm:min-h-60 overflow-y-auto"
        :placeholder="placeholder"
        data-no-gesture
        @input="handleInput"
        @keyup="onCursorChange"
        @click="onCursorChange"
        @keydown.up.prevent="handleKeyUp"
        @keydown.down.prevent="handleKeyDown"
        @keydown.enter="onKeyEnter"
        @keydown.esc.prevent="handleKeyEsc"
        @keydown.space="handleKeySpace"
        @blur="handleBlur"
        @focus="handleFocus"
      ></textarea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, useTemplateRef } from "vue";

import { useFloating, offset, flip, shift, autoUpdate } from "@floating-ui/vue";
import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";
import {
  HooksDocument,
  DispatchNoteHookDocument,
} from "@/graphql/generated";
import useTextAreaAutoHeight from "@/composables/useTextAreaAutoHeight";
import useNotification from "@/composables/useNotification";
import useClickOutside from "@/composables/useClickOutside";
import { mdiConsole, mdiLightningBolt, mdiChevronDown, mdiLoading } from "@mdi/js";
import { useNoteAutocomplete, computeDirectiveInsertion } from "@/composables/useNoteAutocomplete";
import type { InsertParams } from "@/composables/useNoteAutocomplete";
import type { Suggestion } from "@/utils/directiveAutocomplete";

const typeLabels: Record<string, string> = {
  subcommand: "子命令",
  positional: "参数",
  option: "选项",
};

const props = defineProps<{
  placeholder?: string;
  /** 当前笔记 ID，为空时不显示派发按钮 */
  noteId?: string;
  onBeforeDispatch?: () => Promise<void>;
}>();

const model = defineModel<string>({ required: true });

const emit = defineEmits<(e: "input") => void>();

const textareaRef = useTemplateRef<HTMLTextAreaElement>("textarea");
const floatingEl = ref<HTMLElement | null>(null);

const { floatingStyles } = useFloating(textareaRef, floatingEl, {
  placement: "top-start",
  strategy: "fixed",
  whileElementsMounted: autoUpdate,
  middleware: [
    offset({
      mainAxis: 8,
      crossAxis: 16,
    }),
    flip(),
    shift(),
  ],
});

// 使用声明式缓存加载，避免多余的组件内部状态
const { data: hooksData } = useQuery(HooksDocument, {
  fetchPolicy: "cache-first",
});

// 可即时派发的钩子：canDispatchByNote 且当前笔记已保存（有 noteId）
const dispatchableHooks = computed(() => {
  if (!props.noteId) return [];
  return (
    hooksData.value?.hooks.filter((h) => {
      if (!h.canDispatchByNote) return false;
      if (!h.directive) return false;
      // 检查当前笔记内容中是否包含该指令（要求处于行首，前面只有空格或制表符，且指令后面有单词边界）
      const escapedName = h.directive.name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const regex = new RegExp(`(?:^|\\r?\\n)[ \\t]*/${escapedName}\\b`);
      return regex.test(model.value);
    }) || []
  );
});

const { showSuccess, showError, showInfo, remove } = useNotification();
const isDispatching = ref(false);

// 下拉菜单状态
const showDispatchMenu = ref(false);
const dispatchMenuRef = useTemplateRef<HTMLDivElement>("dispatchMenuRef");

useClickOutside(dispatchMenuRef, () => {
  showDispatchMenu.value = false;
});

async function triggerDispatch(hookId: string, hookName: string) {
  if (!props.noteId || isDispatching.value) return;
  isDispatching.value = true;

  if (props.onBeforeDispatch) {
    try {
      await props.onBeforeDispatch();
    } catch (err) {
      showError(`保存笔记修改失败: ${err instanceof Error ? err.message : String(err)}`);
      isDispatching.value = false;
      return;
    }
  }

  const infoNotificationId = showInfo(`正在执行动作 ${hookName}...`, 0);

  try {
    await mutate(DispatchNoteHookDocument, {
      variables: {
        input: {
          hookId,
          noteId: props.noteId,
        },
      },
    });

    showSuccess(`动作 ${hookName} 已成功触发`);
  } finally {
    remove(infoNotificationId);
    isDispatching.value = false;
  }
}

// #region 自动补全逻辑接入
const cursorStart = ref(0);
const cursorEnd = ref(0);
const isFocused = ref(false);
const dynamicLoadingCount = ref(0);

const {
  suggestions,
  activeIndex,
  isSearching,
  state: autocompleteState,
  resetDismissed,
  onFocus: onAutocompleteFocus,
  onBlur: onAutocompleteBlur,
  handleSelectSuggestion,
  handleKeyUp,
  handleKeyDown,
  handleKeySpace,
  handleKeyEnter,
  handleKeyEsc,
  flushDebounced,
  directives,
} = useNoteAutocomplete({
  model: () => model.value,
  cursorStart: () => cursorStart.value,
  cursorEnd: () => cursorEnd.value,
  isFocused: () => isFocused.value,
  noteId: computed(() => props.noteId),
  loadingCount: dynamicLoadingCount,
  hooksData,
});

/** 执行文本插入操作 */
function applyInsertion(params: InsertParams) {
  const el = textareaRef.value;
  if (!el) return;

  el.focus();
  el.setSelectionRange(params.start, params.end);

  let inserted = false;
  if (typeof document.execCommand === "function") {
    try {
      inserted = document.execCommand("insertText", false, params.textToInsert);
    } catch {
      // 忽略
    }
  }

  if (!inserted) {
    const before = model.value.slice(0, params.start);
    const after = model.value.slice(params.end);
    model.value = before + params.textToInsert + after;
  }

  nextTick(() => {
    el.focus();
    el.setSelectionRange(params.selectStart, params.selectEnd);
    onCursorChange();
    emit("input");
    if (params.hasPlaceholder) {
      flushDebounced();
    }
  });
}

/** 选择建议后的回调 */
function onSelectSuggestion(sug: Suggestion) {
  const p = handleSelectSuggestion(sug, textareaRef.value?.selectionEnd ?? 0);
  if (p) applyInsertion(p);
}

/** Enter 键确认 */
function onKeyEnter(e: KeyboardEvent) {
  const p = handleKeyEnter(e, textareaRef.value?.selectionEnd ?? 0);
  if (p) applyInsertion(p);
}

/** 插入指令（纯函数计算 + DOM 执行） */
function onInsertDirective(dirName: string) {
  const params = computeDirectiveInsertion(
    dirName,
    model.value,
    textareaRef.value?.selectionStart ?? 0,
    textareaRef.value?.selectionEnd ?? 0,
  );
  applyInsertion(params);
}

function onCursorChange() {
  const el = textareaRef.value;
  if (!el) return;
  cursorStart.value = el.selectionStart;
  cursorEnd.value = el.selectionEnd;
  resetDismissed();
}

function handleInput() {
  emit("input");
  onCursorChange();
}

function handleFocus() {
  isFocused.value = true;
  onAutocompleteFocus();
  onCursorChange();
}

function handleBlur() {
  isFocused.value = false;
  onAutocompleteBlur();
}

// 自动高度绑定
useTextAreaAutoHeight(textareaRef, model);

function focus() {
  textareaRef.value?.focus();
}

defineExpose({ focus });
</script>