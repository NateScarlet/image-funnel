import { beforeEach, describe, expect, test } from "vitest";
import { migrate } from "./migration";

describe("migrate", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test("只清理旧目录顺序记录前缀的键", () => {
    localStorage.setItem("directoryOrder@122b3b541739-parent-a", "[]");
    localStorage.setItem("directoryOrder@122b3b541739-parent-b", "[]");
    localStorage.setItem("directoryOrder@other-prefix-parent", "[]");
    localStorage.setItem("unrelated", "1");

    migrate();

    expect(localStorage.getItem("directoryOrder@122b3b541739-parent-a")).toBeNull();
    expect(localStorage.getItem("directoryOrder@122b3b541739-parent-b")).toBeNull();
    expect(localStorage.getItem("directoryOrder@other-prefix-parent")).toBe("[]");
    expect(localStorage.getItem("unrelated")).toBe("1");
  });

  test("没有旧键时无操作且不报错", () => {
    localStorage.setItem("unrelated", "1");

    expect(() => migrate()).not.toThrow();
    expect(localStorage.getItem("unrelated")).toBe("1");
  });

  test("重复执行无副作用", () => {
    localStorage.setItem("directoryOrder@122b3b541739-parent", "[]");

    migrate();
    expect(() => migrate()).not.toThrow();
    expect(localStorage.getItem("directoryOrder@122b3b541739-parent")).toBeNull();
  });
});
