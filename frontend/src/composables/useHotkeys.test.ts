import { test, expect, vi, afterEach } from "vitest";
import { mount } from "@vue/test-utils";
import { defineComponent, h, ref, nextTick } from "vue";
import { useHotkeys, activeHotkeys, type HotkeyBindings } from "./useHotkeys";

// 注册表是模块级单例，通过卸载宿主组件清理，避免用例之间互相污染
const wrappers: ReturnType<typeof mount>[] = [];

afterEach(() => {
  while (wrappers.length > 0) {
    wrappers.pop()?.unmount();
  }
});

function mountWithHotkeys(setup: () => unknown) {
  const wrapper = mount(
    defineComponent({
      setup() {
        const render = setup();
        return typeof render === "function" ? (render as () => unknown) : () => null;
      },
    }),
  );
  wrappers.push(wrapper);
  return wrapper;
}

function pressKey(key: string, init: KeyboardEventInit = {}) {
  const event = new KeyboardEvent("keydown", {
    key,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  window.dispatchEvent(event);
  return event;
}

function listedKeys() {
  return activeHotkeys.value.map((item) => `${item.keys[0].join("+")}:${item.invalid ?? "ok"}`);
}

// #region 应用自身快捷键

test("应用快捷键命中后触发 handler", () => {
  const handler = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys({ f9: handler }, { description: "应用快捷键" });
  });

  pressKey("F9");
  expect(handler).toHaveBeenCalledTimes(1);
  expect(listedKeys()).toEqual(["F9:ok"]);
});

test("输入框聚焦时快捷键失效", () => {
  const handler = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys({ f9: handler }, { description: "应用快捷键" });
  });

  const input = document.createElement("input");
  input.dispatchEvent(new KeyboardEvent("keydown", { key: "F9", bubbles: true, cancelable: true }));
  expect(handler).not.toHaveBeenCalled();
});

test("输入框聚焦时声明式快捷键同样失效", () => {
  const handler = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys([{ keys: "F9", handler, options: { description: "钩子动作", declared: "hk:a" } }]);
  });

  const input = document.createElement("input");
  input.dispatchEvent(new KeyboardEvent("keydown", { key: "F9", bubbles: true, cancelable: true }));
  expect(handler).not.toHaveBeenCalled();
});

// #endregion

// #region 声明式快捷键的冲突降级

test("声明式快捷键与应用快捷键冲突时不触发，并以冲突登记在帮助列表中", () => {
  const app = vi.fn();
  const declared = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys({ f9: app }, { description: "应用快捷键" });
  });
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "f9", handler: declared, options: { description: "钩子动作", declared: "hk:a" } },
    ]);
  });

  pressKey("F9");
  expect(app).toHaveBeenCalledTimes(1);
  expect(declared).not.toHaveBeenCalled();
  expect(listedKeys()).toEqual(["F9:ok", "F9:conflict"]);
});

test("声明式快捷键的冲突按语义比对，不区分大小写与修饰键别名", () => {
  const declared = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys({ "ctrl+k": vi.fn() }, { description: "应用快捷键" });
  });
  mountWithHotkeys(() => {
    useHotkeys([
      {
        keys: "Control+K",
        handler: declared,
        options: { description: "钩子动作", declared: "hk:a" },
      },
    ]);
  });

  pressKey("k", { ctrlKey: true });
  expect(declared).not.toHaveBeenCalled();
  expect(listedKeys()).toContain("Ctrl+K:conflict");
});

test("两个声明占用同一组合键时双方都失效，且不拦截按键事件", () => {
  const first = vi.fn();
  const second = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "F9", handler: first, options: { description: "钩子甲", declared: "hk:a" } },
    ]);
    useHotkeys([
      { keys: "F9", handler: second, options: { description: "钩子乙", declared: "hk:b" } },
    ]);
  });

  const event = pressKey("F9");
  expect(first).not.toHaveBeenCalled();
  expect(second).not.toHaveBeenCalled();
  expect(event.defaultPrevented).toBe(false);
  expect(listedKeys()).toEqual(["F9:conflict", "F9:conflict"]);
});

test("同一声明的多个键逐键独立生效，一个冲突不拖累其余", () => {
  const handler = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys({ f9: vi.fn() }, { description: "应用快捷键" });
  });
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: ["F9", "F8"], handler, options: { description: "钩子动作", declared: "hk:a" } },
    ]);
  });

  pressKey("F9");
  expect(handler).not.toHaveBeenCalled();

  pressKey("F8");
  expect(handler).toHaveBeenCalledTimes(1);
  expect(listedKeys()).toEqual(["F9:ok", "F9:conflict", "F8:ok"]);
});

