import { describe, expect, it } from "vitest";
import { detectEngine, detectRetrievalMethod, type ResourceTimingLike } from "./retrieval";

const ORIGIN = "http://localhost:34898";
const url = (path: string) => `${ORIGIN}/image?${path}`;

const network: ResourceTimingLike = { transferSize: 12345, encodedBodySize: 12000 };
const httpCache: ResourceTimingLike = { transferSize: 0, encodedBodySize: 12000 };

describe("detectEngine", () => {
  it("按引擎分支，未知 UA 保守归为 unknown", () => {
    expect(detectEngine("Mozilla/5.0 (Windows NT 10.0) Chrome/120.0 Safari/537.36")).toBe(
      "chromium",
    );
    expect(detectEngine("Mozilla/5.0 (X11; Linux) Gecko/20100101 Firefox/121.0")).toBe("firefox");
    expect(detectEngine("Mozilla/5.0 (Macintosh) Version/17.0 Safari/605.1.15")).toBe("safari");
    expect(detectEngine("curl/8.0")).toBe("unknown");
  });
});

describe("detectRetrievalMethod", () => {
  it("跨引擎判据只在同源下成立", () => {
    const foreign = "https://cdn.example.com/image";
    expect(detectRetrievalMethod(foreign, network, new Set([foreign]), "chromium", ORIGIN)).toBe(
      "unknown",
    );
  });

  it("已由本页预载进内存镜像的请求记为内存缓存", () => {
    const target = url("a");
    expect(detectRetrievalMethod(target, network, new Set([target]), "chromium", ORIGIN)).toBe(
      "memory-cache",
    );
  });

  it("Chromium 依据 transferSize 区分网络与 HTTP 缓存", () => {
    expect(detectRetrievalMethod(url("a"), network, new Set(), "chromium", ORIGIN)).toBe("network");
    expect(detectRetrievalMethod(url("a"), httpCache, new Set(), "chromium", ORIGIN)).toBe(
      "http-cache",
    );
  });

  it("无法可靠判定时记未知，而不是猜错", () => {
    expect(detectRetrievalMethod(url("a"), network, new Set(), "firefox", ORIGIN)).toBe("unknown");
    expect(detectRetrievalMethod(url("a"), network, new Set(), "safari", ORIGIN)).toBe("unknown");
    expect(detectRetrievalMethod(url("a"), undefined, new Set(), "chromium", ORIGIN)).toBe(
      "unknown",
    );
  });
});
