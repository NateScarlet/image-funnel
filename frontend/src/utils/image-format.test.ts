import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ImageFormat } from "@/graphql/generated";

// 测试 isImageFormatSupported 的缓存与记忆化逻辑
// 因为 isImageFormatSupported 依赖浏览器 API（createImageBitmap / Image），
// 在 vitest 的 jsdom 环境中需要 mock 这些 API

describe("isImageFormatSupported", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("returns true for universally supported formats without async check", async () => {
    const { default: isImageFormatSupported } = await import("./image-format");
    const result = isImageFormatSupported("image/png");
    expect(result.value).toBe(true);
    expect(await result).toBe(true);
  });

  it("caches result after first check", async () => {
    const { default: isImageFormatSupported } = await import("./image-format");

    // 第一次调用
    const result1 = isImageFormatSupported("image/png");
    // 第二次调用应该返回相同的缓存对象
    const result2 = isImageFormatSupported("image/png");

    expect(result1).toBe(result2);
    expect(result1.value).toBe(true);
  });

  it("returns false for unsupported format without data URL", async () => {
    const { default: isImageFormatSupported } = await import("./image-format");
    // HEIC 在测试环境中不支持，且没有 data URL
    const result = isImageFormatSupported("image/heic");
    expect(result.value).toBe(false);
    expect(await result).toBe(false);
  });
});

describe("getPreferredFormat", () => {
  it("returns WEBP when AVIF is not supported", async () => {
    // Mock isImageFormatSupported to return false for AVIF
    vi.doMock("./image-format", () => {
      const actual = vi.importActual("./image-format");
      return actual;
    });

    // 在 jsdom 环境中 createImageBitmap 不存在，AVIF 探测会走 Image 路径
    // jsdom 的 Image 不会触发 onload，所以会返回 false
    const { getPreferredFormat } = await import("./image-format");
    const format = getPreferredFormat();
    expect(format).toBe("WEBP" as ImageFormat);
  });
});
