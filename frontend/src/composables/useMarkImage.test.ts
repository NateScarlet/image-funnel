import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import Time from "@/utils/Time";
import { ImageAction } from "@/graphql/generated";
import useMarkImage from "./useMarkImage";

const domainMocks = vi.hoisted(() => ({ markImage: vi.fn() }));
vi.mock("./domain/useSession", () => ({
  default: () => ({ markImage: domainMocks.markImage }),
}));

const notificationMocks = vi.hoisted(() => ({ show: vi.fn(() => 0), remove: vi.fn() }));
vi.mock("@/composables/useNotification", () => ({
  default: () => ({ show: notificationMocks.show, remove: notificationMocks.remove }),
}));

beforeEach(() => {
  domainMocks.markImage.mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useMarkImage", () => {
  test("无目标图片时直接丢弃，不发送请求", async () => {
    const { mark, marking } = useMarkImage("session-1", undefined);

    await mark(undefined, ImageAction.REJECT);

    expect(domainMocks.markImage).not.toHaveBeenCalled();
    expect(marking.value).toBe(false);
  });

  test("有目标图片时发送请求并携带时长", async () => {
    const loadedAt = Time.now();
    const { mark, marking } = useMarkImage("session-1", loadedAt);

    await mark("img-1", ImageAction.KEEP);

    expect(domainMocks.markImage).toHaveBeenCalledTimes(1);
    const [imageId, action, duration] = domainMocks.markImage.mock.calls[0];
    expect(imageId).toBe("img-1");
    expect(action).toBe(ImageAction.KEEP);
    expect(duration).toMatch(/^P/);
    expect(marking.value).toBe(false);
  });

  test("标记上下文给出原始间隔与扣除自动排除设定后的加载耗时", async () => {
    const t0 = Time.from("2026-11-16T10:00:00.000Z") ?? Time.now();
    const t1 = t0.add(1000);
    vi.spyOn(Time, "now").mockReturnValueOnce(t0).mockReturnValueOnce(t1);
    const onMark = vi.fn();

    const { mark } = useMarkImage("session-1", t0, {
      autoRejectTimeoutMs: 300,
      onMark,
    });
    await mark("img-1", ImageAction.KEEP);

    expect(onMark).toHaveBeenCalledWith(
      expect.objectContaining({
        imageId: "img-1",
        markIntervalMs: 1000,
        autoRejectTimeoutMs: 300,
        loadMs: 700,
        markAt: t1,
        loadedAt: t0,
      }),
    );
    expect(onMark.mock.calls[0][0].sessionId).toBe("session-1");
  });
});
