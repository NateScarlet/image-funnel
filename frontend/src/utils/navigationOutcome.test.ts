import { describe, expect, test } from "vitest";
import { classifyNavigationOutcome, type ViewSample } from "./navigationOutcome";

const before = "/session/session-1";

function sample(route: string, completed: boolean): ViewSample {
  return { route, completed };
}

describe("导航结果判定", () => {
  test("路由与完成视图都变化视为成功", () => {
    expect(
      classifyNavigationOutcome(
        before,
        sample("/session/session-2", false),
        sample("/session/session-2", false),
      ),
    ).toBe("switched");
  });

  test("路由没变且完成视图仍在视为未切换", () => {
    expect(classifyNavigationOutcome(before, sample(before, true), sample(before, true))).toBe(
      "unchanged",
    );
  });

  test("路由变了但完成视图仍在也视为未切换", () => {
    expect(
      classifyNavigationOutcome(
        before,
        sample("/session/session-2", true),
        sample("/session/session-2", true),
      ),
    ).toBe("unchanged");
  });

  test("完成视图先消失又重新出现视为闪烁", () => {
    expect(
      classifyNavigationOutcome(
        before,
        sample("/session/session-2", false),
        sample("/session/session-2", true),
      ),
    ).toBe("flicker");
  });
});
