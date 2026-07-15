import { describe, test, expect } from "vitest";
import { getTypename, filterForPersistence } from "./cache-persistence";
import type { NormalizedCacheObject } from "@apollo/client/core";

// #region 测试数据

const EMPTY = {};

const BASE_CACHE: NormalizedCacheObject = {
  ROOT_QUERY: {
    rootDirectory: { __ref: "Directory:root" },
  },
};

const DIRECTORY_WITH_STATS: NormalizedCacheObject = {
  "Directory:root": {
    __typename: "Directory",
    id: "root",
    relPath: "",
    root: true,
    stats: {
      __typename: "DirectoryStats",
      imageCount: 10,
      subdirectoryCount: 2,
      latestImage: { __ref: "Image:photo1" },
      ratingCounts: [],
    },
  },
  "Directory:photos": {
    __typename: "Directory",
    id: "photos",
    parentId: "root",
    relPath: "photos",
    root: false,
    stats: {
      __typename: "DirectoryStats",
      imageCount: 5,
      subdirectoryCount: 0,
      latestImage: { __ref: "Image:photo2" },
      ratingCounts: [],
    },
  },
};

const DIRECTORY_WITHOUT_STATS: NormalizedCacheObject = {
  "Directory:empty": {
    __typename: "Directory",
    id: "empty",
    parentId: "root",
    relPath: "empty",
    root: false,
  },
};

const IMAGES: NormalizedCacheObject = {
  "Image:photo1": {
    __typename: "Image",
    id: "photo1",
    filename: "photo1.jpg",
  },
  "Image:photo2": {
    __typename: "Image",
    id: "photo2",
    filename: "photo2.jpg",
  },
  "Image:orphan": {
    __typename: "Image",
    id: "orphan",
    filename: "orphan.jpg",
  },
};

const IMAGES_WITH_MODTIME: NormalizedCacheObject = {
  "Image:old": {
    __typename: "Image",
    id: "old",
    filename: "old.jpg",
    modTime: "2024-01-01T00:00:00Z",
  },
  "Image:mid": {
    __typename: "Image",
    id: "mid",
    filename: "mid.jpg",
    modTime: "2025-06-15T00:00:00Z",
  },
  "Image:recent": {
    __typename: "Image",
    id: "recent",
    filename: "recent.jpg",
    modTime: "2026-07-01T00:00:00Z",
  },
};

const OTHER_ENTITIES: NormalizedCacheObject = {
  "Session:abc": {
    __typename: "Session",
    id: "abc",
    updatedAt: "2024-01-01T00:00:00Z",
  },
  "Device:dev1": {
    __typename: "Device",
    id: "dev1",
    name: "Device 1",
  },
  "Hook:hook1": {
    __typename: "Hook",
    id: "hook1",
    name: "Hook 1",
  },
  "Note:note1": {
    __typename: "Note",
    id: "note1",
    content: "Test note",
  },
};

// #endregion

// #region getTypename

describe("getTypename", () => {
  test("returns typename for entity key", () => {
    expect(getTypename("Session:abc")).toBe("Session");
    expect(getTypename("Image:xyz")).toBe("Image");
    expect(getTypename("Directory:root")).toBe("Directory");
  });

  test("returns undefined for non-entity key", () => {
    expect(getTypename("ROOT_QUERY")).toBeUndefined();
    expect(getTypename("ROOT_MUTATION")).toBeUndefined();
    expect(getTypename("MASK_foo")).toBeUndefined();
  });

  test("handles keys with multiple colons", () => {
    expect(getTypename("Image:img:abc")).toBe("Image");
  });

  test("handles empty string", () => {
    expect(getTypename("")).toBeUndefined();
  });
});

// #endregion

// #region filterForPersistence

