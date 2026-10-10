import { test, expect, vi, beforeEach, afterEach } from "vitest";
import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import type { ImageFiltersInput } from "@/graphql/generated";
import { useHotkeys, activeHotkeys } from "@/composables/useHotkeys";
import useImageHooks from "./useImageHooks";

// #region 数据源 mock
const mocks = vi.hoisted(() => ({
  mutate: vi.fn(async (_document: unknown, _options: unknown) => undefined),
  setHooks: (_hooks: unknown) => {},
}));

vi.mock("@/graphql/utils/mutate", () => ({ default: mocks.mutate }));

vi.mock("@/graphql/utils/useQuery", async () => {
  const { shallowRef } = await import("vue");
  const data = shallowRef<{ hooks: unknown[] } | undefined>(undefined);
  mocks.setHooks = (hooks: unknown) => {
    data.value = { hooks: hooks as unknown[] };
  };
  return { default: () => ({ data }) };
});

vi.mock("@/composables/useNotification", () => ({
  default: () => ({
    showSuccess: vi.fn(),
    showError: vi.fn(),
    showInfo: vi.fn(() => "info-id"),
    remove: vi.fn(),
  }),
}));

function hook(overrides: { id: string; name: string; hotkeys: string[] }) {
  return {
    __typename: "Hook" as const,
    id: overrides.id,
    name: overrides.name,
    description: "",
    canDispatchByImage: true,
    canDispatchByNote: false,
    hotkeys: overrides.hotkeys,
    directive: null,
  };
}
// #endregion

const wrappers: ReturnType<typeof mount>[] = [];

function pressKey(key: string) {
  window.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }));
}

function mountWithHooks(selectedFilterBy: () => ImageFiltersInput | undefined) {
  const captured: { hookHotkeys?: ReturnType<typeof useImageHooks>["hookHotkeys"] } = {};
  const wrapper = mount(
    defineComponent({
      setup() {
        const hooks = useImageHooks({ selectedFilterBy });
        captured.hookHotkeys = hooks.hookHotkeys;
        useHotkeys(hooks.hookHotkeys, { category: "钩子动作" });
        return () => null;
      },
    }),
  );
  wrappers.push(wrapper);
  if (!captured.hookHotkeys) throw new Error("hookHotkeys 未被捕获");
  return captured.hookHotkeys;
}

beforeEach(() => {
  mocks.mutate.mockClear();
});

afterEach(() => {
  while (wrappers.length > 0) {
    wrappers.pop()?.unmount();
  }
});

test("钩子声明的快捷键原样透传给快捷键系统，描述取钩子名", () => {
  mocks.setHooks([hook({ id: "hk:krita", name: "发送到 Krita", hotkeys: ["F6", "ctrl+k"] })]);
  const hookHotkeys = mountWithHooks(() => ({ id: ["img-1"] }));

  expect(hookHotkeys.value).toHaveLength(1);
  expect(hookHotkeys.value[0].keys).toEqual(["F6", "ctrl+k"]);
  expect(hookHotkeys.value[0].options?.description).toBe("发送到 Krita");
});

test("同一声明内的重复键只登记一次", () => {
  mocks.setHooks([hook({ id: "hk:krita", name: "发送到 Krita", hotkeys: ["F6", "f6", "F6"] })]);
  mountWithHooks(() => ({ id: ["img-1"] }));

  expect(
    activeHotkeys.value
      .filter((item) => item.category === "钩子动作")
      .map((item) => `${item.description}:${item.keys[0].join("+")}`),
  ).toEqual(["发送到 Krita:F6"]);
});

test("在筛选上下文中按下声明的快捷键会按当前筛选派发对应钩子", () => {
  mocks.setHooks([hook({ id: "hk:krita", name: "发送到 Krita", hotkeys: ["F6"] })]);
  mountWithHooks(() => ({ id: ["img-1", "img-2"] }));

  pressKey("F6");
  expect(mocks.mutate).toHaveBeenCalledTimes(1);
  expect(mocks.mutate.mock.calls[0][1]).toEqual({
    variables: { input: { hookId: "hk:krita", filterBy: { id: ["img-1", "img-2"] } } },
  });
});

test("没有筛选上下文时不派发", () => {
  mocks.setHooks([hook({ id: "hk:krita", name: "发送到 Krita", hotkeys: ["F6"] })]);
  mountWithHooks(() => undefined);

  pressKey("F6");
  expect(mocks.mutate).not.toHaveBeenCalled();
});

test("未声明快捷键的钩子不产生任何快捷键登记", () => {
  mocks.setHooks([hook({ id: "hk:krita", name: "发送到 Krita", hotkeys: [] })]);
  mountWithHooks(() => ({ id: ["img-1"] }));

  expect(activeHotkeys.value.filter((item) => item.category === "钩子动作")).toEqual([]);
});
