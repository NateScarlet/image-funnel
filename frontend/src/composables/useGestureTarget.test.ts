import { describe, test, expect, vi } from "vitest";
import { ref } from "vue";
import useGestureTarget from "./useGestureTarget";
import Time from "@/utils/Time";

describe("useGestureTarget", () => {
  test("当当前图片显示时长 ≥ 250ms 时，手势目标为当前图片", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const loadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureStart } = useGestureTarget(
      currentImageId,
      250,
    );

    handleImageLoaded({ id: "img-1", time: loadedAt });

    // 模拟时间流逝 300ms 后触发 recordGestureStart
    vi.spyOn(Time, "now").mockReturnValue(loadedAt.add(300));
    recordGestureStart();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("当当前图片显示时长 < 250ms 时，若存在上一张图，手势目标为上一张图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureStart } = useGestureTarget(
      currentImageId,
      250,
    );

    // img-1 加载
    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2 并加载
    currentImageId.value = "img-2";
    const img2LoadedAt = Time.now();
    handleImageLoaded({ id: "img-2", time: img2LoadedAt });

    // 仅过了 100ms（< 250ms 人类反应时间）触发 recordGestureStart
    vi.spyOn(Time, "now").mockReturnValue(img2LoadedAt.add(100));
    recordGestureStart();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("首张图（无上一张图）且显示时长 < 250ms 时，手势目标回退为当前图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const loadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureStart } = useGestureTarget(
      currentImageId,
      250,
    );

    handleImageLoaded({ id: "img-1", time: loadedAt });

    // 仅过了 50ms，且无上一张图
    vi.spyOn(Time, "now").mockReturnValue(loadedAt.add(50));
    recordGestureStart();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("当图片未加载完或 ID 不匹配时，若存在上一张图，手势目标为上一张图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureStart } =
      useGestureTarget(currentImageId);

    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2，但 img-2 尚未触发 handleImageLoaded
    currentImageId.value = "img-2";
    recordGestureStart();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("撤销（Undo）退回到上一张图后，手势目标正确指向当前图，不受已被撤销图片干扰", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureStart } =
      useGestureTarget(currentImageId);

    // img-1 加载
    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2 并加载
    currentImageId.value = "img-2";
    const img2LoadedAt = Time.now();
    handleImageLoaded({ id: "img-2", time: img2LoadedAt });

    // 撤销退回到 img-1 并重新加载
    currentImageId.value = "img-1";
    const img1ReLoadedAt = Time.now();
    handleImageLoaded({ id: "img-1", time: img1ReLoadedAt });

    // 退回到 img-1 后不足 250ms 内手势
    vi.spyOn(Time, "now").mockReturnValue(img1ReLoadedAt.add(100));
    recordGestureStart();

    // 应指向 img-1，而不是被撤销的 img-2
    expect(gestureTargetId.value).toBe("img-1");
  });
});
