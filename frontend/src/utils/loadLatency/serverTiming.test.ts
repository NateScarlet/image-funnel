import { describe, expect, it } from "vitest";
import { parseServerTiming } from "./serverTiming";

describe("parseServerTiming", () => {
  it("解析服务端下发的四项分段耗时", () => {
    const metrics = parseServerTiming(
      'queue;dur=12.34, decode;dur=0.50, encode;dur=88.00;desc="svt-av1", cache;dur=0;desc="hit"',
    );

    expect(metrics).toEqual([
      { name: "queue", durationMs: 12.34 },
      { name: "decode", durationMs: 0.5 },
      { name: "encode", durationMs: 88, description: "svt-av1" },
      { name: "cache", durationMs: 0, description: "hit" },
    ]);
  });

  it("desc 中的逗号与转义字符不破坏切分", () => {
    const metrics = parseServerTiming('encode;dur=1;desc="a,b\\"c"');
    expect(metrics).toHaveLength(1);
    expect(metrics[0].description).toBe('a,b"c');
  });

  it("缺失 dur 的指标按 0 处理，空头返回空数组", () => {
    expect(parseServerTiming('cache;desc="miss"')[0].durationMs).toBe(0);
    expect(parseServerTiming("")).toEqual([]);
  });
});
