import { describe, test, expect } from "vitest";
import {
  getLinePrefix,
  computeAutocompleteState,
  computeApiSuggestions,
  computeSuggestions,
  needsDynamicLoading,
  computeSuggestionInsertion,
  computeDirectiveInsertion,
  type AutocompleteState,
  type HookInfo,
  type InsertParams,
} from "./autocompleteStateMachine";
import { parseUsage, type DirectiveRule, type Suggestion } from "./directiveAutocomplete";

// #region 测试数据

const hookList: HookInfo[] = [
  {
    id: "hook1",
    name: "调整图片",
    directive: {
      name: "adjust",
      usage:
        "/adjust lora <name> <weight> [-j <N>]\n/adjust prompt <text> <weight> [--region <region>]...",
      autocomplete: true,
    },
  },
  {
    id: "hook2",
    name: "添加元素",
    directive: {
      name: "add",
      usage:
        "/add [--neg] [--region <region>]... [--node <node-id>]... <prompt>...\n给当前目录图片添加提示词",
      autocomplete: true,
    },
  },
  {
    id: "hook3",
    name: "无补全钩子",
    directive: {
      name: "noauto",
      usage: "/noauto",
      autocomplete: false,
    },
  },
];

const directiveNames = hookList
  .filter((h) => h.directive?.name != null)
  .map((h) => (h.directive as NonNullable<typeof h.directive>).name);

function buildRules(): DirectiveRule[] {
  const rules: DirectiveRule[] = [];
  for (const h of hookList) {
    if (h.directive?.usage) {
      rules.push(...parseUsage(h.directive.usage));
    }
  }
  return rules;
}

function makeArgsState(overrides: Partial<AutocompleteState> = {}): AutocompleteState {
  return {
    show: true,
    type: "args",
    query: "",
    triggerIndex: 5,
    selectionStart: 5,
    directiveName: "add",
    argsText: "",
    ...overrides,
  };
}

function makeNameState(overrides: Partial<AutocompleteState> = {}): AutocompleteState {
  return {
    show: true,
    type: "name",
    query: "",
    triggerIndex: 1,
    selectionStart: 3,
    ...overrides,
  };
}

const rawApiSuggestion = {
  type: "positional" as const,
  text: "positive",
  displayText: "positive",
  description: null,
  style: null,
};

// #endregion

// #region getLinePrefix

describe("getLinePrefix", () => {
  test("returns prefix from line start to cursor", () => {
    expect(getLinePrefix("hello world", 5)).toBe("hello");
  });

  test("handles mid-line cursor", () => {
    expect(getLinePrefix("line1\nline2 text", 11)).toBe("line2");
  });

  test("returns empty for cursor at line start", () => {
    expect(getLinePrefix("line1\n", 6)).toBe("");
  });

  test("returns empty for empty text", () => {
    expect(getLinePrefix("", 0)).toBe("");
  });
});

// #endregion

// #region computeAutocompleteState

describe("computeAutocompleteState", () => {
  test("returns null when menu not visible", () => {
    expect(computeAutocompleteState("/a", 2, false, false, directiveNames)).toBeNull();
  });

  test("returns null when dismissed", () => {
    expect(computeAutocompleteState("/a", 2, true, true, directiveNames)).toBeNull();
  });

  test("derives name autocomplete state for /a", () => {
    const state = computeAutocompleteState("/a", 2, true, false, directiveNames);
    expect(state).not.toBeNull();
    expect(state?.show).toBe(true);
    expect(state?.type).toBe("name");
    expect(state?.query).toBe("a");
    expect(state?.triggerIndex).toBe(1);
  });

  test("derives name autocomplete state for empty query after /", () => {
    const state = computeAutocompleteState("/", 1, true, false, directiveNames);
    expect(state).not.toBeNull();
    expect(state?.type).toBe("name");
    expect(state?.query).toBe("");
  });

  test("derives args autocomplete state for known directive", () => {
    const state = computeAutocompleteState("/add ", 5, true, false, directiveNames);
    expect(state).not.toBeNull();
    expect(state?.type).toBe("args");
    expect(state?.directiveName).toBe("add");
    expect(state?.query).toBe(""); // 光标在空格后，无输入
  });

  test("derives args autocomplete with partial query", () => {
    const state = computeAutocompleteState("/add --reg", 10, true, false, directiveNames);
    expect(state).not.toBeNull();
    expect(state?.type).toBe("args");
    expect(state?.query).toBe("--reg");
  });

  test("returns null for unknown directive", () => {
    const state = computeAutocompleteState("/unknown ", 9, true, false, directiveNames);
    expect(state).toBeNull();
  });

  test("returns null for plain text", () => {
    const state = computeAutocompleteState("hello world", 5, true, false, directiveNames);
    expect(state).toBeNull();
  });

  test("matches directive with leading whitespace", () => {
    const state = computeAutocompleteState("  /add ", 7, true, false, directiveNames);
    expect(state).not.toBeNull();
    expect(state?.type).toBe("args");
    expect(state?.directiveName).toBe("add");
  });
});

