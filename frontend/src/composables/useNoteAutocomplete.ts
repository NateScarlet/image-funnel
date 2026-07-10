import { ref, computed, watch, shallowRef, toValue } from "vue";
import type { Ref, MaybeRefOrGetter } from "vue";

import useQuery from "@/graphql/utils/useQuery";
import {
  HooksDocument,
  HookAutocompleteDocument,
  type HookAutocompleteQuery,
  type HookAutocompleteQueryVariables,
} from "@/graphql/generated";
import useCurrentTime from "@/composables/useCurrentTime";
import Time from "@/utils/Time";
import { parseUsage, getArgsContext, getSuggestionsForRules } from "@/utils/directiveAutocomplete";
import type { DirectiveRule, Suggestion } from "@/utils/directiveAutocomplete";
import { debounce } from "es-toolkit";

export interface TextareaOperator {
  selectionStart: number;
  selectionEnd: number;
  insertText(
    textToInsert: string,
    start: number,
    end: number,
    selectStart: number,
    selectEnd: number,
    hasPlaceholder: boolean
  ): void;
}

function mapToSuggestion(s: HookAutocompleteQuery["hookAutocomplete"][number]): Suggestion {
  return {
    type: s.type ?? "positional",
    text: s.text,
    displayText: s.displayText,
    description: s.description ?? undefined,
    style: s.style ?? undefined,
  };
}

export interface Options {
  model: MaybeRefOrGetter<string>;
  cursorStart: MaybeRefOrGetter<number>;
  cursorEnd: MaybeRefOrGetter<number>;
  isFocused: MaybeRefOrGetter<boolean>;
  noteId?: MaybeRefOrGetter<string | undefined>;
  loadingCount?: Ref<number>;
}

