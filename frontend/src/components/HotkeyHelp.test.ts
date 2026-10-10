import { test, expect, afterEach, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import HotkeyHelp from "./HotkeyHelp.vue";
import { activeHotkeys } from "@/composables/useHotkeys";

beforeEach(() => {
  activeHotkeys.value = [
    { id: "a", keys: [["F8"]], description: "发送到 Krita", category: "钩子动作" },
    {
      id: "b",
      keys: [["F5"]],
      description: "在 ComfyUI 页面打开",
      category: "钩子动作",
      invalid: "conflict",
    },
  ];
});

afterEach(() => {
  activeHotkeys.value = [];
});

test("失效的声明式快捷键以错误样式的键帽呈现", () => {
  const wrapper = mount(HotkeyHelp);

  const conflictRow = wrapper.findAll("kbd").find((kbd) => kbd.text() === "F5");
  expect(conflictRow?.classes()).toContain("text-red-300");

  const validRow = wrapper.findAll("kbd").find((kbd) => kbd.text() === "F8");
  expect(validRow?.classes()).toContain("text-primary-100");
});

test("冲突标记紧跟在对应键位之后，正常项不显示标记", () => {
  const wrapper = mount(HotkeyHelp);

  const rows = wrapper.findAll("div.justify-between");
  const conflictRow = rows.find((row) => row.text().includes("ComfyUI"));
  expect(conflictRow?.text()).toContain("F5[冲突]");

  const validRow = rows.find((row) => row.text().includes("Krita"));
  expect(validRow?.text()).not.toContain("冲突");
});
