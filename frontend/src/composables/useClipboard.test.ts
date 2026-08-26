import { describe, test, expect, vi, beforeEach } from "vitest";

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutate: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/graphql/utils/query", () => ({ default: mocks.query }));
vi.mock("@/graphql/utils/mutate", () => ({ default: mocks.mutate }));
vi.mock("@/composables/useNotification", () => ({
  default: () => ({ showSuccess: mocks.showSuccess }),
}));
vi.mock("@/graphql/utils/useQuery", async () => {
  const { ref } = await import("vue");
  return { default: () => ({ data: ref(undefined) }) };
});

// jsdom 未实现剪贴板 API，注入测试替身
function stubClipboard() {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: {
      writeText: vi.fn().mockResolvedValue(undefined),
      write: vi.fn().mockResolvedValue(undefined),
    },
  });
  vi.stubGlobal(
    "ClipboardItem",
    class ClipboardItem {
      constructor(public items: Record<string, Blob>) {}
    },
  );
}

beforeEach(() => {
  sessionStorage.clear();
  // 模块级状态基于 sessionStorage，重置模块以获得干净的初始状态
  vi.resetModules();
  mocks.query.mockReset();
  mocks.mutate.mockReset();
  mocks.showSuccess.mockClear();
  stubClipboard();
});

async function importUseClipboard() {
  return await import("@/composables/useClipboard");
}

describe("useClipboard 复制内容标记", () => {
  test("复制增强内容成功时按钩子描述记录文案", async () => {
    mocks.query.mockResolvedValue({
      data: {
        imageCopyContent: {
          content: "{}",
          description: "已复制 ComfyUI 工作流（输出目录已调整）",
        },
      },
    });
    const { useClipboard } = await importUseClipboard();
    const clipboard = useClipboard();

    await clipboard.copyEnhancedOrFile("C:\\root\\a.png", "img-1");

    expect(mocks.showSuccess).toHaveBeenCalledWith("已复制 ComfyUI 工作流（输出目录已调整）");
    expect(clipboard.copiedImageLabels.value["img-1"]).toBe(
      "已复制 ComfyUI 工作流（输出目录已调整）",
    );
  });

  test("无增强内容且服务器支持文件复制时记录已复制图片文件", async () => {
    mocks.query.mockResolvedValue({ data: { imageCopyContent: null } });
    mocks.mutate.mockResolvedValue({ data: { attachFileToClipboard: { supported: true } } });
    const { useClipboard } = await importUseClipboard();
    const clipboard = useClipboard();

    await clipboard.copyEnhancedOrFile("C:\\root\\a.png", "img-1");

    expect(clipboard.copiedImageLabels.value["img-1"]).toBe("已复制图片文件");
  });

  test("服务器不支持文件复制时降级为路径并记录已复制图片路径", async () => {
    mocks.query.mockResolvedValue({ data: { imageCopyContent: null } });
    mocks.mutate.mockResolvedValue({ data: { attachFileToClipboard: { supported: false } } });
    const { useClipboard } = await importUseClipboard();
    const clipboard = useClipboard();

    await clipboard.copyEnhancedOrFile("C:\\root\\a.png", "img-1");

    expect(clipboard.copiedImageLabels.value["img-1"]).toBe("已复制图片路径");
  });

  test("copyFiles 记录传入图片的最后复制文案且未传图片时不记录", async () => {
    mocks.mutate.mockResolvedValue({ data: { attachFileToClipboard: { supported: false } } });
    const { useClipboard } = await importUseClipboard();
    const clipboard = useClipboard();

    await clipboard.copyFiles(["C:\\root\\a.png"], ["img-1"]);

    expect(clipboard.copiedImageLabels.value["img-1"]).toBe("已复制绝对路径");

    await clipboard.copyFiles(["C:\\root\\b.png"]);
    // 未传图片 ID 时不应产生任何记录
    expect(Object.keys(clipboard.copiedImageLabels.value)).toEqual(["img-1"]);
  });

  test("重复复制时覆盖为最新一次的文案", async () => {
    mocks.query.mockResolvedValue({ data: { imageCopyContent: null } });
    mocks.mutate.mockResolvedValue({ data: { attachFileToClipboard: { supported: false } } });
    const { useClipboard } = await importUseClipboard();
    const clipboard = useClipboard();

    await clipboard.copyFiles(["C:\\root\\a.png"], ["img-1"]);
    expect(clipboard.copiedImageLabels.value["img-1"]).toBe("已复制绝对路径");

    mocks.query.mockResolvedValue({
      data: { imageCopyContent: { content: "workflow-json", description: null } },
    });
    await clipboard.copyEnhancedOrFile("C:\\root\\a.png", "img-1");

    expect(clipboard.copiedImageLabels.value["img-1"]).toBe("已复制增强内容");
  });
});
