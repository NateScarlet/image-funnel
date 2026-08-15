import { describe, test, expect, vi, beforeEach } from "vitest";
import { ref, type Ref } from "vue";
import { useNoteAutocomplete } from "./useNoteAutocomplete";

const mockQuery = vi.hoisted(() => vi.fn());
const refreshOn = vi.hoisted(() => vi.fn());
const timeNow = vi.hoisted(() => ({ value: 1000000 }));

const hookList = [
  {
    id: "hook1",
    name: "调整图片",
    canDispatchByNote: false,
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
    canDispatchByNote: false,
    directive: {
      name: "add",
      usage:
        "/add [--neg] [--region <region>]... [--node <node-id>]... <prompt>...\n给当前目录图片添加提示词",
      autocomplete: true,
    },
  },
];

const mockHooksData = { value: { hooks: hookList } };
const mockAutocompleteData = ref<unknown>(undefined);

const mockUseQuery = vi.hoisted(() =>
  vi.fn((document: unknown, _options: unknown) => {
    if (document === "HooksDocument") {
      return { data: mockHooksData, loading: { value: false } };
    }
    return {
      data: mockAutocompleteData,
      loading: { value: false },
    };
  }),
);

vi.mock("@/graphql/utils/useQuery", () => ({
  default: mockUseQuery,
}));
vi.mock("@/graphql/generated", () => ({
  HooksDocument: "HooksDocument",
  HookAutocompleteDocument: "HookAutocompleteDocument",
}));
vi.mock("@/composables/useCurrentTime", () => ({
  default: vi.fn(() => ({
    currentTime: {
      get value() {
        return {
          ms: timeNow.value,
          sub: (other: unknown) => timeNow.value - (other as { ms: number }).ms,
          add: (d: unknown) => ({ ms: timeNow.value + (d as number) }),
        };
      },
    },
    refreshOn,
  })),
}));
vi.mock("@/utils/Time", () => ({
  default: {
    now: () => ({
      ms: timeNow.value,
      sub: (other: unknown) => timeNow.value - (other as { ms: number }).ms,
      add: (d: unknown) => ({ ms: timeNow.value + (d as number) }),
    }),
    from: (input: unknown) => input,
  },
}));

