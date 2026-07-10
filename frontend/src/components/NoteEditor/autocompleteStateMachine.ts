import type { DirectiveRule, Suggestion } from "./directiveAutocomplete";
import { getArgsContext, getSuggestionsForRules } from "./directiveAutocomplete";

// #region 类型定义

export interface AutocompleteState {
  show: boolean;
  type: "name" | "args";
  query: string;
  triggerIndex: number;
  selectionStart: number;
  directiveName?: string;
  argsText?: string;
}

/** 补全确认后返回的插入参数，调用方负责执行 DOM 操作 */
export interface InsertParams {
  textToInsert: string;
  start: number;
  end: number;
  selectStart: number;
  selectEnd: number;
  hasPlaceholder: boolean;
}

/** 传给纯函数的 hook 精简视图，仅包含补全所需字段 */
export interface HookInfo {
  id: string;
  name: string;
  description?: string | null;
  directive?: {
    name: string;
    autocomplete?: boolean | null;
    usage?: string | null;
  } | null;
}

// #endregion

// #region 行前缀提取

/** 获取光标所在行的前缀文本（从行首到光标位置） */
export function getLinePrefix(text: string, cursorStart: number): string {
  const textBeforeCursor = text.slice(0, cursorStart);
  const lastNewline = textBeforeCursor.lastIndexOf("\n");
  const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
  return textBeforeCursor.slice(lineStart);
}

// #endregion

// #region 状态推导

/** 从文本和光标位置推导当前自动完成状态（纯函数，无副作用） */
export function computeAutocompleteState(
  text: string,
  cursorStart: number,
  menuVisible: boolean,
  dismissed: boolean,
  directiveNames: string[],
): AutocompleteState | null {
  if (!menuVisible || dismissed) return null;

  const textBeforeCursor = text.slice(0, cursorStart);
  const lastNewline = textBeforeCursor.lastIndexOf("\n");
  const lineStart = lastNewline === -1 ? 0 : lastNewline + 1;
  const lineTextBeforeCursor = textBeforeCursor.slice(lineStart);

  // 优先匹配指令参数补全
  const directiveMatch = lineTextBeforeCursor.match(/^[ \t]*\/([a-zA-Z0-9_-]+)\s+(.*)$/);
  if (directiveMatch) {
    const dirName = directiveMatch[1];
    const argsText = directiveMatch[2] ?? "";
    if (directiveNames.includes(dirName)) {
      const { currentQuery } = getArgsContext(argsText);
      return {
        show: true,
        type: "args",
        query: currentQuery,
        triggerIndex: lineStart + lineTextBeforeCursor.length - currentQuery.length,
        selectionStart: cursorStart,
        directiveName: dirName,
        argsText,
      };
    }
  }

  // 匹配指令名补全
  const nameMatch = lineTextBeforeCursor.match(/^[ \t]*\/([a-zA-Z0-9_-]*)$/);
  if (nameMatch) {
    return {
      show: true,
      type: "name",
      query: nameMatch[1].toLowerCase(),
      triggerIndex: lineStart + lineTextBeforeCursor.indexOf("/") + 1,
      selectionStart: cursorStart,
    };
  }

  return null;
}

// #endregion

// #region API 建议过滤

/** 将 API 返回的原始建议过滤/合并为当前查询可用的建议列表（纯函数） */
export function computeApiSuggestions(
  state: AutocompleteState | null,
  apiQueryVars: { input?: { hookId?: string; linePrefix?: string; query?: string } } | undefined,
  rawApiSuggestions: ReadonlyArray<{
    type?: string | null;
    text: string;
    displayText: string;
    description?: string | null;
    style?: string | null;
  }>,
  currentHookId: string | null,
  linePrefix: string,
): Suggestion[] {
  if (!state || state.type !== "args") return [];

  if (
    !apiQueryVars ||
    apiQueryVars.input?.hookId !== currentHookId ||
    apiQueryVars.input?.linePrefix !== linePrefix
  ) {
    return [];
  }

  const suggestionsMapped: Suggestion[] = rawApiSuggestions.map((s) => ({
    type: s.type ?? "positional",
    text: s.text,
    displayText: s.displayText,
    description: s.description ?? undefined,
    style: s.style ?? undefined,
  }));

  const queryVal = apiQueryVars.input?.query ?? "";

  if (!state.query) return suggestionsMapped;

  // 如果当前 query 与 API 请求的 query 相同，直接返回
  if (state.query === queryVal) {
    return suggestionsMapped;
  }

  // 否则本地过滤
  const q = state.query.toLowerCase();
  return suggestionsMapped.filter((item) => {
    if (item.text.toLowerCase() === q) return false;
    return item.text.toLowerCase().startsWith(q) || item.displayText.toLowerCase().includes(q);
  });
}

