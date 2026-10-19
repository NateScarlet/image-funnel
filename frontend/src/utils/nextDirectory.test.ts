import { describe, test, expect } from "vitest";
import { getNextDirectory } from "./nextDirectory";

const anyDirectory = () => true;

describe("getNextDirectory", () => {
  test("返回当前目录之后的第一项", () => {
    expect(getNextDirectory(["a", "b", "c"], "a", anyDirectory)).toBe("b");
  });

  test("跳过不可进入的目录，返回之后第一个可进入的目录", () => {
    const target = "d";
    const isEnterable = (id: string) => id === target;
    expect(getNextDirectory(["a", "b", "c", "d", "e"], "a", isEnterable)).toBe("d");
  });

  test("当前目录是最后一项时没有下一个目录", () => {
    expect(getNextDirectory(["a", "b"], "b", anyDirectory)).toBeUndefined();
  });

  test("当前目录不在列表中时没有下一个目录", () => {
    expect(getNextDirectory(["a", "b"], "c", anyDirectory)).toBeUndefined();
  });

  test("空列表没有下一个目录", () => {
    expect(getNextDirectory([], "a", anyDirectory)).toBeUndefined();
  });

  test("后续项都不可进入时返回 undefined 而不是回退到当前目录", () => {
    expect(getNextDirectory(["a", "b"], "a", () => false)).toBeUndefined();
  });
});