test("同一声明内重复的键只登记与注册一次", () => {
  const handler = vi.fn();
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: ["F8", "f8", "F8"], handler, options: { description: "钩子动作", declared: "hk:a" } },
    ]);
  });

  pressKey("F8");
  expect(handler).toHaveBeenCalledTimes(1);
  expect(listedKeys()).toEqual(["F8:ok"]);
});

test("冲突的应用快捷键注销后声明式快捷键恢复可用", () => {
  const declared = vi.fn();
  const app = mountWithHotkeys(() => {
    useHotkeys({ f9: vi.fn() }, { description: "应用快捷键" });
  });
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "F9", handler: declared, options: { description: "钩子动作", declared: "hk:a" } },
    ]);
  });
  expect(listedKeys()).toEqual(["F9:ok", "F9:conflict"]);

  app.unmount();
  wrappers.splice(wrappers.indexOf(app), 1);

  pressKey("F9");
  expect(declared).toHaveBeenCalledTimes(1);
  expect(listedKeys()).toEqual(["F9:ok"]);
});

// #endregion

// #region 同一声明在多个上下文中的绑定

test("同一声明标识绑定到两个注册点时不判为冲突", () => {
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "发送到 Krita", declared: "hk:a" } },
    ]);
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "发送到 Krita", declared: "hk:a" } },
    ]);
  });

  expect(listedKeys()).toEqual(["F9:ok", "F9:ok"]);
});

test("同一声明标识在两个上下文中的绑定仍各自与应用快捷键撞键", () => {
  mountWithHotkeys(() => {
    useHotkeys({ f9: vi.fn() }, { description: "应用快捷键" });
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "发送到 Krita", declared: "hk:a" } },
    ]);
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "发送到 Krita", declared: "hk:a" } },
    ]);
  });

  expect(listedKeys()).toEqual(["F9:ok", "F9:conflict", "F9:conflict"]);
});

test("两个不同声明占用同一组合键时仍然双方都失效", () => {
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "钩子甲", declared: "hk:a" } },
    ]);
    useHotkeys([
      { keys: "F9", handler: vi.fn(), options: { description: "钩子乙", declared: "hk:b" } },
    ]);
  });

  expect(listedKeys()).toEqual(["F9:conflict", "F9:conflict"]);
});

test("scope 决定同一声明的两个上下文中当下生效的那一个", async () => {
  const viewerScope = ref<string | undefined>(undefined);
  const viewer = vi.fn();
  const batch = vi.fn();

  // 查看器上下文：父组件声明 scope，子组件在其下注册快捷键
  const viewerChild = defineComponent({
    setup() {
      useHotkeys([
        { keys: "F9", handler: viewer, options: { description: "发送到 Krita", declared: "hk:a" } },
      ]);
      return () => null;
    },
  });
  mountWithHotkeys(() => {
    useHotkeys({ defineScope: viewerScope });
    return () => h(viewerChild);
  });
  // 网格上下文：没有压入任何 scope
  mountWithHotkeys(() => {
    useHotkeys([
      { keys: "F9", handler: batch, options: { description: "发送到 Krita", declared: "hk:a" } },
    ]);
  });

  pressKey("F9");
  expect(batch).toHaveBeenCalledTimes(1);
  expect(viewer).not.toHaveBeenCalled();

  viewerScope.value = "viewer";
  await nextTick();
  pressKey("F9");
  expect(viewer).toHaveBeenCalledTimes(1);
  expect(batch).toHaveBeenCalledTimes(1);
});

// #endregion

// #region 响应式绑定

test("绑定内容变化时整体重新注册", async () => {
  const handler = vi.fn();
  const bindings = ref<HotkeyBindings>([{ keys: "F8", handler }]);
  mountWithHotkeys(() => {
    useHotkeys(bindings, { description: "钩子动作" });
  });

  bindings.value = [{ keys: "F7", handler }];
  await nextTick();

  pressKey("F8");
  expect(handler).not.toHaveBeenCalled();

  pressKey("F7");
  expect(handler).toHaveBeenCalledTimes(1);
  expect(listedKeys()).toEqual(["F7:ok"]);
});

// #endregion
