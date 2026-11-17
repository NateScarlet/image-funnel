/** 图片请求的取回方式。无从判断时一律记 unknown，不做猜测 */
export type RetrievalMethod = "network" | "http-cache" | "memory-cache" | "unknown";

/** 浏览器引擎分支。判据只在 Chromium 上可靠 */
export type Engine = "chromium" | "firefox" | "safari" | "unknown";

/** 判定所需的最小资源计时信息 */
export interface ResourceTimingLike {
  transferSize: number;
  encodedBodySize: number;
}

export function detectEngine(userAgent: string): Engine {
  if (/Firefox\//.test(userAgent)) {
    return "firefox";
  }
  if (/Edg\/|Chrome\/|Chromium\//.test(userAgent)) {
    return "chromium";
  }
  if (/Safari\//.test(userAgent)) {
    return "safari";
  }
  return "unknown";
}

/**
 * 判定一次图片请求的取回方式。
 *
 * 同源才成立；本页已预载进内存镜像的 URL 直接归为 memory-cache；
 * Chromium 上再依据 transferSize 区分网络与 HTTP 缓存，其余引擎记 unknown。
 */
export function detectRetrievalMethod(
  targetUrl: string,
  timing: ResourceTimingLike | undefined,
  preloadedUrls: ReadonlySet<string>,
  engine: Engine,
  currentOrigin: string = globalThis.location?.origin ?? "",
): RetrievalMethod {
  let parsed: URL;
  try {
    parsed = new URL(targetUrl, currentOrigin);
  } catch {
    return "unknown";
  }
  if (parsed.origin !== currentOrigin) {
    return "unknown";
  }
  const href = parsed.href;
  for (const preloaded of preloadedUrls) {
    try {
      if (new URL(preloaded, currentOrigin).href === href) {
        return "memory-cache";
      }
    } catch {
      // 预载集合里出现坏 URL 不影响其余判定
    }
  }
  if (engine !== "chromium" || !timing) {
    return "unknown";
  }
  if (timing.transferSize > 0) {
    return "network";
  }
  if (timing.transferSize === 0 && timing.encodedBodySize > 0) {
    return "http-cache";
  }
  return "unknown";
}
