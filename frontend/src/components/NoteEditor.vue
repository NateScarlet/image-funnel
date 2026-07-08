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
          @click="insertDirective(h.directive!.name)"
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
      <div
        v-if="dispatchableHooks.length"
        ref="dispatchMenuRef"
        class="relative"
      >
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
            <svg
              class="w-4 h-4 text-secondary-400 shrink-0"
              viewBox="0 0 24 24"
            >
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
          v-if="
            (autocompleteState?.show && suggestions.length) ||
            (autocompleteEnabled && dynamicLoading)
          "
          class="fixed z-50 bg-primary-900 border border-primary-700 rounded-xl shadow-2xl p-2 max-h-56 sm:max-h-80 overflow-y-auto min-w-48 max-w-[calc(100vw-2rem)] backdrop-blur-md flex flex-col gap-1"
        >
          <div
            class="px-3 py-2 text-xs font-bold text-primary-400 border-b border-primary-800 uppercase tracking-wider select-none"
          >
            {{
              autocompleteState?.type === "name"
                ? "选择笔记指令"
                : "指令参数建议"
            }}
          </div>
          <div
            v-if="dynamicLoading"
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
            @click="handleSelectSuggestion(sug)"
            @mouseenter="activeIndex = idx"
          >
            <div class="flex items-center gap-2 font-bold text-xs sm:text-sm">
              <span
                :class="[
                  idx === activeIndex ? 'text-white' : 'text-secondary-400',
                  sug.style === 'muted' ? (idx === activeIndex ? 'line-through font-normal opacity-70' : 'line-through font-normal opacity-40') : ''
                ]"
              >
                {{ sug.displayText }}
              </span>
              <span
                v-if="sug.type"
                class="text-[10px] uppercase px-1 rounded font-normal"
                :class="
                  idx === activeIndex
                    ? 'bg-white/20 text-white'
                    : 'bg-primary-800 text-primary-400'
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
        @keydown.enter="handleKeyEnter"
        @keydown.esc.prevent="handleKeyEsc"
        @keydown.space="handleKeySpace"
        @blur="handleBlur"
        @focus="handleFocus"
      ></textarea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, useTemplateRef, shallowRef } from "vue";
import { useFloating, offset, flip, shift, autoUpdate } from "@floating-ui/vue";
import useQuery from "@/graphql/utils/useQuery";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import {
  HooksDocument,
  DispatchNoteHookDocument,
  HookAutocompleteDocument,
} from "@/graphql/generated";
import useTextAreaAutoHeight from "@/composables/useTextAreaAutoHeight";
import useNotification from "@/composables/useNotification";
import useClickOutside from "@/composables/useClickOutside";
import {
  mdiConsole,
  mdiLightningBolt,
  mdiChevronDown,
  mdiLoading,
} from "@mdi/js";
import { debounce } from "es-toolkit";
import useCurrentTime from "@/composables/useCurrentTime";
import Time from "@/utils/Time";
import {
  parseUsage,
  getArgsContext,
  getSuggestionsForRules,
} from "@/utils/directiveAutocomplete";
import type { DirectiveRule, Suggestion } from "@/utils/directiveAutocomplete";

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

const directives = computed(() => {
  return hooksData.value?.hooks.filter((h) => h.directive != null) || [];
});

const currentHook = computed(() => {
  const dirName = autocompleteState.value?.directiveName;
  if (!dirName) return null;
  return (
    hooksData.value?.hooks.find((h) => h.directive?.name === dirName) ?? null
  );
});

const autocompleteEnabled = computed(() => {
  return currentHook.value?.directive?.autocomplete ?? false;
});

