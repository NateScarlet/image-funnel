import { describe, test, expect, vi, beforeEach } from "vitest";
import { ref } from "vue";
import useBrowseImages from "./useBrowseImages";
import type { ImageFragment, ImageFiltersInput } from "@/graphql/generated";

// #region 数据源 mock 与测试数据工厂
// 模拟服务端返回的图片（可能因元数据变更而过期，不再匹配前端筛选）
const mockImages = ref<ImageFragment[]>([]);

vi.mock("./domain/useImage", () => ({
  useImageBrowse: () => ({
    images: mockImages,
    hasNextPage: ref(false),
    fetchMore: vi.fn(),
  }),
}));

function makeImage(overrides?: Partial<ImageFragment>): ImageFragment {
  return {
    __typename: "Image",
    id: "img-1",
    filename: "img-1.jpg",
    url: "",
    rawURL: "",
    modTime: "2026-01-01T00:00:00Z",
    width: 100,
    height: 100,
    size: 1024,
    currentRating: 0,
    label: null,
    relPath: "dir/img-1.jpg",
    url256: "",
    url512: "",
    url1024: "",
    url2048: "",
    url4096: "",
    note: {
      __typename: "Note",
      id: "note-1",
      relPath: "dir/img-1.md",
      content: "",
      rawContent: "",
      hidden: false,
      modTime: "2026-01-01T00:00:00Z",
    },
    ...overrides,
  };
}
// #endregion

describe("useBrowseImages 本地筛选隐藏统计", () => {
  beforeEach(() => {
    mockImages.value = [
      makeImage({ id: "img-a", filename: "a.jpg", currentRating: 5 }),
      makeImage({ id: "img-b", filename: "b.jpg", currentRating: 2 }),
    ];
  });

  test("应用本地筛选后不匹配的图片被隐藏并计入 hiddenImageCount", () => {
    const { images, hiddenImageCount, applyLocalFilter } = useBrowseImages(() => ({
      id: "dir-1",
      filterBy: { rating: [5] },
    }));

    // 服务端数据过期：img-b 已不符合 rating=5 筛选但仍在列表中
    expect(images.value.map((i) => i.id)).toEqual(["img-a", "img-b"]);

    applyLocalFilter();

    expect(images.value.map((i) => i.id)).toEqual(["img-a"]);
    expect(hiddenImageCount.value).toBe(1);
  });

  test("clearLocalFilter 恢复全部显示且计数归零", () => {
    const { images, hiddenImageCount, applyLocalFilter, clearLocalFilter } = useBrowseImages(
      () => ({
        id: "dir-1",
        filterBy: { rating: [5] },
      }),
    );

    applyLocalFilter();
    expect(images.value.map((i) => i.id)).toEqual(["img-a"]);
    expect(hiddenImageCount.value).toBe(1);

    clearLocalFilter();

    expect(images.value.map((i) => i.id)).toEqual(["img-a", "img-b"]);
    expect(hiddenImageCount.value).toBe(0);
  });

  test("筛选条件放宽后重新匹配的图片自动恢复显示且不计入隐藏", () => {
    const filterBy = ref<ImageFiltersInput>({ rating: [5] });
    const { images, hiddenImageCount, applyLocalFilter } = useBrowseImages(() => ({
      id: "dir-1",
      filterBy: filterBy.value,
    }));

    applyLocalFilter();
    expect(images.value.map((i) => i.id)).toEqual(["img-a"]);
    expect(hiddenImageCount.value).toBe(1);

    filterBy.value = { rating: [5, 2] };

    expect(images.value.map((i) => i.id)).toEqual(["img-a", "img-b"]);
    expect(hiddenImageCount.value).toBe(0);
  });
});