describe("useNoteAutocomplete composable", () => {
  let model: Ref<string>;
  let cursorStart: Ref<number>;
  let cursorEnd: Ref<number>;
  let isFocused: Ref<boolean>;

  beforeEach(() => {
    mockQuery.mockReset();
    mockQuery.mockResolvedValue({ data: { hookAutocomplete: [] } });
    refreshOn.mockClear();
    timeNow.value = 1000000;

    model = ref("");
    cursorStart = ref(0);
    cursorEnd = ref(0);
    isFocused = ref(true);
  });

  function createAutocomplete() {
    return useNoteAutocomplete({
      model,
      cursorStart,
      cursorEnd,
      isFocused,
    });
  }

  test("derives name autocomplete state for /a", () => {
    model.value = "/a";
    cursorStart.value = 2;
    cursorEnd.value = 2;

    const { state, suggestions } = createAutocomplete();

    expect(state.value).not.toBeNull();
    expect(state.value?.show).toBe(true);
    expect(state.value?.type).toBe("name");
    expect(state.value?.query).toBe("a");
    expect(suggestions.value.length).toBe(2); // adjust, add
  });

  test("derives args autocomplete state for /add with space", () => {
    model.value = "/add ";
    cursorStart.value = 5;
    cursorEnd.value = 5;

    const { state, suggestions } = createAutocomplete();

    expect(state.value).not.toBeNull();
    expect(state.value?.show).toBe(true);
    expect(state.value?.type).toBe("args");
    expect(suggestions.value.some((s) => s.text.startsWith("--region"))).toBe(true);
  });

  test("arrow keys navigate index in suggestions", () => {
    model.value = "/a";
    cursorStart.value = 2;
    cursorEnd.value = 2;

    const { activeIndex, handleKeyDown, handleKeyUp } = createAutocomplete();

    expect(activeIndex.value).toBe(-1); // default not selected

    const downEvent1 = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    handleKeyDown(downEvent1);
    expect(activeIndex.value).toBe(0);
    expect(downEvent1.defaultPrevented).toBe(true);

    const downEvent2 = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    handleKeyDown(downEvent2);
    expect(activeIndex.value).toBe(1);
    expect(downEvent2.defaultPrevented).toBe(true);

    const downEvent3 = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    handleKeyDown(downEvent3);
    expect(activeIndex.value).toBe(0); // circular
    expect(downEvent3.defaultPrevented).toBe(true);

    const upEvent = new KeyboardEvent("keydown", { key: "ArrowUp", cancelable: true });
    handleKeyUp(upEvent);
    expect(activeIndex.value).toBe(1); // circular up
    expect(upEvent.defaultPrevented).toBe(true);
  });

  test("up and down keys do not prevent default when autocomplete is not active", () => {
    model.value = "hello world";
    cursorStart.value = 5;
    cursorEnd.value = 5;

    const { handleKeyDown, handleKeyUp } = createAutocomplete();

    const downEvent = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    handleKeyDown(downEvent);
    expect(downEvent.defaultPrevented).toBe(false);

    const upEvent = new KeyboardEvent("keydown", { key: "ArrowUp", cancelable: true });
    handleKeyUp(upEvent);
    expect(upEvent.defaultPrevented).toBe(false);
  });

  test("Ctrl+Space selects the first item immediately", () => {
    model.value = "/a";
    cursorStart.value = 2;
    cursorEnd.value = 2;

    const { activeIndex, handleKeySpace } = createAutocomplete();
    const event = new KeyboardEvent("keydown", { key: " ", ctrlKey: true });

    handleKeySpace(event);
    expect(activeIndex.value).toBe(0);
  });

  test("handleSelectSuggestion returns correct InsertParams", () => {
    model.value = "/ad";
    cursorStart.value = 3;
    cursorEnd.value = 3;

    const { suggestions, handleSelectSuggestion } = createAutocomplete();
    const sug = suggestions.value[0]; // /adjust or /add

    const params = handleSelectSuggestion(sug, 3);

    expect(params).not.toBeNull();
    if (!params) return;
    expect(params.textToInsert).toBe(sug.text + " "); // name mode adds space
    expect(params.start).toBe(1); // trigger index (after '/')
    expect(params.end).toBe(3); // selectionEnd
    expect(params.selectStart).toBe(1 + sug.text.length + 1); // selectStart
  });

  test("activeIndex preserves selection when suggestion text is unchanged after model change", () => {
    // 初始状态：指令名补全，建议为 [adjust, add]
    model.value = "/a";
    cursorStart.value = 2;
    cursorEnd.value = 2;

    const { activeIndex, suggestions, handleKeyDown } = createAutocomplete();

    handleKeyDown(new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true })); // 选中 adjust
    expect(activeIndex.value).toBe(0);
    expect(suggestions.value[0].text).toBe("adjust");

    // 修改模型为 "/ad"（仍匹配 adjust），建议列表不变
    model.value = "/ad";
    cursorStart.value = 3;
    cursorEnd.value = 3;

    // 同一建议文本仍在同一位置，activeIndex 应保持
    expect(suggestions.value[0].text).toBe("adjust");
    expect(activeIndex.value).toBe(0);
  });

  test("handleKeyEsc returns true and dismisses when autocomplete is showing", () => {
    model.value = "/a";
    cursorStart.value = 2;
    cursorEnd.value = 2;

    const { state, handleKeyEsc } = createAutocomplete();
    expect(state.value?.show).toBe(true);

    const handled = handleKeyEsc();
    expect(handled).toBe(true);
    expect(state.value).toBeNull();
  });

  test("handleKeyEsc returns false when autocomplete is not showing", () => {
    model.value = "plain text";
    cursorStart.value = 5;
    cursorEnd.value = 5;

    const { handleKeyEsc } = createAutocomplete();
    expect(handleKeyEsc()).toBe(false);
  });

  test("isSearching is false when query completes with empty results", () => {
    vi.useFakeTimers();
    model.value = "/add p";
    cursorStart.value = 6;
    cursorEnd.value = 6;

    const { isSearching, flushDebounced } = createAutocomplete();

    // Trigger debounce flush
    flushDebounced();

    // Mock autocomplete data returning empty list
    mockAutocompleteData.value = { hookAutocomplete: [] };

    // After debounce is flushed and variables have updated, isSearching should be false
    expect(isSearching.value).toBe(false);
  });
});
