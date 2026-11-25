import { describe, expect, test } from "vitest";
import { newSalt, pseudonymize, scrubEvent, scrubId, type ScrubbableEvent } from "./telemetryScrub";

const SALT = "00112233445566778899aabbccddeeff";

function sampleEvent(): ScrubbableEvent {
  return {
    message: "post-commit-navigation:error-classified dir:subdir/风景/a.png",
    request: { url: "http://localhost:34898/session/session-1" },
    exception: {
      values: [
        {
          type: "Error",
          value: "无法打开 dir:subdir/风景/a.png",
          stacktrace: {
            frames: [
              {
                filename: "C:\\Users\\private\\风景\\a.png",
                abs_path: "dir:subdir/风景/a.png",
                function: "render 风景/a.png",
                module: "subdir/风景",
                context_line: "img:123:subdir/风景/a.png",
                pre_context: ["dir:subdir/风景"],
                post_context: ["C:\\Users\\private\\风景\\b.png"],
              },
            ],
          },
        },
      ],
    },
    extra: {
      step: "error-classified",
      sessionId: "session-1",
      directoryId: "dir:subdir/风景",
      directoryRelPath: "subdir/风景",
      parentId: "dir:parent/私密目录",
      nextDirectoryId: "dir:subdir/下一项",
      imageId: "img:1700000000000000000:subdir/风景/a.png",
      cursor: { after: "e5b09f", sortKey: "page\x00dir", query: "风景" },
      stats: [{ id: "dir:subdir/风景", isCompleted: false }],
      statsSnapshot: { totalCount: 2, isCompleted: false },
      decisionBranch: "paged",
      errorClass: "local",
      message: "打开 C:\\Users\\private\\风景\\a.png 失败",
    },
    breadcrumbs: [
      {
        message: "post-commit-navigation:decision-resolved",
        data: {
          directoryId: "dir:subdir/风景",
          sessionId: "session-1",
          stats: [{ id: "dir:subdir/下一项", isCompleted: true }],
        },
      },
      {
        message: "http://localhost:34898/image/sig/subdir/风景/a.png?sig=secret",
        data: { url: "http://localhost:34898/image/sig/subdir/风景/a.png?sig=secret" },
      },
    ],
  };
}

describe("发送前脱敏", () => {
  test("所有 ID、路径与搜索词都不出现在序列化事件中", () => {
    const result = scrubEvent(sampleEvent(), SALT);
    const serialized = JSON.stringify(result);

    for (const secret of [
      "session-1",
      "dir:subdir/风景",
      "subdir/风景",
      "dir:subdir/下一项",
      "dir:parent/私密目录",
      "img:1700000000000000000:subdir/风景/a.png",
      "风景",
      "e5b09f",
      "page\x00dir",
      "sig=secret",
      "Users\\private",
    ]) {
      expect(serialized).not.toContain(secret);
    }
  });

  test("盐随事件下发，同一事件内的相同原始值得到相同伪标识", () => {
    const result = scrubEvent(sampleEvent(), SALT);

    expect(result.extra?.scrubSalt).toBe(SALT);
    expect(result.extra?.directoryId).toBe(result.breadcrumbs?.[0]?.data?.directoryId);
    expect(result.extra?.sessionId).toBe(result.breadcrumbs?.[0]?.data?.sessionId);
    expect(result.extra?.directoryId).toMatch(/^dir:[0-9a-f]{16}$/);
    expect(result.extra?.imageId).toMatch(/^img:[0-9a-f]{16}$/);
    expect(result.extra?.sessionId).toMatch(/^session:[0-9a-f]{16}$/);
  });

  test("不同盐得到不同伪标识，同一原始值的哈希可复算还原", () => {
    const first = scrubEvent(sampleEvent(), SALT);
    const second = scrubEvent(sampleEvent(), "ffeeddccbbaa99887766554433221100");

    expect(first.extra?.directoryId).not.toBe(second.extra?.directoryId);
    expect(first.extra?.directoryId).toBe(scrubId("dir:subdir/风景", SALT, "dir:"));
    expect(pseudonymize("dir:subdir/风景", SALT)).toHaveLength(16);
  });

  test("应用数据原样保留，只有敏感字段被替换", () => {
    const result = scrubEvent(sampleEvent(), SALT);

    expect(result.extra?.step).toBe("error-classified");
    expect(result.extra?.decisionBranch).toBe("paged");
    expect(result.extra?.errorClass).toBe("local");
    expect(result.extra?.statsSnapshot).toEqual({ totalCount: 2, isCompleted: false });
    const stats = result.extra?.stats as { id: string; isCompleted: boolean }[];
    expect(stats[0]?.isCompleted).toBe(false);
    expect(result.message).toContain("post-commit-navigation:error-classified");
    expect(result.message).not.toContain("subdir/风景");
    expect(result.message).toMatch(/dir:[0-9a-f]{16}$/);
  });

  test("每次事件生成不同的随机盐", () => {
    const first = newSalt();
    const second = newSalt();

    expect(first).toMatch(/^[0-9a-f]{32}$/);
    expect(first).not.toBe(second);
  });
});
