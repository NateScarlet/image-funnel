import { test, expect, vi, afterEach } from "vitest";
import { mount } from "@vue/test-utils";
import { defineComponent, h, ref, nextTick } from "vue";
import HotkeyHelp from "./HotkeyHelp.vue";
import { useHotkeys } from "@/composables/useHotkeys";

// 注册表是模块级单例，通过卸载宿主组件清理，避免用例之间互相污染
const wrappers: ReturnType<typeof mount>[] = [];

afterEach(() => {
  while (wrappers.length > 0) {
    wrappers.pop()?.unmount();
  }
});

function pressF8() {
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "F8", bubbles: true, cancelable: true }),
  );
}

function mountHelp() {
  const wrapper = mount(
    defineComponent({
      setup() {
        useHotkeys(
          { f8: () => {} },
          { description: "发送到 Krita", category: "钩子动作", declared: "hk:a" },
        );
        useHotkeys(
          { f5: () => {} },
          { description: "在 ComfyUI 页面打开", category: "钩子动作", declared: "hk:b" },
        );
        // 无描述的应用快捷键参与撞键判定，但不在帮助列表中出现
        useHotkeys({ f5: () => {} });
        return () => h(HotkeyHelp);
      },
    }),
  );
  wrappers.push(wrapper);
  return wrapper;
}

test("失效的声明式快捷键以错误样式的键帽呈现", () => {
  const wrapper = mountHelp();

  const conflictRow = wrapper.findAll("kbd").find((kbd) => kbd.text() === "F5");
  expect(conflictRow?.classes()).toContain("text-red-300");

  const validRow = wrapper.findAll("kbd").find((kbd) => kbd.text() === "F8");
  expect(validRow?.classes()).toContain("text-primary-100");
});

test("冲突标记紧跟在对应键位之后，正常项不显示标记", () => {
  const wrapper = mountHelp();

  const rows = wrapper.findAll("div.justify-between");
  const conflictRow = rows.find((row) => row.text().includes("ComfyUI"));
  expect(conflictRow?.text()).toContain("F5[冲突]");

  const validRow = rows.find((row) => row.text().includes("Krita"));
  expect(validRow?.text()).not.toContain("冲突");
});

test("派发与帮助列表对 enabledWhen 的同一声明得出一致结论", async () => {
  const enabled = ref(true);
  const handler = vi.fn();
  const wrapper = mount(
    defineComponent({
      setup() {
        useHotkeys({ F8: handler }, { description: "条件动作", enabledWhen: () => enabled.value });
        return () => h(HotkeyHelp);
      },
    }),
  );
  wrappers.push(wrapper);

  pressF8();
  expect(handler).toHaveBeenCalledTimes(1);
  expect(wrapper.text()).toContain("条件动作");

  enabled.value = false;
  await nextTick();
  pressF8();
  expect(handler).toHaveBeenCalledTimes(1);
  expect(wrapper.text()).not.toContain("条件动作");
});

test("帮助列表展示所有作用域的快捷键，不因当前作用域改变而减少条目", async () => {
  const scope = ref<string | undefined>(undefined);
  const scopedChild = defineComponent({
    setup() {
      useHotkeys({ F8: () => {} }, { description: "作用域内动作", declared: "hk:a" });
      return () => null;
    },
  });
  const wrapper = mount(
    defineComponent({
      setup() {
        useHotkeys([], { defineScope: scope });
        useHotkeys({ F7: () => {} }, { description: "无作用域动作", declared: "hk:b" });
        return () => [h(scopedChild), h(HotkeyHelp)];
      },
    }),
  );
  wrappers.push(wrapper);

  expect(wrapper.text()).toContain("作用域内动作");
  expect(wrapper.text()).toContain("无作用域动作");

  scope.value = "viewer";
  await nextTick();
  expect(wrapper.text()).toContain("作用域内动作");
  expect(wrapper.text()).toContain("无作用域动作");
});