// #endregion

// #region 建议合并

/** 合并静态规则建议和 API 建议，生成最终补全列表（纯函数） */
export function computeSuggestions(
  state: AutocompleteState | null,
  hooks: HookInfo[],
  parsedRules: DirectiveRule[],
  apiSuggestions: Suggestion[],
  enabled: boolean,
): Suggestion[] {
  if (!state?.show) return [];

  if (state.type === "name") {
    const q = state.query;
    const list = hooks.filter((h) => h.directive?.name != null);
    const matched = q
      ? list.filter((h) => (h.directive?.name ?? "").toLowerCase().includes(q))
      : list;
    return matched.map((h) => {
      const dirName = h.directive?.name ?? "";
      const relatedRule = parsedRules.find((r) => r.directive === dirName);

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
  }

  const dirName = state.directiveName;
  if (!dirName) return [];

  const q = state.query;
  const rules = parsedRules.filter((r) => r.directive === dirName);

  const argsText = state.argsText ?? "";
  const { confirmedTokens } = getArgsContext(argsText);

  const staticResults = getSuggestionsForRules(rules, confirmedTokens, q);

  if (
    enabled &&
    !q.startsWith("-") &&
    apiSuggestions.length > 0
  ) {
    const nonPositional = staticResults.filter((item) => item.type !== "positional");
    return [...apiSuggestions, ...nonPositional];
  }

  return staticResults;
}

// #endregion

// #region 动态加载判断

/** 判断是否需要触发新的 API 查询（纯函数） */
export function needsDynamicLoading(
  state: AutocompleteState | null,
  enabled: boolean,
  apiQueryVars: { input?: { hookId?: string; linePrefix?: string; query?: string } } | undefined,
  currentHookId: string | null,
  linePrefix: string,
  rawApiSuggestions: ReadonlyArray<{ text: string; displayText: string }>,
): boolean {
  if (!state || !enabled || state.type !== "args") return false;
  if (state.query.startsWith("-")) return false;

  if (
    !apiQueryVars ||
    apiQueryVars.input?.hookId !== currentHookId ||
    apiQueryVars.input?.linePrefix !== linePrefix
  ) {
    return true;
  }

  if (rawApiSuggestions.length === 0) return true;

  const queryVal = apiQueryVars.input?.query ?? "";

  let filteredLength: number;
  if (state.query === queryVal) {
    filteredLength = rawApiSuggestions.length;
  } else {
    const q = state.query.toLowerCase();
    filteredLength = rawApiSuggestions.filter((item) => {
      if (item.text.toLowerCase() === q) return false;
      return item.text.toLowerCase().startsWith(q) || item.displayText.toLowerCase().includes(q);
    }).length;
  }

  return filteredLength < rawApiSuggestions.length * 0.5;
}

// #endregion

// #region 插入参数计算

/** 计算选择建议后的插入参数（纯函数，不执行 DOM 操作） */
export function computeSuggestionInsertion(
  sug: Suggestion,
  state: AutocompleteState,
  selectionEnd: number,
): InsertParams {
  const triggerIdx = state.triggerIndex;
  const endIdx = selectionEnd;

  let textToInsert = sug.text;
  if (state.type === "name") {
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

  return {
    textToInsert,
    start: triggerIdx,
    end: endIdx,
    selectStart: newSelectionStart,
    selectEnd: newSelectionEnd,
    hasPlaceholder: sug.placeholder !== undefined,
  };
}

/** 计算插入指令的插入参数（纯函数，不执行 DOM 操作） */
export function computeDirectiveInsertion(
  dirName: string,
  text: string,
  selectionStart: number,
  selectionEnd: number,
): InsertParams {
  const before = text.slice(0, selectionStart);
  const needsNewline = before.length > 0 && !/(?:^|\n)[ \t]*$/.test(before);
  const prefix = needsNewline ? "\n" : "";
  const textToInsert = prefix + `/${dirName} `;

  const newCursorPos = selectionStart + textToInsert.length;
  return {
    textToInsert,
    start: selectionStart,
    end: selectionEnd,
    selectStart: newCursorPos,
    selectEnd: newCursorPos,
    hasPlaceholder: false,
  };
}

// #endregion