// #endregion

// #region computeApiSuggestions

describe("computeApiSuggestions", () => {
  test("returns empty for null state", () => {
    expect(computeApiSuggestions(null, undefined, [rawApiSuggestion], null, "")).toEqual([]);
  });

  test("returns empty for name mode state", () => {
    const state = makeNameState();
    expect(computeApiSuggestions(state, undefined, [rawApiSuggestion], null, "")).toEqual([]);
  });

  test("returns empty when apiQueryVars mismatch hookId", () => {
    const state = makeArgsState();
    const vars = { input: { hookId: "other", linePrefix: "/add ", query: "" } };
    expect(computeApiSuggestions(state, vars, [rawApiSuggestion], "hook2", "/add ")).toEqual([]);
  });

  test("returns empty when apiQueryVars mismatch linePrefix", () => {
    const state = makeArgsState();
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "" } };
    expect(computeApiSuggestions(state, vars, [rawApiSuggestion], "hook2", "/other ")).toEqual([]);
  });

  test("returns all when query is empty", () => {
    const state = makeArgsState({ query: "" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "" } };
    const result = computeApiSuggestions(state, vars, [rawApiSuggestion], "hook2", "/add ");
    expect(result).toHaveLength(1);
  });

  test("returns all when query matches API query exactly", () => {
    const state = makeArgsState({ query: "pos" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "pos" } };
    const result = computeApiSuggestions(state, vars, [rawApiSuggestion], "hook2", "/add ");
    expect(result).toHaveLength(1);
  });

  test("filters locally when query differs from API query", () => {
    const state = makeArgsState({ query: "pos" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "p" } };
    const result = computeApiSuggestions(
      state,
      vars,
      [
        rawApiSuggestion,
        {
          type: "positional",
          text: "negative",
          displayText: "negative",
          description: null,
          style: null,
        },
      ],
      "hook2",
      "/add ",
    );
    expect(result).toHaveLength(1);
    expect(result[0].text).toBe("positive");
  });

  test("excludes exact match when filtering", () => {
    const state = makeArgsState({ query: "positive" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "pos" } };
    const result = computeApiSuggestions(state, vars, [rawApiSuggestion], "hook2", "/add ");
    expect(result).toHaveLength(0);
  });
});

// #endregion

// #region computeSuggestions

