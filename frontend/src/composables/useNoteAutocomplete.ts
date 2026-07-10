import { ref, computed, watch, shallowRef, toValue } from "vue";
import type { Ref, MaybeRefOrGetter } from "vue";

import useQuery from "@/graphql/utils/useQuery";
import {
  HooksDocument,
  HookAutocompleteDocument,
  type HookAutocompleteQueryVariables,
  type HooksQuery,
} from "@/graphql/generated";
import useCurrentTime from "@/composables/useCurrentTime";
import Time from "@/utils/Time";
import { parseUsage } from "@/utils/directiveAutocomplete";
import type { DirectiveRule, Suggestion } from "@/utils/directiveAutocomplete";
import {
  computeAutocompleteState,
  computeApiSuggestions,
  computeSuggestions,
  computeSuggestionInsertion,
  computeDirectiveInsertion,
  getLinePrefix,
  needsDynamicLoading,
  type AutocompleteState,
  type InsertParams,
  type HookInfo,
} from "@/utils/autocompleteStateMachine";
import { debounce } from "es-toolkit";

export type { AutocompleteState, InsertParams, HookInfo };

export interface Options {
  model: MaybeRefOrGetter<string>;
  cursorStart: MaybeRefOrGetter<number>;
  cursorEnd: MaybeRefOrGetter<number>;
  isFocused: MaybeRefOrGetter<boolean>;
  noteId?: MaybeRefOrGetter<string | undefined>;
  loadingCount?: Ref<number>;
  /** 外部传入的 hooks 数据，避免内部重复查询 */
  hooksData?: Ref<HooksQuery | undefined>;
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

  // 获取所有 Hooks 列表（若外部传入则直接使用，避免重复查询）
  const internalHooksData = useQuery(HooksDocument, {
    fetchPolicy: "cache-first",
  });
  const hooksData = computed(() => {
    if (options.hooksData) return options.hooksData.value;
    return internalHooksData.data.value;
  });

  const directives = computed(() => {
    return hooksData.value?.hooks.filter((h) => h.directive != null) || [];
  });

  const directiveNames = computed(() =>
    directives.value.map((h) => h.directive?.name ?? ""),
  );

  const currentHook = computed(() => {
    const dirName = state.value?.directiveName;
    if (!dirName) return null;
    return hooksData.value?.hooks.find((h) => h.directive?.name === dirName) ?? null;
  });

  const enabled = computed(() => {
    return currentHook.value?.directive?.autocomplete ?? false;
  });

  // 自动完成状态（委托给纯函数）
  const state = computed<AutocompleteState | null>(() =>
    computeAutocompleteState(
      toValue(options.model),
      toValue(options.cursorStart),
      menuVisible.value,
      dismissed.value,
      directiveNames.value,
    ),
  );

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
        linePrefix: getLinePrefix(toValue(options.model), toValue(options.cursorStart)),
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
    { immediate: true },
  );

  const { data: autocompleteData } = useQuery(HookAutocompleteDocument, {
    variables: variables,
    loadingCount: options.loadingCount ?? dynamicLoadingCount,
    fetchPolicy: "cache-first",
  });

  const isDebouncing = computed(() => {
    return variablesRaw.value !== null && variablesRaw.value !== variables.value;
  });

  const currentLinePrefix = computed(() =>
    getLinePrefix(toValue(options.model), toValue(options.cursorStart)),
  );

  // API 建议（委托给纯函数）
  const apiSuggestions = computed(() =>
    computeApiSuggestions(
      state.value,
      variables.value as { input?: { hookId?: string; linePrefix?: string; query?: string } } | undefined,
      autocompleteData.value?.hookAutocomplete ?? [],
      currentHook.value?.id ?? null,
      currentLinePrefix.value,
    ),
  );

  // 动态加载判断（委托给纯函数）
  const needsDynamic = computed(() =>
    needsDynamicLoading(
      state.value,
      enabled.value,
      variables.value as { input?: { hookId?: string; linePrefix?: string; query?: string } } | undefined,
      currentHook.value?.id ?? null,
      currentLinePrefix.value,
      autocompleteData.value?.hookAutocomplete ?? [],
    ),
  );

  const isSearching = computed(() => {
    return dynamicLoading.value || (isDebouncing.value && needsDynamic.value);
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

  // 建议合并（委托给纯函数）
  const suggestions = computed<Suggestion[]>(() =>
    computeSuggestions(
      state.value,
      directives.value as HookInfo[],
      parsedRules.value,
      apiSuggestions.value,
      enabled.value,
    ),
  );

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

  // #region 键盘交互

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

  function handleKeyEsc() {
    if (state.value?.show) {
      dismissed.value = true;
    }
  }

  // #endregion

  // #region 建议提交（纯计算，返回 InsertParams，调用方负责 DOM 操作）

  /** 计算选择建议后的插入参数，返回 null 表示无操作 */
  function handleSelectSuggestion(sug: Suggestion, selectionEnd: number): InsertParams | null {
    const s = state.value;
    if (!s) return null;
    return computeSuggestionInsertion(sug, s, selectionEnd);
  }

  /** 计算 Enter 键确认后的插入参数，返回 null 表示无需处理（由调用方让事件继续传播） */
  function handleKeyEnter(e: KeyboardEvent, selectionEnd: number): InsertParams | null {
    if (state.value?.show && suggestions.value.length && activeIndex.value !== -1) {
      const sug = suggestions.value[activeIndex.value];
      const s = state.value;
      const text = toValue(options.model);
      const params = computeSuggestionInsertion(sug, s, selectionEnd);

      if (params.textToInsert === text.slice(params.start, params.end)) {
        return null;
      }

      e.preventDefault();
      return params;
    }
    return null;
  }

  // #endregion

  watch(
    () => state.value?.show,
    (show) => {
      if (!show) {
        updateVariablesDebounced.cancel();
      }
    },
  );

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
    handleKeyUp,
    handleKeyDown,
    handleKeySpace,
    handleKeyEnter,
    handleKeyEsc,
    flushDebounced,
    directives,
  };
}

/** 计算插入指令的插入参数（纯函数，不执行 DOM 操作） */
export { computeDirectiveInsertion };