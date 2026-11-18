import { describe, test, expect } from "vitest";
import Duration from "./Duration";

function expectInvalid(input: string) {
  const dur = Duration.parse(input);

  expect(dur.valid).toBe(false);
  expect(Number.isNaN(dur.toMilliseconds())).toBe(true);
}

describe("Duration.parse", () => {
  // 日期段未知单位：应判定为 invalid，避免被当成有效的部分解析结果
  test("P1Q 日期段直接出现未知单位时 valid 为 false", () => {
    expectInvalid("P1Q");
  });

  test("P1D2Q 日期段出现有效字段后再遇到未知单位时 valid 为 false", () => {
    expectInvalid("P1D2Q");
  });

  // 回归守护：时间段未知单位与其余错误路径行为不变
  test("PT1Q 时间段未知单位时 valid 仍为 false", () => {
    expectInvalid("PT1Q");
  });

  // 回归守护：正常保留期选项解析结果不变
  test.each([
    ["PT5M", 300_000],
    ["PT1H", 3_600_000],
    ["P1D", 86_400_000],
    ["P7D", 604_800_000],
    ["P30D", 2_592_000_000],
  ])("%s 解析为有效时长且毫秒值不变", (input, expectedMs) => {
    const dur = Duration.parse(input);

    expect(dur.valid).toBe(true);
    expect(dur.toMilliseconds()).toBe(expectedMs);
  });
});