describe("computeSuggestions", () => {
  const rules = buildRules();

  test("returns empty for null state", () => {
    expect(computeSuggestions(null, hookList, rules, [], true)).toEqual([]);
  });

  test("returns empty when state.show is false", () => {
    const state = makeNameState({ show: false });
    expect(computeSuggestions(state, hookList, rules, [], true)).toEqual([]);
  });

  test("name mode: returns all matching directives", () => {
    const state = makeNameState({ query: "a" });
    const result = computeSuggestions(state, hookList, rules, [], true);
    expect(result.length).toBe(3); // adjust, add, noauto
    expect(result.every((s) => s.type === "subcommand")).toBe(true);
  });

  test("name mode: filters by query", () => {
    const state = makeNameState({ query: "adj" });
    const result = computeSuggestions(state, hookList, rules, [], true);
    expect(result.length).toBe(1);
    expect(result[0].text).toBe("adjust");
  });

  test("name mode: returns all when query is empty", () => {
    const state = makeNameState({ query: "" });
    const result = computeSuggestions(state, hookList, rules, [], true);
    expect(result.length).toBe(3); // adjust, add, noauto
  });

  test("args mode: returns static suggestions for known directive", () => {
    const state = makeArgsState({ directiveName: "add", query: "" });
    const result = computeSuggestions(state, hookList, rules, [], true);
    // /add has [--neg] [--region <region>]... [--node <node-id>]... <prompt>...
    expect(result.some((s) => s.text.startsWith("--region"))).toBe(true);
    expect(result.some((s) => s.text === "--neg")).toBe(true);
  });

  test("args mode: returns empty for unknown directive", () => {
    const state = makeArgsState({ directiveName: "unknown" });
    const result = computeSuggestions(state, hookList, rules, [], true);
    expect(result).toEqual([]);
  });

  test("args mode: merges API suggestions with non-positional static results", () => {
    const state = makeArgsState({ directiveName: "add", query: "n" });
    const apiSugs: Suggestion[] = [
      { type: "positional", text: "positive", displayText: "positive" },
    ];
    const result = computeSuggestions(state, hookList, rules, apiSugs, true);
    // API suggestions come first, then non-positional options
    expect(result[0].text).toBe("positive");
    expect(result.some((s) => s.type === "option")).toBe(true);
  });

  test("args mode: skips API suggestions when autocomplete disabled", () => {
    const state = makeArgsState({ directiveName: "noauto", query: "" });
    const apiSugs: Suggestion[] = [{ type: "positional", text: "test", displayText: "test" }];
    const result = computeSuggestions(state, hookList, rules, apiSugs, false);
    // noauto has no autocomplete, so API suggestions should not appear
    const hasApiSug = result.some((s) => s.text === "test");
    expect(hasApiSug).toBe(false);
  });

  test("args mode: skips API suggestions when query starts with -", () => {
    const state = makeArgsState({ directiveName: "add", query: "-" });
    const apiSugs: Suggestion[] = [
      { type: "positional", text: "positive", displayText: "positive" },
    ];
    const result = computeSuggestions(state, hookList, rules, apiSugs, true);
    // query starts with '-', so it should show option suggestions
    expect(result.some((s) => s.type === "option")).toBe(true);
  });
});

// #endregion

// #region needsDynamicLoading

describe("needsDynamicLoading", () => {
  test("returns false for null state", () => {
    expect(needsDynamicLoading(null, true, undefined, null, "", [])).toBe(false);
  });

  test("returns false when not enabled", () => {
    const state = makeArgsState();
    expect(needsDynamicLoading(state, false, undefined, null, "", [])).toBe(false);
  });

  test("returns false when query starts with -", () => {
    const state = makeArgsState({ query: "-" });
    expect(
      needsDynamicLoading(
        state,
        true,
        { input: { hookId: "hook2", linePrefix: "/add ", query: "-" } },
        "hook2",
        "/add ",
        [],
      ),
    ).toBe(false);
  });

  test("returns true when apiQueryVars is undefined", () => {
    const state = makeArgsState({ query: "test" });
    expect(needsDynamicLoading(state, true, undefined, "hook2", "/add ", [])).toBe(true);
  });

  test("returns true when hookId mismatches", () => {
    const state = makeArgsState({ query: "test" });
    const vars = { input: { hookId: "other", linePrefix: "/add ", query: "t" } };
    expect(needsDynamicLoading(state, true, vars, "hook2", "/add ", [])).toBe(true);
  });

  test("returns true when raw suggestions is empty", () => {
    const state = makeArgsState({ query: "test" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "t" } };
    expect(needsDynamicLoading(state, true, vars, "hook2", "/add ", [])).toBe(true);
  });

  test("returns false when query matches API query exactly", () => {
    const state = makeArgsState({ query: "pos" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "pos" } };
    const raw = [{ text: "positive", displayText: "positive" }];
    expect(needsDynamicLoading(state, true, vars, "hook2", "/add ", raw)).toBe(false);
  });

  test("returns true when filtered count is below 50%", () => {
    const state = makeArgsState({ query: "x" });
    const vars = { input: { hookId: "hook2", linePrefix: "/add ", query: "p" } };
    const raw = [
      { text: "positive", displayText: "positive" },
      { text: "negative", displayText: "negative" },
      { text: "neutral", displayText: "neutral" },
      { text: "custom", displayText: "custom" },
    ];
    // "x" matches none, so filteredLength = 0, 0 < 4 * 0.5 = true
    expect(needsDynamicLoading(state, true, vars, "hook2", "/add ", raw)).toBe(true);
  });
});