export function useNoteAutocomplete(options: Options) {
  const { currentTime, refreshOn } = useCurrentTime();
  const blurAt = shallowRef<Time | null>(null);
  const dismissed = ref(false);

  // 补全可见性
  const menuVisible = computed(() => {
    if (toValue(options.isFocused)) return true;
    if (blurAt.value == null) return false;
    return currentTime.value.sub(blurAt.value) < 200;
  });

  // 获取所有 Hooks 列表
  const { data: hooksData } = useQuery(HooksDocument, {
    fetchPolicy: "cache-first",
  });

  const directives = computed(() => {
    return hooksData.value?.hooks.filter((h) => h.directive != null) || [];
  });

  const currentHook = computed(() => {
    const dirName = state.value?.directiveName;
    if (!dirName) return null;
    return hooksData.value?.hooks.find((h) => h.directive?.name === dirName) ?? null;
  });

  const enabled = computed(() => {
    return currentHook.value?.directive?.autocomplete ?? false;
  });

  // 自动完成状态（纯推导，仅从 options.model 中读取，不修改）
  const state = computed<{
    show: boolean;
    type: "name" | "args";
    query: string;
    triggerIndex: number;
    selectionStart: number;
    directiveName?: string;
    argsText?: string;
  } | null>(() => {
    if (!menuVisible.value || dismissed.value) return null;

    const text = toValue(options.model);
    const start = toValue(options.cursorStart);
    const textBeforeCursor = text.slice(0, start);
    const lastNewline = textBeforeCursor.lastIndexOf("\n");
    const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
    const lineTextBeforeCursor = textBeforeCursor.slice(lineStart);

    // 1. 优先匹配指令参数补全
    const directiveMatch = lineTextBeforeCursor.match(/^[ \t]*\/([a-zA-Z0-9_-]+)\s+(.*)$/);
    if (directiveMatch) {
      const dirName = directiveMatch[1];
      const argsText = directiveMatch[2] ?? "";
      if (directives.value.some((h) => h.directive?.name === dirName)) {
        const { currentQuery } = getArgsContext(argsText);
        return {
          show: true,
          type: "args" as const,
          query: currentQuery,
          triggerIndex: lineStart + lineTextBeforeCursor.length - currentQuery.length,
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

  // activeIndex 声明式重置：query 变化时自动重置为 -1
  const activeIndexBuffer = ref({ queryKey: "", index: -1 });
  const activeIndex = computed({
    get: () => {
      const key = state.value?.query ?? "";
      return activeIndexBuffer.value.queryKey === key ? activeIndexBuffer.value.index : -1;
    },
    set: (val: number) => {
      const key = state.value?.query ?? "";
      activeIndexBuffer.value = { queryKey: key, index: val };
    },
  });

  const apiSuggestions = computed(() => {
    const s = state.value;
    if (!s || s.type !== "args") return [];

    const currentVars = variables.value;
    const currentPrefix = getLinePrefix();
    if (
      !currentVars ||
      currentVars.input?.hookId !== currentHook.value?.id ||
      currentVars.input?.linePrefix !== currentPrefix
    ) {
      return [];
    }

    const rawSuggestions = autocompleteData.value?.hookAutocomplete ?? [];
    const suggestionsMapped = rawSuggestions.map(mapToSuggestion);
    const queryVal = currentVars.input?.query ?? "";

    if (!s.query) return suggestionsMapped;

    // 如果当前的 query 与获取 suggestions 时的 query 相同，说明是一手数据
    if (s.query === queryVal) {
      return suggestionsMapped;
    }

    const q = s.query.toLowerCase();
    return suggestionsMapped.filter((item) => {
      if (item.text.toLowerCase() === q) return false;
      return item.text.toLowerCase().startsWith(q) || item.displayText.toLowerCase().includes(q);
    });
  });

  const dynamicLoadingCount = ref(0);
  const dynamicLoading = computed(() => (options.loadingCount?.value ?? dynamicLoadingCount.value) > 0);

  const variablesRaw = computed(() => {
    const s = state.value;
    if (!toValue(options.isFocused) || !s || !enabled.value || s.type !== "args") {
      return null;
    }
    if (s.query.startsWith("-")) {
      return null;
    }
    return {
      input: {
        hookId: currentHook.value?.id ?? "",
        noteId: toValue(options.noteId),
        linePrefix: getLinePrefix(),
        query: s.query,
      },
    };
  });

  const variables = ref<HookAutocompleteQueryVariables | undefined>(undefined);

  const updateVariablesDebounced = debounce((val) => {
    variables.value = val;
  }, 300);

  watch(
    variablesRaw,
    (newVal) => {
      if (!newVal) {
        updateVariablesDebounced.cancel();
        variables.value = undefined;
      } else {
        updateVariablesDebounced(newVal);
      }
    },
    { immediate: true }
  );

  const { data: autocompleteData } = useQuery(HookAutocompleteDocument, {
    variables: variables,
    loadingCount: options.loadingCount ?? dynamicLoadingCount,
    fetchPolicy: "cache-first",
  });


  const isDebouncing = computed(() => {
    return (
      variablesRaw.value !== null &&
      variablesRaw.value !== variables.value
    );
  });

  const needsDynamicLoading = computed(() => {
    const s = state.value;
    if (!s || !enabled.value || s.type !== "args") return false;
    if (s.query.startsWith("-")) return false;

    const currentVars = variables.value;
    const currentPrefix = getLinePrefix();
    if (
      !currentVars ||
      currentVars.input?.hookId !== currentHook.value?.id ||
      currentVars.input?.linePrefix !== currentPrefix
    ) {
      return true;
    }

    const rawSuggestions = autocompleteData.value?.hookAutocomplete ?? [];
    if (rawSuggestions.length === 0) return true;

    const queryVal = currentVars.input?.query ?? "";

    let filteredLength = 0;
    if (s.query === queryVal) {
      filteredLength = rawSuggestions.length;
    } else {
      const q = s.query.toLowerCase();
      filteredLength = rawSuggestions.filter((item) => {
        if (item.text.toLowerCase() === q) return false;
        return item.text.toLowerCase().startsWith(q) || item.displayText.toLowerCase().includes(q);
      }).length;
    }

    return filteredLength < rawSuggestions.length * 0.5;
  });

  const isSearching = computed(() => {
    return dynamicLoading.value || (isDebouncing.value && needsDynamicLoading.value);
  });

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
    if (!state.value?.show) return [];

    if (state.value.type === "name") {
      const q = state.value.query;
      const list = directives.value;
      const matched = q ? list.filter((h) => h.directive?.name.toLowerCase().includes(q)) : list;
      return matched.map((h) => {
        const dirName = h.directive?.name ?? "";
        const relatedRule = parsedRules.value.find((r) => r.directive === dirName);

        const header = h.name;
        const body = relatedRule?.generalDescription || h.description || "";
        const desc = body ? `${header}\n\n${body}` : header;

        return {
          type: "subcommand",
          text: dirName,
          displayText: `/${dirName}`,
          description: desc,
        };
      });
    } else {
      const dirName = state.value.directiveName;
      if (!dirName) return [];
      const q = state.value.query;
      const rules = parsedRules.value.filter((r) => r.directive === dirName);

      const argsText = state.value.argsText ?? "";
      const { confirmedTokens } = getArgsContext(argsText);

      const staticResults = getSuggestionsForRules(rules, confirmedTokens, q);

      if (
        enabled.value &&
        !state.value.query.startsWith("-") &&
        apiSuggestions.value.length > 0
      ) {
        const nonPositional = staticResults.filter((item) => item.type !== "positional");
        return [...apiSuggestions.value, ...nonPositional];
      }

      return staticResults;
    }
  });

  function getLinePrefix(): string {
    const text = toValue(options.model);
    const start = toValue(options.cursorStart);
    const textBeforeCursor = text.slice(0, start);
    const lastNewline = textBeforeCursor.lastIndexOf("\n");
    const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
    return textBeforeCursor.slice(lineStart);
  }

  function resetDismissed() {
    dismissed.value = false;
  }

  function onFocus() {
    blurAt.value = null;
    resetDismissed();
  }

  function onBlur() {
    blurAt.value = Time.now();
    refreshOn(blurAt.value.add(200));
  }

  function handleSelectSuggestion(sug: Suggestion, op: TextareaOperator) {
    const s = state.value;
    if (!s) return;

    const triggerIdx = s.triggerIndex;
    const endIdx = op.selectionEnd;

    let textToInsert = sug.text;
    if (s.type === "name") {
      textToInsert = `${sug.text} `;
    } else if (sug.type === "option" && !sug.placeholder) {
      textToInsert = `${sug.text} `;
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
    op.insertText(textToInsert, triggerIdx, endIdx, newSelectionStart, newSelectionEnd, hasPlaceholder);
  }

  function insertDirective(dirName: string, op: TextareaOperator) {
    const text = toValue(options.model);
    const start = op.selectionStart;
    const end = op.selectionEnd;

    const before = text.slice(0, start);
    const needsNewline = before.length > 0 && !/(?:^|\n)[ \t]*$/.test(before);
    const prefix = needsNewline ? "\n" : "";
    const textToInsert = prefix + `/${dirName} `;

    const newCursorPos = start + textToInsert.length;
    op.insertText(textToInsert, start, end, newCursorPos, newCursorPos, false);
  }

  function handleKeyUp() {
    if (state.value?.show && suggestions.value.length) {
      if (activeIndex.value === -1) {
        activeIndex.value = suggestions.value.length - 1;
      } else {
        activeIndex.value =
          (activeIndex.value - 1 + suggestions.value.length) % suggestions.value.length;
      }
    }
  }

  // 下键
  function handleKeyDown() {
    if (state.value?.show && suggestions.value.length) {
      if (activeIndex.value === -1) {
        activeIndex.value = 0;
      } else {
        activeIndex.value = (activeIndex.value + 1) % suggestions.value.length;
      }
    }
  }

  function handleKeySpace(e: KeyboardEvent) {
    if (!e.ctrlKey) return;
    e.preventDefault();
    dismissed.value = false;
    updateVariablesDebounced.flush();
    activeIndex.value = 0;
  }

  function handleKeyEnter(e: KeyboardEvent, op: TextareaOperator) {
    if (state.value?.show && suggestions.value.length && activeIndex.value !== -1) {
      const sug = suggestions.value[activeIndex.value];
      const text = toValue(options.model);
      const triggerIdx = state.value.triggerIndex;
      const endIdx = op.selectionEnd;

      let textToInsert = sug.text;
      if (state.value.type === "name") {
        textToInsert = `${sug.text} `;
      } else if (sug.type === "option" && !sug.placeholder) {
        textToInsert = `${sug.text} `;
      }

      if (textToInsert === text.slice(triggerIdx, endIdx)) {
        return;
      }

      e.preventDefault();
      handleSelectSuggestion(sug, op);
    }
  }

  watch(
    () => state.value?.show,
    (show) => {
      if (!show) {
        updateVariablesDebounced.cancel();
      }
    }
  );

  function handleKeyEsc() {
    if (state.value?.show) {
      dismissed.value = true;
    }
  }

  function flushDebounced() {
    updateVariablesDebounced.flush();
  }

  return {
    suggestions,
    activeIndex,
    isSearching,
    state,
    dismissed,
    resetDismissed,
    onFocus,
    onBlur,
    handleSelectSuggestion,
    insertDirective,
    handleKeyUp,
    handleKeyDown,
    handleKeySpace,
    handleKeyEnter,
    handleKeyEsc,
    flushDebounced,
    directives,
  };
}
