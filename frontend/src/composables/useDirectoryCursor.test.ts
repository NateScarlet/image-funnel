import { beforeEach, describe, expect, test } from "vitest";
import {
  getBrowseCursor,
  getSessionCursor,
  setBrowseCursor,
  setSessionCursor,
} from "./useDirectoryCursor";
import type { DirectoryCursor } from "@/utils/directoryOrder";

const cursor: DirectoryCursor = {
  isForcedShown: false,
  hasStats: true,
  isEmpty: false,
  modTime: "2024-01-01T00:00:00.000Z",
};

describe("useDirectoryCursor", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setBrowseCursor(undefined);
  });

  test("筛选场景的游标按会话 ID 持久化到 sessionStorage", () => {
    setSessionCursor("session-a", cursor);

    expect(getSessionCursor("session-a")).toEqual(cursor);
    expect(getSessionCursor("session-b")).toBeUndefined();
  });

  test("浏览场景的游标只保存在模块内存中", () => {
    setBrowseCursor(cursor);

    expect(getBrowseCursor()).toEqual(cursor);
    expect(sessionStorage.length).toBe(0);
  });

  test("存储中的非法游标视为缺失", () => {
    sessionStorage.setItem("directoryCursor_0ea737683c03-session-a", "{not-json");
    expect(getSessionCursor("session-a")).toBeUndefined();

    sessionStorage.setItem("directoryCursor_0ea737683c03-session-a", JSON.stringify({ foo: 1 }));
    expect(getSessionCursor("session-a")).toBeUndefined();
  });
});
