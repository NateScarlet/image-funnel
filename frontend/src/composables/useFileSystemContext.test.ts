import { describe, test, expect, vi, beforeEach } from "vitest";
import { computed, ref } from "vue";

vi.mock("@/graphql/utils/useQuery", () => ({ default: vi.fn() }));

import useQuery from "@/graphql/utils/useQuery";
import { resolveAbsPath, resolveDisplayName, useFileSystemContext } from "./useFileSystemContext";
import type { MetaQuery } from "@/graphql/generated";

// #region 纯函数内核测试

describe("resolveAbsPath", () => {
  test("根目录的零值根相对路径解析为根绝对路径本身", () => {
    expect(resolveAbsPath("C:\\Pictures", "")).toBe("C:\\Pictures");
    expect(resolveAbsPath("/mnt/pics", "")).toBe("/mnt/pics");
  });

  test("未加载的根相对路径解析为空串，与根目录产生不同结果", () => {
    expect(resolveAbsPath("C:\\Pictures", undefined)).toBe("");
    expect(resolveAbsPath("C:\\Pictures", undefined)).not.toBe(resolveAbsPath("C:\\Pictures", ""));
  });

  test("根绝对路径缺失时一律返回空串", () => {
    expect(resolveAbsPath(undefined, "a/b.jpg")).toBe("");
    expect(resolveAbsPath("", "a/b.jpg")).toBe("");
    expect(resolveAbsPath(undefined, "")).toBe("");
  });

  test("Windows 风格根路径按反斜杠拼接并转换相对路径分隔符", () => {
    expect(resolveAbsPath("C:\\Pictures", "a/b/c.jpg")).toBe("C:\\Pictures\\a\\b\\c.jpg");
    expect(resolveAbsPath("C:\\Pictures\\", "a\\b")).toBe("C:\\Pictures\\\\a\\b");
  });

  test("Unix 风格根路径按斜杠拼接并转换相对路径分隔符", () => {
    expect(resolveAbsPath("/mnt/pics", "a/b/c.jpg")).toBe("/mnt/pics/a/b/c.jpg");
    expect(resolveAbsPath("/mnt/pics", "a\\b")).toBe("/mnt/pics/a/b");
  });
});

describe("resolveDisplayName", () => {
  test("根目录显示根绝对路径本身", () => {
    expect(resolveDisplayName("C:\\Pictures", "")).toBe("C:\\Pictures");
  });

  test("未加载的根相对路径显示为空串，与根目录产生不同结果", () => {
    expect(resolveDisplayName("C:\\Pictures", undefined)).toBe("");
    expect(resolveDisplayName("C:\\Pictures", undefined)).not.toBe(
      resolveDisplayName("C:\\Pictures", ""),
    );
  });

  test("根绝对路径缺失时一律返回空串", () => {
    expect(resolveDisplayName(undefined, "a/b.jpg")).toBe("");
    expect(resolveDisplayName("", "a/b.jpg")).toBe("");
    expect(resolveDisplayName(undefined, "")).toBe("");
  });

  test("非根目录取 basename", () => {
    expect(resolveDisplayName("C:\\Pictures", "a/b/c.jpg")).toBe("c.jpg");
    expect(resolveDisplayName("/mnt/pics", "a\\b")).toBe("b");
    expect(resolveDisplayName("/mnt/pics", "c.jpg")).toBe("c.jpg");
  });
});
// #endregion

// #region 组合式函数测试

describe("useFileSystemContext", () => {
  const rootAbsPath = ref<string | undefined>("C:\\Pictures");

  beforeEach(() => {
    rootAbsPath.value = "C:\\Pictures";
    vi.mocked(useQuery).mockImplementation(
      () =>
        ({
          data: computed(() =>
            rootAbsPath.value === undefined
              ? undefined
              : ({ meta: { rootAbsPath: rootAbsPath.value } } as MetaQuery),
          ),
        }) as never,
    );
  });

  test("根路径来自服务端元数据", () => {
    const { rootPath } = useFileSystemContext();
    expect(rootPath.value).toBe("C:\\Pictures");
  });

  test("元数据未加载时根路径为空串且所有推导为空串", () => {
    rootAbsPath.value = undefined;
    const { rootPath, absPath, displayName } = useFileSystemContext();
    expect(rootPath.value).toBe("");
    expect(absPath("a/b.jpg")).toBe("");
    expect(displayName("a/b.jpg")).toBe("");
  });

  test("推导结果跟随根路径变化", () => {
    const { absPath, displayName } = useFileSystemContext();
    expect(absPath("a/b.jpg")).toBe("C:\\Pictures\\a\\b.jpg");
    expect(displayName("a/b.jpg")).toBe("b.jpg");

    rootAbsPath.value = "/mnt/pics";
    expect(absPath("a/b.jpg")).toBe("/mnt/pics/a/b.jpg");
  });
});
// #endregion
