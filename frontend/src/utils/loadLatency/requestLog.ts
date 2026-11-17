import type Time from "@/utils/Time";
import { detectEngine, detectRetrievalMethod, type ResourceTimingLike } from "./retrieval";
import { classifyImageUrl } from "./segment";
import { parseServerTiming, type ServerTimingMetric } from "./serverTiming";
import type { RequestRecord } from "./types";

// PerformanceServerTiming 在部分浏览器的 lib.dom 中缺失，按实际字段做最小声明
interface ResourceEntryLike extends ResourceTimingLike {
  name: string;
  startTime: number;
  responseStart: number;
  responseEnd: number;
  serverTiming?: { name: string; duration: number; description?: string }[];
}

/** 在资源计时里找该 URL 最近的一条；同源才有完整分段，跨引擎记 unknown */
function findResourceEntry(url: string): ResourceEntryLike | undefined {
  if (typeof performance === "undefined" || typeof performance.getEntriesByType !== "function") {
    return undefined;
  }
  let found: ResourceEntryLike | undefined;
  for (const entry of performance.getEntriesByType("resource") as unknown as ResourceEntryLike[]) {
    if (entry.name === url) {
      found = entry;
    }
  }
  return found;
}

/**
 * 由一次图片请求构造归因记录。
 * 请求计时来自浏览器资源计时，服务端分段来自 Server-Timing；
 * 取回方式按引擎分支判定，无从判断时记 unknown。
 */
export function buildRequestRecord(
  url: string,
  startedAt: Time,
  decodeMs: number,
  preloadedUrls: ReadonlySet<string>,
  serverTimingHeader?: string | null,
): RequestRecord {
  const entry = findResourceEntry(url);
  // 同源 HEAD 响应能直接读到原始 Server-Timing；其余情况退回资源计时已解析的条目
  const serverTiming: ServerTimingMetric[] = serverTimingHeader
    ? parseServerTiming(serverTimingHeader)
    : (entry?.serverTiming ?? []).map((metric) => ({
        name: metric.name,
        durationMs: metric.duration,
        ...(metric.description ? { description: metric.description } : {}),
      }));
  return {
    ...classifyImageUrl(url),
    startedAt: startedAt.toISOString(),
    ttfbMs: entry ? Math.max(0, entry.responseStart - entry.startTime) : 0,
    downloadMs: entry ? Math.max(0, entry.responseEnd - entry.responseStart) : 0,
    decodeMs,
    bytes: entry ? entry.transferSize || entry.encodedBodySize || 0 : 0,
    retrieval: detectRetrievalMethod(url, entry, preloadedUrls, detectEngine(navigator.userAgent)),
    serverTiming,
  };
}