describe("filterForPersistence", () => {
  test("keeps ROOT_QUERY", () => {
    const result = filterForPersistence(BASE_CACHE);
    expect(result).toHaveProperty("ROOT_QUERY");
  });

  test("keeps all Directory entities within limit", () => {
    const data = { ...BASE_CACHE, ...DIRECTORY_WITH_STATS, ...DIRECTORY_WITHOUT_STATS };
    const result = filterForPersistence(data);
    expect(result).toHaveProperty("Directory:root");
    expect(result).toHaveProperty("Directory:photos");
    expect(result).toHaveProperty("Directory:empty");
  });

  test("keeps Image entities referenced by directory stats", () => {
    const data = { ...BASE_CACHE, ...DIRECTORY_WITH_STATS, ...IMAGES };
    const result = filterForPersistence(data);
    expect(result).toHaveProperty("Image:photo1");
    expect(result).toHaveProperty("Image:photo2");
  });

  test("excludes Image entities not referenced by any directory", () => {
    const data = { ...BASE_CACHE, ...DIRECTORY_WITH_STATS, ...IMAGES };
    const result = filterForPersistence(data);
    expect(result).not.toHaveProperty("Image:orphan");
  });

  test("excludes all non-whitelisted entity types", () => {
    const data = { ...BASE_CACHE, ...DIRECTORY_WITH_STATS, ...OTHER_ENTITIES };
    const result = filterForPersistence(data);
    expect(result).not.toHaveProperty("Session:abc");
    expect(result).not.toHaveProperty("Device:dev1");
    expect(result).not.toHaveProperty("Hook:hook1");
    expect(result).not.toHaveProperty("Note:note1");
  });

  test("handles empty data", () => {
    const result = filterForPersistence(EMPTY);
    expect(result).toEqual({});
  });

  test("returns filtered data with only whitelisted entries", () => {
    const data = {
      ...BASE_CACHE,
      ...DIRECTORY_WITH_STATS,
      ...DIRECTORY_WITHOUT_STATS,
      ...IMAGES,
      ...OTHER_ENTITIES,
    };
    const result = filterForPersistence(data);
    const keys = Object.keys(result);
    expect(keys).toContain("ROOT_QUERY");
    expect(keys).toContain("Directory:root");
    expect(keys).toContain("Directory:photos");
    expect(keys).toContain("Directory:empty");
    expect(keys).toContain("Image:photo1");
    expect(keys).toContain("Image:photo2");
    expect(keys).not.toContain("Image:orphan");
    expect(keys).not.toContain("Session:abc");
    expect(keys).not.toContain("Device:dev1");
    expect(keys).not.toContain("Hook:hook1");
    expect(keys).not.toContain("Note:note1");
  });

  test("works with directory having stats but null latestImage", () => {
    const data: NormalizedCacheObject = {
      "Directory:novel": {
        __typename: "Directory",
        id: "novel",
        relPath: "novel",
        stats: {
          __typename: "DirectoryStats",
          imageCount: 0,
          subdirectoryCount: 0,
          latestImage: null,
          ratingCounts: [],
        },
      },
    };
    const result = filterForPersistence(data);
    expect(result).toHaveProperty("Directory:novel");
    expect(Object.keys(result).filter((k) => k.startsWith("Image:"))).toHaveLength(0);
  });

  test("limits directories to maxDirectories", () => {
    const data: NormalizedCacheObject = {
      ...BASE_CACHE,
      "Directory:a": { __typename: "Directory", id: "a", relPath: "a" },
      "Directory:b": { __typename: "Directory", id: "b", relPath: "b" },
      "Directory:c": { __typename: "Directory", id: "c", relPath: "c" },
    };
    const result = filterForPersistence(data, 2);
    expect(Object.keys(result).filter((k) => k.startsWith("Directory:"))).toHaveLength(2);
    expect(result).not.toHaveProperty("ROOT_MUTATION"); // sanity
  });

  test("handles invalid modTime without breaking sort", () => {
    const dirs: NormalizedCacheObject = {
      "Directory:a": {
        __typename: "Directory",
        id: "a",
        stats: { latestImage: { __ref: "Image:a" } },
      },
      "Directory:b": {
        __typename: "Directory",
        id: "b",
        stats: { latestImage: { __ref: "Image:b" } },
      },
    };
    const badImages: NormalizedCacheObject = {
      "Image:a": {
        __typename: "Image",
        id: "a",
        modTime: "not-a-date",
      },
      "Image:b": {
        __typename: "Image",
        id: "b",
        modTime: "also-invalid",
      },
    };
    const data = { ...BASE_CACHE, ...dirs, ...badImages };
    const result = filterForPersistence(data, 1);
    expect(Object.keys(result).filter((k) => k.startsWith("Directory:"))).toHaveLength(1);
  });

  test("prioritizes directories with recent images when over limit", () => {
    const dirs: NormalizedCacheObject = {
      "Directory:recent": {
        __typename: "Directory",
        id: "recent",
        relPath: "recent",
        stats: {
          latestImage: { __ref: "Image:recent" },
        },
      },
      "Directory:old": {
        __typename: "Directory",
        id: "old",
        relPath: "old",
        stats: {
          latestImage: { __ref: "Image:old" },
        },
      },
      "Directory:noimg": {
        __typename: "Directory",
        id: "noimg",
        relPath: "noimg",
      },
    };
    const data = { ...BASE_CACHE, ...dirs, ...IMAGES_WITH_MODTIME };
    const result = filterForPersistence(data, 2);
    expect(result).toHaveProperty("Directory:recent");
    expect(result).toHaveProperty("Directory:old");
    expect(result).not.toHaveProperty("Directory:noimg");
    expect(result).toHaveProperty("Image:recent");
    expect(result).toHaveProperty("Image:old");
    expect(result).not.toHaveProperty("Image:mid");
  });
});

// #endregion
