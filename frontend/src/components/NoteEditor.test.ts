import { describe, test, expect, vi, beforeEach, afterEach } from "vitest";
import { type VueWrapper, mount } from "@vue/test-utils";
import { nextTick } from "vue";
import NoteEditor from "./NoteEditor.vue";

const mockQuery = vi.hoisted(() => vi.fn());
const refreshOn = vi.hoisted(() => vi.fn());
const timeNow = vi.hoisted(() => ({ value: 1000000 }));

const hookList = [
  { id: "hook1", name: "调整图片", canDispatchByNote: false, directive: { name: "adjust", usage: "/adjust lora <name> <weight> [-j <N>]\n/adjust prompt <text> <weight> [--region <region>]...", autocomplete: true } },
  { id: "hook2", name: "添加元素", canDispatchByNote: false, directive: { name: "add", usage: "/add [--neg] [--region <region>]... [--node <node-id>]... <prompt>...\n给当前目录图片添加提示词", autocomplete: true } },
  { id: "hook3", name: "移除元素", canDispatchByNote: false, directive: { name: "remove", usage: "/remove [--neg] [--region <region>]... [--node <node-id>]... <prompt>...\n移除指定提示词", autocomplete: true } },
  { id: "hook4", name: "无指令钩子", canDispatchByNote: false, directive: null },
];

vi.mock("@/graphql/utils/useQuery", () => ({ default: vi.fn(() => ({ data: { value: { hooks: hookList } }, loading: { value: false } })) }));
vi.mock("@/graphql/utils/query", () => ({ default: mockQuery }));
vi.mock("@/graphql/utils/mutate", () => ({ default: vi.fn() }));
vi.mock("@/graphql/generated", () => ({ HooksDocument: "HooksDocument", DispatchNoteHookDocument: "DispatchNoteHookDocument", HookAutocompleteDocument: "HookAutocompleteDocument" }));
vi.mock("@floating-ui/vue", () => ({ useFloating: vi.fn(() => ({ floatingStyles: { value: {} }, update: vi.fn(), placement: { value: "top-start" } })), offset: vi.fn(() => vi.fn()), flip: vi.fn(() => vi.fn()), shift: vi.fn(() => vi.fn()), autoUpdate: vi.fn(() => vi.fn()) }));
vi.mock("@/composables/useTextAreaAutoHeight", () => ({ default: vi.fn() }));
vi.mock("@/composables/useNotification", () => ({ default: vi.fn(() => ({ showSuccess: vi.fn(), showError: vi.fn(), showInfo: vi.fn(), remove: vi.fn() })) }));
vi.mock("@/composables/useClickOutside", () => ({ default: vi.fn() }));
vi.mock("@/composables/useCurrentTime", () => ({ default: vi.fn(() => ({ currentTime: { get value() { return { ms: timeNow.value, sub: (other: unknown) => timeNow.value - (other as { ms: number }).ms, add: (d: unknown) => ({ ms: timeNow.value + (d as number) }) }; } }, refreshOn })) }));
vi.mock("@/utils/Time", () => ({ default: { now: () => ({ ms: timeNow.value, sub: (other: unknown) => timeNow.value - (other as { ms: number }).ms, add: (d: unknown) => ({ ms: timeNow.value + (d as number) }) }), from: (input: unknown) => input } }));
vi.mock("@mdi/js", () => ({ mdiConsole: "", mdiLightningBolt: "", mdiChevronDown: "", mdiLoading: "" }));

async function typeInTextarea(wrapper: VueWrapper, text: string) {
  const el = wrapper.find("textarea");
  await el.trigger("focus");
  await el.setValue(text);
  el.element.selectionStart = text.length;
  el.element.selectionEnd = text.length;
  el.element.dispatchEvent(new Event("keyup", { bubbles: true }));
  await nextTick();
}

function getSuggestionMenu() {
  return document.body.querySelector("[class*='z-50']");
}

