import { describe, test, expect, vi, beforeEach, afterEach } from "vitest";
import { type VueWrapper, mount } from "@vue/test-utils";
import { nextTick, ref, watch, toValue, customRef } from "vue";
import NoteEditor from "./NoteEditor.vue";

const mockQuery = vi.hoisted(() => vi.fn());
const refreshOn = vi.hoisted(() => vi.fn());
const timeNow = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  return {
    currentMs: 1000000,
    get value() {
      return this.currentMs;
    },
    set value(v: number) {
      this.currentMs = v;
      listeners.forEach((l) => l());
    },
    subscribe(l: () => void) {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
});

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
  {
    id: "hook3",
    name: "移除元素",
    canDispatchByNote: false,
    directive: {
      name: "remove",
      usage:
        "/remove [--neg] [--region <region>]... [--node <node-id>]... <prompt>...\n移除指定提示词",
      autocomplete: true,
    },
  },
  { id: "hook4", name: "无指令钩子", canDispatchByNote: false, directive: null },
];

const mockHooksData = { value: { hooks: hookList } };
const mockAutocompleteData = ref<unknown>(undefined);

const mockUseQuery = vi.hoisted(() =>
  vi.fn((document: unknown, options: unknown) => {
    if (document === "HooksDocument") {
      return { data: mockHooksData, loading: { value: false } };
    }

    const opts = options as { variables?: unknown; loadingCount?: { value: number } } | undefined;

    if (opts?.variables) {
      watch(
        () => toValue(opts.variables),
        async (vars) => {
          if (!vars) {
            mockAutocompleteData.value = undefined;
            return;
          }

          if (opts.loadingCount) {
            opts.loadingCount.value++;
          }

          try {
            const res = await mockQuery(document, { variables: vars });
            mockAutocompleteData.value = res.data;
          } catch {
            // 忽略
          } finally {
            if (opts.loadingCount) {
              opts.loadingCount.value--;
            }
          }
        },
        { immediate: true, deep: true },
      );
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
vi.mock("@/graphql/utils/query", () => ({ default: mockQuery }));
vi.mock("@/graphql/utils/mutate", () => ({ default: vi.fn() }));
vi.mock("@/graphql/generated", () => ({
  HooksDocument: "HooksDocument",
  DispatchNoteHookDocument: "DispatchNoteHookDocument",
  HookAutocompleteDocument: "HookAutocompleteDocument",
}));
vi.mock("@floating-ui/vue", () => ({
  useFloating: vi.fn(() => ({
    floatingStyles: { value: {} },
    update: vi.fn(),
    placement: { value: "top-start" },
  })),
  offset: vi.fn(() => vi.fn()),
  flip: vi.fn(() => vi.fn()),
  shift: vi.fn(() => vi.fn()),
  autoUpdate: vi.fn(() => vi.fn()),
}));
vi.mock("@/composables/useTextAreaAutoHeight", () => ({ default: vi.fn() }));
vi.mock("@/composables/useNotification", () => ({
  default: vi.fn(() => ({
    showSuccess: vi.fn(),
    showError: vi.fn(),
    showInfo: vi.fn(),
    remove: vi.fn(),
  })),
}));
vi.mock("@/composables/useClickOutside", () => ({ default: vi.fn() }));
vi.mock("@/composables/useCurrentTime", () => {
  return {
    default: vi.fn(() => {
      const currentTime = customRef((track: () => void, trigger: () => void) => {
        timeNow.subscribe(() => {
          trigger();
        });
        return {
          get() {
            track();
            return {
              ms: timeNow.value,
              sub: (other: unknown) => timeNow.value - (other as { ms: number }).ms,
              add: (d: unknown) => ({ ms: timeNow.value + (d as number) }),
            };
          },
          set() {},
        };
      });
      return {
        currentTime,
        refreshOn,
      };
    }),
  };
});
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
vi.mock("@mdi/js", () => ({
  mdiConsole: "",
  mdiLightningBolt: "",
  mdiChevronDown: "",
  mdiLoading: "",
}));

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
    document.body.querySelectorAll("[class*='z-50']").forEach((el) => el.remove());
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
    wrapper
      .find("textarea")
      .element.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
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

    await vi.waitFor(
      () => {
        expect(mockQuery).toHaveBeenCalled();
      },
      { timeout: 2000, interval: 50 },
    );
  });

  test("shows loading state during autocomplete debounce period", async () => {
    createWrapper();
    const el = wrapper.find("textarea");
    await el.trigger("focus");
    await el.setValue("/add ");
    el.element.selectionStart = 5;
    el.element.selectionEnd = 5;

    // 触发 input 事件以启动动态补全防抖
    el.element.dispatchEvent(new Event("input", { bubbles: true }));
    await nextTick();

    // 在防抖期间（API 尚未被调用前），确认菜单浮层是可见的，并且包含“加载中...”
    const menu = getSuggestionMenu();
    expect(menu).not.toBeNull();
    expect(menu?.textContent).toContain("加载中…");
    expect(mockQuery).not.toHaveBeenCalled();

    // 等待防抖结束并让 API 调用完成
    await vi.waitFor(
      () => {
        expect(mockQuery).toHaveBeenCalled();
      },
      { timeout: 2000, interval: 50 },
    );
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
    await vi.waitFor(
      () => {
        expect(mockQuery).toHaveBeenCalled();
      },
      { timeout: 2000, interval: 50 },
    );

    // 等待菜单更新与 API 建议
    await vi.waitFor(
      () => {
        const texts = findSuggestionTexts();
        expect(texts.some((t) => t.includes("positive"))).toBe(true);
      },
      { timeout: 2000, interval: 50 },
    );

    // 点击 positive API 建议
    const menu = getSuggestionMenu();
    if (!menu) {
      expect(menu).not.toBeNull();
      return;
    }
    const allButtons = Array.from(menu.querySelectorAll("button"));
    const positiveBtn = allButtons.find((b) => b.textContent?.includes("positive"));
    expect(positiveBtn).toBeDefined();
    positiveBtn?.click();
    await nextTick();

    // 验证选区 `<region>` 被替换为 `positive `，且追加了尾随空格，而不是 `positive<region>`
    expect(wrapper.find("textarea").element.value).toBe("/add --region positive ");
  });

  test("uses document.execCommand to insert suggestion and preserve undo history", async () => {
    // 手动在 document 上挂载 execCommand，因为 JSDOM 默认不提供该方法
    const mockExec = vi.fn((commandId: string, _showUI: boolean, value?: unknown) => {
      if (commandId === "insertText" && typeof value === "string") {
        const textarea = wrapper.find("textarea").element;
        const text = textarea.value;
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        textarea.value = text.slice(0, start) + value + text.slice(end);
        textarea.dispatchEvent(new Event("input", { bubbles: true }));
        return true;
      }
      return false;
    });
    (document as unknown as Record<string, unknown>).execCommand = mockExec;

    try {
      createWrapper();
      const el = wrapper.find("textarea");
      await el.trigger("focus");
      await el.setValue("/ad");
      el.element.selectionStart = 3;
      el.element.selectionEnd = 3;
      el.element.dispatchEvent(new Event("keyup", { bubbles: true }));
      await nextTick();

      // 点击弹出的第一个建议 (/adjust)
      const menu = getSuggestionMenu();
      expect(menu).not.toBeNull();
      const firstButton = menu?.querySelector("button");
      expect(firstButton).not.toBeNull();
      firstButton?.click();
      await nextTick();

      // 验证 execCommand 被正确调用以插入文本
      expect(mockExec).toHaveBeenCalledWith("insertText", false, "adjust ");

      // 验证内容正确被修改了
      expect(el.element.value).toBe("/adjust ");
    } finally {
      delete (document as unknown as Record<string, unknown>).execCommand;
    }
  });

  test("uses document.execCommand to insert directive", async () => {
    const mockExec = vi.fn((commandId: string, _showUI: boolean, value?: unknown) => {
      if (commandId === "insertText" && typeof value === "string") {
        const textarea = wrapper.find("textarea").element;
        const text = textarea.value;
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        textarea.value = text.slice(0, start) + value + text.slice(end);
        textarea.dispatchEvent(new Event("input", { bubbles: true }));
        return true;
      }
      return false;
    });
    (document as unknown as Record<string, unknown>).execCommand = mockExec;

    try {
      createWrapper();
      const el = wrapper.find("textarea");
      await el.trigger("focus");

      // 触发常用指令插入按钮
      const directiveButtons = wrapper.findAll("button[title]");
      expect(directiveButtons.length).toBeGreaterThan(0);

      // 点击第一个快捷指令 /adjust
      await directiveButtons[0].trigger("click");
      await nextTick();

      expect(mockExec).toHaveBeenCalledWith("insertText", false, "/adjust ");
      expect(el.element.value).toBe("/adjust ");
    } finally {
      delete (document as unknown as Record<string, unknown>).execCommand;
    }
  });

  test("autocomplete menu defaults to no item selected on typing", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    const menu = getSuggestionMenu();
    expect(menu).not.toBeNull();

    // 默认不应该有任何项被高亮选中
    const activeItem = menu?.querySelector(".bg-secondary-500");
    expect(activeItem).toBeNull();
  });

  test("navigates suggestions using Up and Down keys from unselected state", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    const textarea = wrapper.find("textarea");

    // 初始状态没有选中
    let menu = getSuggestionMenu();
    expect(menu?.querySelector(".bg-secondary-500")).toBeNull();

    // 按 Down 键，应该选中第 0 项
    await textarea.trigger("keydown", { key: "ArrowDown" });
    await nextTick();
    menu = getSuggestionMenu();
    let activeItem = menu?.querySelector(".bg-secondary-500");
    expect(activeItem).not.toBeNull();
    const buttons = Array.from(menu?.querySelectorAll("button") ?? []);
    expect(activeItem).toBe(buttons[0]);

    // 按 Up 键，应该循环到最后一项
    await textarea.trigger("keydown", { key: "ArrowUp" });
    await nextTick();
    activeItem = menu?.querySelector(".bg-secondary-500");
    expect(activeItem).toBe(buttons[buttons.length - 1]);

    // 从未选中状态按 Up 键，应该选中最后一项
    wrapper?.unmount();
    document.body.querySelectorAll("[class*='z-50']").forEach((el) => el.remove());
    await typeInTextarea(createWrapper(), "/add ");
    await nextTick();
    menu = getSuggestionMenu();
    expect(menu?.querySelector(".bg-secondary-500")).toBeNull();

    await wrapper.find("textarea").trigger("keydown", { key: "ArrowUp" });
    await nextTick();
    menu = getSuggestionMenu();
    activeItem = menu?.querySelector(".bg-secondary-500");
    const newButtons = Array.from(menu?.querySelectorAll("button") ?? []);
    expect(activeItem).toBe(newButtons[newButtons.length - 1]);
  });

  test("does not prevent default Enter behavior when no item is selected", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    const textareaEl = wrapper.find("textarea").element;

    const event = new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true });
    textareaEl.dispatchEvent(event);
    await nextTick();

    expect(event.defaultPrevented).toBe(false);
  });

  test("prevents default Enter behavior if the selected suggestion will change the text by appending a space", async () => {
    mockQuery.mockResolvedValue({
      data: {
        hookAutocomplete: [{ text: "abc", displayText: "abc", type: "positional" }],
      },
    });

    createWrapper();
    const el = wrapper.find("textarea");
    await el.trigger("focus");
    await el.setValue("/add abc");
    el.element.selectionStart = 5;
    el.element.selectionEnd = 8;
    el.element.dispatchEvent(new Event("input", { bubbles: true }));
    el.element.dispatchEvent(new Event("keyup", { bubbles: true }));

    await vi.waitFor(
      () => {
        expect(mockQuery).toHaveBeenCalled();
      },
      { timeout: 2000, interval: 50 },
    );
    await nextTick();

    const menu = getSuggestionMenu();
    expect(menu).not.toBeNull();

    await el.trigger("keydown", { key: "ArrowDown" });
    await nextTick();

    expect(menu?.querySelector(".bg-secondary-500")).not.toBeNull();

    const event = new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true });
    el.element.dispatchEvent(event);
    await nextTick();

    expect(event.defaultPrevented).toBe(true);
  });

  test("selects the first item automatically when Ctrl+Space is pressed", async () => {
    await typeInTextarea(createWrapper(), "/add ");
    const el = wrapper.find("textarea");

    let menu = getSuggestionMenu();
    expect(menu).not.toBeNull();
    expect(menu?.querySelector(".bg-secondary-500")).toBeNull();

    const event = new KeyboardEvent("keydown", {
      key: " ",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });
    el.element.dispatchEvent(event);
    await nextTick();

    menu = getSuggestionMenu();
    const activeItem = menu?.querySelector(".bg-secondary-500");
    expect(activeItem).not.toBeNull();
    const buttons = Array.from(menu?.querySelectorAll("button") ?? []);
    expect(activeItem).toBe(buttons[0]);
  });

  test("API suggestions remain rendered during blur grace period (200ms)", async () => {
    mockQuery.mockResolvedValue({
      data: {
        hookAutocomplete: [{ text: "positive", displayText: "positive", type: null }],
      },
    });

    createWrapper();
    const el = wrapper.find("textarea");
    await el.trigger("focus");
    await el.setValue("/add ");
    el.element.selectionStart = 5;
    el.element.selectionEnd = 5;
    el.element.dispatchEvent(new Event("input", { bubbles: true }));
    el.element.dispatchEvent(new Event("keyup", { bubbles: true }));

    await vi.waitFor(
      () => {
        expect(mockQuery).toHaveBeenCalled();
      },
      { timeout: 2000, interval: 50 },
    );

    await vi.waitFor(
      () => {
        expect(findSuggestionTexts().some((t) => t.includes("positive"))).toBe(true);
      },
      { timeout: 2000, interval: 50 },
    );

    // 触发 blur 事件
    el.element.dispatchEvent(new Event("blur", { bubbles: true }));
    await nextTick();

    // 在 200ms 宽限期内，API 建议应该继续留在 DOM 中
    expect(getSuggestionMenu()).not.toBeNull();
    expect(findSuggestionTexts().some((t) => t.includes("positive"))).toBe(true);

    // 时间流逝 200ms
    timeNow.value = 1000200;
    await nextTick();

    // 200ms 之后，菜单被隐藏
    expect(getSuggestionMenu()).toBeNull();
  });

  test("ignores pointerenter events from touch input", async () => {
    await typeInTextarea(createWrapper(), "/a");
    const menu = getSuggestionMenu();
    expect(menu).not.toBeNull();

    const buttons = Array.from(menu!.querySelectorAll("button"));
    expect(buttons.length).toBeGreaterThan(1);

    // 触发 touch 类型的 pointerenter，不应该设为 active
    buttons[1].dispatchEvent(new PointerEvent("pointerenter", { bubbles: true, pointerType: "touch" }));
    await nextTick();
    expect(buttons[1].classList.contains("bg-secondary-500")).toBe(false);

    // 触发 mouse 类型的 pointerenter，应该设为 active
    buttons[1].dispatchEvent(new PointerEvent("pointerenter", { bubbles: true, pointerType: "mouse" }));
    await nextTick();
    expect(buttons[1].classList.contains("bg-secondary-500")).toBe(true);
  });
});
