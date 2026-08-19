import { describe, it, expect } from "vitest";
import { getCoverImageUrl, getImageUrlByZoom } from "./image";
import type { ImageFragment } from "@/graphql/generated";

const mockImage = (width: number, height: number): ImageFragment =>
  ({
    id: "img-1",
    filename: "test.png",
    url: "https://example.com/raw.png",
    rawURL: "https://example.com/raw.png",
    url256: "https://example.com/256.png",
    url512: "https://example.com/512.png",
    url1024: "https://example.com/1024.png",
    url2048: "https://example.com/2048.png",
    url4096: "https://example.com/4096.png",
    modTime: "2026-08-19T00:00:00Z",
    width,
    height,
    size: 1024,
    currentRating: 0,
    label: null,
    relPath: "test.png",
  }) as ImageFragment;

describe("image utils", () => {
  describe("getImageUrlByZoom", () => {
    it("returns url256 for small target width", () => {
      const img = mockImage(1000, 1000);
      expect(getImageUrlByZoom(img, 0.2)).toBe("https://example.com/256.png");
    });
  });

  describe("getCoverImageUrl", () => {
    it("returns url256 for square image in 256x256 container", () => {
      const img = mockImage(1000, 1000);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/256.png");
    });

    it("returns url256 for tall portrait image in 256x256 container", () => {
      const img = mockImage(500, 2000);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/256.png");
    });

    it("returns url1024 for moderately wide image (1000x300, ratio 3.33)", () => {
      // required width = 256 * (1000 / 300) = 853px -> needs <= 1024
      const img = mockImage(1000, 300);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/1024.png");
    });

    it("returns url4096 for wide image (2000x200, ratio 10)", () => {
      // required width = 256 * 10 = 2560px -> needs <= 4096
      const img = mockImage(2000, 200);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/4096.png");
    });

    it("returns raw url for ultra wide image exceeding 4096px required width", () => {
      // required width = 256 * 20 = 5120px -> needs raw url
      const img = mockImage(5000, 250);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/raw.png");
    });

    it("falls back to url256 when width or height is 0/missing", () => {
      const img = mockImage(0, 0);
      expect(getCoverImageUrl(img, 256, 256)).toBe("https://example.com/256.png");
    });
  });
});
