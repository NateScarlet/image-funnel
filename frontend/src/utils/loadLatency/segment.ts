import type { RequestRecord, SegmentRecord } from "./types";

const encoder = new TextEncoder();

/** 一段序列化为单行 JSONL，换行是段的边界 */
export function serializeSegment(segment: SegmentRecord): string {
  return `${JSON.stringify(segment)}\n`;
}

/** 按 UTF-8 字节计体积；积压上限以此为准 */
export function byteLength(text: string): number {
  return encoder.encode(text).length;
}

/** 记录请求时的 URL 形态与请求宽度，便于读取侧按档位分组 */
export function classifyImageUrl(url: string): Pick<RequestRecord, "urlShape" | "width"> {
  try {
    const parsed = new URL(url, globalThis.location?.href);
    if (parsed.searchParams.has("raw")) {
      return { urlShape: "raw" };
    }
    const rawWidth = parsed.searchParams.get("w");
    if (rawWidth) {
      const width = Number(rawWidth);
      if (Number.isInteger(width) && width > 0) {
        return { urlShape: "width", width };
      }
    }
  } catch {
    // 非标准 URL 归入 other，不让诊断数据阻断记录
  }
  return { urlShape: "other" };
}