// #endregion

// #region computeSuggestionInsertion

describe("computeSuggestionInsertion", () => {
  test("name mode: appends space after suggestion text", () => {
    const state = makeNameState({ triggerIndex: 1, type: "name" });
    const sug: Suggestion = { type: "subcommand", text: "adjust", displayText: "/adjust" };
    const params = computeSuggestionInsertion(sug, state, 3) as InsertParams;
    expect(params.textToInsert).toBe("adjust ");
    expect(params.start).toBe(1);
    expect(params.end).toBe(3);
    expect(params.hasPlaceholder).toBe(false);
  });

  test("option mode without placeholder: appends space", () => {
    const state = makeArgsState({ triggerIndex: 5, type: "args" });
    const sug: Suggestion = { type: "option", text: "--neg", displayText: "--neg" };
    const params = computeSuggestionInsertion(sug, state, 5) as InsertParams;
    expect(params.textToInsert).toBe("--neg ");
  });

  test("option mode with placeholder: keeps placeholder and selects it", () => {
    const state = makeArgsState({ triggerIndex: 5, type: "args" });
    const sug: Suggestion = {
      type: "option",
      text: "--region <region>",
      displayText: "--region <region>",
      placeholder: "<region>",
    };
    const params = computeSuggestionInsertion(sug, state, 5) as InsertParams;
    expect(params.textToInsert).toBe("--region <region>");
    expect(params.hasPlaceholder).toBe(true);
    expect(params.selectStart).toBe(5 + "--region ".length);
    expect(params.selectEnd).toBe(5 + "--region <region>".length);
  });

  test("positional type: does not append space", () => {
    const state = makeArgsState({ triggerIndex: 5, type: "args" });
    const sug: Suggestion = {
      type: "positional",
      text: "<prompt>",
      displayText: "<prompt>",
      placeholder: "<prompt>",
    };
    const params = computeSuggestionInsertion(sug, state, 5) as InsertParams;
    expect(params.textToInsert).toBe("<prompt>");
  });

  test("positional type without placeholder (e.g. danbooru tag): appends space", () => {
    const state = makeArgsState({ triggerIndex: 5, type: "args" });
    const sug: Suggestion = { type: "positional", text: "1girl", displayText: "1girl" };
    const params = computeSuggestionInsertion(sug, state, 5) as InsertParams;
    expect(params.textToInsert).toBe("1girl ");
  });

  test("node-id type without placeholder: appends space", () => {
    const state = makeArgsState({ triggerIndex: 5, type: "args" });
    const sug: Suggestion = { type: "node-id", text: "node123", displayText: "node123" };
    const params = computeSuggestionInsertion(sug, state, 5) as InsertParams;
    expect(params.textToInsert).toBe("node123 ");
  });
});

// #endregion

// #region computeDirectiveInsertion

describe("computeDirectiveInsertion", () => {
  test("adds newline when cursor is not at line start", () => {
    const params = computeDirectiveInsertion("add", "some content", 12, 12);
    expect(params.textToInsert).toBe("\n/add ");
    expect(params.start).toBe(12);
    expect(params.end).toBe(12);
    expect(params.selectStart).toBe(12 + "\n/add ".length);
  });

  test("no newline when cursor is at empty line start", () => {
    const params = computeDirectiveInsertion("add", "line1\n", 6, 6);
    // before is "line1\n", regex /(?:^|\n)[ \t]*$/ matches "\n"
    expect(params.textToInsert).toBe("/add ");
  });

  test("no newline when cursor is at start of text", () => {
    const params = computeDirectiveInsertion("add", "", 0, 0);
    expect(params.textToInsert).toBe("/add ");
  });

  test("no newline when cursor is at start of whitespace-only line", () => {
    const params = computeDirectiveInsertion("add", "line1\n  ", 8, 8);
    expect(params.textToInsert).toBe("/add ");
  });
});

// #endregion
