import { describe, expect, it } from "vitest";
import { byteLength, classifyImageUrl, serializeSegment } from "./segment";
import type { SegmentRecord } from "./types";

const segment: SegmentRecord = {
  runId: "0123456789abcdef0123456789abcdef",
  segmentSeq: 1,
  startedAt: "2026-11-16T10:00:00.000Z",
  endedAt: "2026-11-16T10:00:01.000Z",
  gapBefore: false,
  actions: [],
};

describe("serializeSegment", () => {
  it("一行一段且以换行结尾", () => {
    const text = serializeSegment(segment);
    expect(text.endsWith("\n")).toBe(true);
    expect(text.trimEnd().split("\n")).toHaveLength(1);
    expect(JSON.parse(text)).toEqual(segment);
  });
});

describe("byteLength", () => {
  it("按 UTF-8 字节计，而非字符数", () => {
    expect(byteLength("abc")).toBe(3);
    expect(byteLength("中文")).toBe(6);
  });
});

describe("classifyImageUrl", () => {
  it("识别原图、指定宽度与其他形态", () => {
    expect(classifyImageUrl("http://host/image?raw&s=1")).toEqual({ urlShape: "raw" });
    expect(classifyImageUrl("http://host/image?w=1024&s=1")).toEqual({
      urlShape: "width",
      width: 1024,
    });
    expect(classifyImageUrl("http://host/image?s=1")).toEqual({ urlShape: "other" });
  });
});