describe("NoteEditor", () => {
  let wrapper: VueWrapper;

  beforeEach(() => {
    mockQuery.mockReset();
    mockQuery.mockResolvedValue({ data: { hookAutocomplete: [] } });
    refreshOn.mockClear();
    timeNow.value = 1000000;
  });

  afterEach(() => {
    wrapper?.unmount();
  });

  function createWrapper(initialValue = "") {
    wrapper = mount(NoteEditor, {
      props: {
        modelValue: initialValue,
        "onUpdate:modelValue": (val: string) => wrapper.setProps({ modelValue: val }),
      },
    });
    return wrapper;
  }

  function findSuggestionTexts() {
    const menu = getSuggestionMenu();
    if (!menu) return [];
    return Array.from(menu.querySelectorAll("button")).map((btn) => btn.textContent ?? "");
  }

  test("renders textarea", () => {
    createWrapper();
    expect(wrapper.find("textarea").exists()).toBe(true);
  });

  test("renders with initial value", () => {
    createWrapper("hello world");
    expect(wrapper.find("textarea").element.value).toBe("hello world");
  });

  test("name mode for /add", async () => {
    await typeInTextarea(createWrapper(), "/add");
    const texts = findSuggestionTexts();
    expect(texts.length).toBeGreaterThan(0);
  });

  test("args mode for /add with space", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    const texts = findSuggestionTexts();
    expect(texts.some((t) => t.includes("--region"))).toBe(true);
  });

  test("no menu for plain text", async () => {
    await typeInTextarea(createWrapper(), "hello world");
    expect(getSuggestionMenu()).toBeNull();
  });

  test("no menu for unknown directive", async () => {
    await typeInTextarea(createWrapper(), "/unknown ");
    expect(getSuggestionMenu()).toBeNull();
  });

  test("dismisses on escape", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    expect(getSuggestionMenu()).not.toBeNull();
    wrapper.find("textarea").element.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await nextTick();
    expect(getSuggestionMenu()).toBeNull();
  });

  test("menu visible after blur (within 200ms)", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    wrapper.find("textarea").element.dispatchEvent(new Event("blur", { bubbles: true }));
    await nextTick();
    expect(getSuggestionMenu()).not.toBeNull();
  });

  test("menu hidden after 200ms from blur", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    wrapper.find("textarea").element.dispatchEvent(new Event("blur", { bubbles: true }));
    timeNow.value = 1000200;
    await nextTick();
    expect(getSuggestionMenu()).toBeNull();
  });

  test("triggers api on args input", async () => {
    createWrapper();
    const el = wrapper.find("textarea");
    await el.trigger("focus");
    await el.setValue("/add ");
    el.element.selectionStart = 5;
    el.element.selectionEnd = 5;
    // setValue 后 cursorStart 为 0，需重新触发 input 让 handleInput 识别 args 模式
    el.element.dispatchEvent(new Event("input", { bubbles: true }));
    el.element.dispatchEvent(new Event("keyup", { bubbles: true }));

    await vi.waitFor(() => {
      expect(mockQuery).toHaveBeenCalled();
    }, { timeout: 2000, interval: 50 });
  });

  test("replaces placeholder with API suggestion selection", async () => {
    mockQuery.mockResolvedValue({
      data: {
        hookAutocomplete: [
          { text: "positive", displayText: "positive", type: null },
          { text: "negative", displayText: "negative", type: null },
        ],
      },
    });

    createWrapper();
    const el = wrapper.find("textarea");
    await el.trigger("focus");
    await el.setValue("/add --region <region>");
    el.element.selectionStart = 14;
    el.element.selectionEnd = 22;
    // setValue 后 cursorStart 为 0，重发 input 让 handleInput 识别 args 模式并触发动态补全
    el.element.dispatchEvent(new Event("input", { bubbles: true }));
    el.element.dispatchEvent(new Event("keyup", { bubbles: true }));

    // 先确认 API 被调用
    await vi.waitFor(() => {
      expect(mockQuery).toHaveBeenCalled();
    }, { timeout: 2000, interval: 50 });

    // 等待菜单更新与 API 建议
    await vi.waitFor(() => {
      const texts = findSuggestionTexts();
      expect(texts.some((t) => t.includes("positive"))).toBe(true);
    }, { timeout: 2000, interval: 50 });

    // 点击 positive API 建议
    const menu = getSuggestionMenu();
    if (!menu) { expect(menu).not.toBeNull(); return; }
    const allButtons = Array.from(menu.querySelectorAll("button"));
    const positiveBtn = allButtons.find((b) => b.textContent?.includes("positive"));
    expect(positiveBtn).toBeDefined();
    positiveBtn?.click();
    await nextTick();

    // 验证选区 `<region>` 被替换为 `positive`，而不是 `positive<region>`
    expect(wrapper.find("textarea").element.value).toBe("/add --region positive");
  });
});
