import { beforeEach, describe, expect, test, vi } from "vitest";
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
});