// 可即时派发的钩子：canDispatchByNote 且当前笔记已保存（有 noteId）
const dispatchableHooks = computed(() => {
  if (!props.noteId) return [];
  return (
    hooksData.value?.hooks.filter((h) => {
      if (!h.canDispatchByNote) return false;
      if (!h.directive) return false;
      // 检查当前笔记内容中是否包含该指令（要求处于行首，前面只有空格或制表符，且指令后面有单词边界）
      const escapedName = h.directive.name.replace(
        /[.*+?^${}()|[\]\\]/g,
        "\\$&"
      );
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
      showError(
        `保存笔记修改失败: ${err instanceof Error ? err.message : String(err)}`
      );
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

// #region 原始数据
const cursorStart = ref(0);
const cursorEnd = ref(0);
const isFocused = ref(false);
const { currentTime, refreshOn } = useCurrentTime();
const blurAt = shallowRef<Time | null>(null);
const menuVisible = computed(() => {
  if (isFocused.value) return true;
  if (blurAt.value == null) return false;
  return currentTime.value.sub(blurAt.value) < 200;
});
const autocompleteDismissed = ref(false);

function onCursorChange() {
  const el = textareaRef.value;
  if (!el) return;
  cursorStart.value = el.selectionStart;
  cursorEnd.value = el.selectionEnd;
  autocompleteDismissed.value = false;
}
// #endregion

// #region 自动完成状态（纯推导）
const autocompleteState = computed<{
  show: boolean;
  type: "name" | "args";
  query: string;
  triggerIndex: number;
  selectionStart: number;
  directiveName?: string;
  argsText?: string;
} | null>(() => {
  if (!menuVisible.value || autocompleteDismissed.value) return null;

  const text = model.value;
  const start = cursorStart.value;
  const textBeforeCursor = text.slice(0, start);
  const lastNewline = textBeforeCursor.lastIndexOf("\n");
  const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
  const lineTextBeforeCursor = textBeforeCursor.slice(lineStart);

  // 1. 优先匹配指令参数补全
  const directiveMatch = lineTextBeforeCursor.match(
    /^[ \t]*\/([a-zA-Z0-9_-]+)\s+(.*)$/
  );
  if (directiveMatch) {
    const dirName = directiveMatch[1];
    const argsText = directiveMatch[2] ?? "";
    if (directives.value.some((h) => h.directive?.name === dirName)) {
      const { currentQuery } = getArgsContext(argsText);
      return {
        show: true,
        type: "args" as const,
        query: currentQuery,
        triggerIndex:
          lineStart + lineTextBeforeCursor.length - currentQuery.length,
        selectionStart: start,
        directiveName: dirName,
        argsText,
      };
    }
  }

  // 2. 匹配指令名补全
  const nameMatch = lineTextBeforeCursor.match(/^[ \t]*\/([a-zA-Z0-9_-]*)$/);
  if (nameMatch) {
    return {
      show: true,
      type: "name" as const,
      query: nameMatch[1].toLowerCase(),
      triggerIndex: lineStart + lineTextBeforeCursor.indexOf("/") + 1,
      selectionStart: start,
    };
  }

  return null;
});

// activeIndex 声明式重置：query 变化时自动归零
const activeIndexBuffer = ref({ queryKey: "", index: 0 });
const activeIndex = computed({
  get: () => {
    const key = autocompleteState.value?.query ?? "";
    return activeIndexBuffer.value.queryKey === key
      ? activeIndexBuffer.value.index
      : 0;
  },
  set: (val: number) => {
    const key = autocompleteState.value?.query ?? "";
    activeIndexBuffer.value = { queryKey: key, index: val };
  },
});
// #endregion

// #region 动态补全结果（声明式：context 变化时自动归零）
const autocompleteContext = computed(() => {
  const state = autocompleteState.value;
  if (!state || state.type !== "args") return null;
  return `${currentHook.value?.id}|${getLinePrefix()}`;
});

const dynamicBuffer = ref<{
  contextKey: string | null;
  query: string;
  suggestions: Suggestion[];
}>({ contextKey: null, query: "", suggestions: [] });

const apiSuggestions = computed(() => {
  if (dynamicBuffer.value.contextKey !== autocompleteContext.value) return [];
  const state = autocompleteState.value;
  if (!state || state.type !== "args") return [];
  if (!state.query) return dynamicBuffer.value.suggestions;

  // 如果当前的 query 与获取 suggestions 时的 query 相同，说明这是针对当前 query 的一手搜索结果，不需要在本地做字符串过滤
  if (state.query === dynamicBuffer.value.query) {
    return dynamicBuffer.value.suggestions;
  }

  const q = state.query.toLowerCase();
  return dynamicBuffer.value.suggestions.filter((s) => {
    if (s.text.toLowerCase() === q) return false;
    return (
      s.text.toLowerCase().startsWith(q) ||
      s.displayText.toLowerCase().includes(q)
    );
  });
});

const dynamicLoading = ref(false);
// #endregion

const parsedRules = computed<DirectiveRule[]>(() => {
  const rules: DirectiveRule[] = [];
  for (const h of directives.value) {
    if (h.directive?.usage) {
      rules.push(...parseUsage(h.directive.usage));
    }
  }
  return rules;
});

const suggestions = computed<Suggestion[]>(() => {
  if (!autocompleteState.value?.show) return [];

  if (autocompleteState.value.type === "name") {
    const q = autocompleteState.value.query;
    const list = directives.value;
    const matched = q
      ? list.filter((h) => h.directive?.name.toLowerCase().includes(q))
      : list;
    return matched.map((h) => {
      const dirName = h.directive?.name ?? "";
      return {
        type: "subcommand",
        text: dirName,
        displayText: `/${dirName}`,
        description: h.name,
      };
    });
  } else {
    const dirName = autocompleteState.value.directiveName;
    if (!dirName) return [];
    const q = autocompleteState.value.query;
    const rules = parsedRules.value.filter((r) => r.directive === dirName);

    const argsText = autocompleteState.value.argsText ?? "";
    const { confirmedTokens } = getArgsContext(argsText);

    const staticResults = getSuggestionsForRules(rules, confirmedTokens, q);

    // 动态补全结果替换位置参数占位符
    if (
      autocompleteEnabled.value &&
      !autocompleteState.value.query.startsWith("-") &&
      apiSuggestions.value.length > 0
    ) {
      const nonPositional = staticResults.filter(
        (s) => s.type !== "positional"
      );
      return [...apiSuggestions.value, ...nonPositional];
    }

    return staticResults;
  }
});

// 自动高度绑定
useTextAreaAutoHeight(textareaRef, model);

function getLinePrefix(): string {
  const text = model.value;
  const start = cursorStart.value;
  const textBeforeCursor = text.slice(0, start);
  const lastNewline = textBeforeCursor.lastIndexOf("\n");
  const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
  return textBeforeCursor.slice(lineStart);
}

async function executeAutocomplete() {
  const state = autocompleteState.value;
  if (!state || !autocompleteEnabled.value || state.type !== "args") return;
  if (state.query.startsWith("-")) return;

  // 本地缓存足够好（过滤后 ≥50%）就跳过 API 请求
  const buf = dynamicBuffer.value;
  if (
    buf.suggestions.length > 0 &&
    apiSuggestions.value.length >= buf.suggestions.length * 0.5
  ) {
    return;
  }

  const hookId = currentHook.value?.id;
  if (!hookId) return;

  dynamicLoading.value = true;
  try {
    const result = await query(HookAutocompleteDocument, {
      variables: {
        input: {
          hookId,
          noteId: props.noteId,
          linePrefix: getLinePrefix(),

          query: state.query,
        },
      },
    });

    if (result.data?.hookAutocomplete) {
      const results: Suggestion[] = result.data.hookAutocomplete.map(
        (s) => ({
          type: s.type ?? "positional",
          text: s.text,
          displayText: s.displayText,
          description: s.description ?? undefined,
          style: s.style ?? undefined,
        })
      );
      dynamicBuffer.value = {
        contextKey: autocompleteContext.value,
        query: state.query,
        suggestions: results,
      };
    }
  } catch {
    // 全局 ErrorLink 已处理通知
  } finally {
    dynamicLoading.value = false;
  }
}

const triggerDynamicAutocomplete = debounce(executeAutocomplete, 300);

function handleInput() {
  emit("input");
  onCursorChange();
  if (autocompleteState.value?.type === "args") {
    triggerDynamicAutocomplete();
  }
}

function handleSelectSuggestion(sug: Suggestion) {
  const el = textareaRef.value;
  if (!el || !autocompleteState.value) return;

  const text = model.value;
  const triggerIdx = autocompleteState.value.triggerIndex;
  const endIdx = el.selectionEnd;

  let textToInsert = sug.text;
  if (autocompleteState.value.type === "name") {
    textToInsert = `${sug.text} `;
  } else if (sug.type === "option" && !sug.placeholder) {
    textToInsert = `${sug.text} `;
  }

  // 聚焦并选中待替换的内容范围，以便 execCommand 将该范围替换
  el.focus();
  el.setSelectionRange(triggerIdx, endIdx);

  // 使用 insertText 命令插入文本，这能被浏览器原生的撤销历史捕获。
  // 若当前环境不支持该命令（例如 jsdom 测试环境或某些旧浏览器），则安全降级到直接修改 model.value。
  let inserted = false;
  if (typeof document.execCommand === "function") {
    try {
      inserted = document.execCommand("insertText", false, textToInsert);
    } catch {
      // 忽略可能的异常并由 fallback 处理
    }
  }

  if (!inserted) {
    const before = text.slice(0, triggerIdx);
    const after = text.slice(endIdx);
    model.value = before + textToInsert + after;
  }

  let newSelectionStart = triggerIdx + textToInsert.length;
  let newSelectionEnd = newSelectionStart;

  if (sug.placeholder) {
    const placeholderIdx = textToInsert.indexOf(sug.placeholder);
    if (placeholderIdx !== -1) {
      newSelectionStart = triggerIdx + placeholderIdx;
      newSelectionEnd = newSelectionStart + sug.placeholder.length;
    }
  }

  const hasPlaceholder = sug.placeholder !== undefined;

  nextTick(() => {
    el.focus();
    el.setSelectionRange(newSelectionStart, newSelectionEnd);
    onCursorChange();
    emit("input");
    if (hasPlaceholder && autocompleteState.value?.type === "args") {
      triggerDynamicAutocomplete();
      triggerDynamicAutocomplete.flush();
    }
  });
}

function insertDirective(dirName: string) {
  const el = textareaRef.value;
  if (!el) return;

  const text = model.value;
  const start = el.selectionStart;
  const end = el.selectionEnd;

  const before = text.slice(0, start);
  const after = text.slice(end);

  const needsNewline = before.length > 0 && !/(?:^|\n)[ \t]*$/.test(before);
  const prefix = needsNewline ? "\n" : "";
  const textToInsert = prefix + `/${dirName} `;

  el.focus();
  el.setSelectionRange(start, end);

  // 使用 insertText 命令插入文本，以支持快捷指令插入的撤销操作。
  // 若不支持该指令则安全降级到修改 model.value。
  let inserted = false;
  if (typeof document.execCommand === "function") {
    try {
      inserted = document.execCommand("insertText", false, textToInsert);
    } catch {
      // 忽略可能出现的异常并交由 fallback 处理
    }
  }

  if (!inserted) {
    model.value = before + textToInsert + after;
  }

  const newCursorPos = start + textToInsert.length;

  nextTick(() => {
    el.focus();
    el.setSelectionRange(newCursorPos, newCursorPos);
    onCursorChange();
    emit("input");
    // computed autocompleteState 会自动检测到指令，并在 handleInput 中触发动态补全
  });
}

function handleKeyUp() {
  if (autocompleteState.value?.show && suggestions.value.length) {
    activeIndex.value =
      (activeIndex.value - 1 + suggestions.value.length) %
      suggestions.value.length;
  }
}

function handleKeyDown() {
  if (autocompleteState.value?.show && suggestions.value.length) {
    activeIndex.value =
      (activeIndex.value + 1) % suggestions.value.length;
  }
}

function handleKeySpace(e: KeyboardEvent) {
  if (!e.ctrlKey) return;
  e.preventDefault();
  autocompleteDismissed.value = false;

  // 已在指令内 → 立即触发动态补全
  if (autocompleteState.value?.type === "args") {
    triggerDynamicAutocomplete();
    triggerDynamicAutocomplete.flush();
  }
}

function handleKeyEnter(e: KeyboardEvent) {
  if (autocompleteState.value?.show && suggestions.value.length) {
    e.preventDefault();
    handleSelectSuggestion(suggestions.value[activeIndex.value]);
  }
}

function handleKeyEsc() {
  if (autocompleteState.value?.show) {
    autocompleteDismissed.value = true;
  }
}

function handleBlur() {
  isFocused.value = false;
  blurAt.value = Time.now();
  refreshOn(blurAt.value.add(200));
}

function handleFocus() {
  isFocused.value = true;
  blurAt.value = null;
  onCursorChange();
}

function focus() {
  textareaRef.value?.focus();
}

defineExpose({ focus });
</script>
