import { afterEach, describe, expect, test, vi } from "vitest";
import { ref } from "vue";
import useDisplayedImage from "./useDisplayedImage";
import Time from "@/utils/Time";

afterEach(() => {
  vi.restoreAllMocks();
});

// #region 手势 reaction-time 规则
describe("useDisplayedImage 手势目标", () => {
  test("当当前图片显示时长 ≥ 250ms 时，手势目标为当前图片", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const loadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
      250,
    );

    handleImageLoaded({ id: "img-1", time: loadedAt });

    // 模拟时间流逝 300ms 后触发 recordGestureEnd
    vi.spyOn(Time, "now").mockReturnValue(loadedAt.add(300));
    recordGestureEnd();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("当当前图片显示时长 < 250ms 时，若存在上一张图，手势目标为上一张图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
      250,
    );

    // img-1 加载
    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2 并加载
    currentImageId.value = "img-2";
    const img2LoadedAt = Time.now();
    handleImageLoaded({ id: "img-2", time: img2LoadedAt });

    // 仅过了 100ms（< 250ms 人类反应时间）触发 recordGestureEnd
    vi.spyOn(Time, "now").mockReturnValue(img2LoadedAt.add(100));
    recordGestureEnd();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("即使按下较早，但松开（touchend）时显示时长 ≥ 250ms，手势目标为当前图片", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
      250,
    );

    // img-1 加载
    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2 并加载
    currentImageId.value = "img-2";
    const img2LoadedAt = Time.now();
    handleImageLoaded({ id: "img-2", time: img2LoadedAt });

    // 模拟按住较长时间，在 350ms 时松开（touchend）
    vi.spyOn(Time, "now").mockReturnValue(img2LoadedAt.add(350));
    recordGestureEnd();

    expect(gestureTargetId.value).toBe("img-2");
  });

  test("首张图（无上一张图）且显示时长 < 250ms 时，手势目标回退为当前图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const loadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
      250,
    );

    handleImageLoaded({ id: "img-1", time: loadedAt });

    // 仅过了 50ms，且无上一张图
    vi.spyOn(Time, "now").mockReturnValue(loadedAt.add(50));
    recordGestureEnd();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("当图片未加载完或 ID 不匹配时，若存在已加载的上一张图，手势目标为上一张图", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
    );

    handleImageLoaded({ id: "img-1", time: img1LoadedAt });

    // 切换到 img-2，但 img-2 尚未触发 handleImageLoaded
    currentImageId.value = "img-2";
    recordGestureEnd();

    expect(gestureTargetId.value).toBe("img-1");
  });

  test("撤销（Undo）退回到上一张图后，手势目标正确指向当前图，不受已被撤销图片干扰", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const img1LoadedAt = Time.now();

    const { gestureTargetId, handleImageLoaded, recordGestureEnd } = useDisplayedImage(
      "session-1",
      currentImageId,
    );

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
    recordGestureEnd();

    // 应指向 img-1，而不是被撤销的 img-2
    expect(gestureTargetId.value).toBe("img-1");
  });
});
// #endregion

// #region 会话作用域
describe("useDisplayedImage 会话作用域", () => {
  test("切换到不同目录的会话后，新会话首图加载完成前目标图为空", () => {
    const sessionId = ref("session-a");
    const currentImageId = ref<string | null | undefined>("img-a");
    const { displayedId, loadedAt, gestureTargetId, handleImageLoaded, recordGestureEnd } =
      useDisplayedImage(sessionId, currentImageId);

    handleImageLoaded({ id: "img-a", time: Time.now() });
    expect(displayedId.value).toBe("img-a");
    expect(loadedAt.value).toBeDefined();

    // 提交后进入不同目录的下一会话：会话数据已就位，首图仍在加载
    sessionId.value = "session-b";
    currentImageId.value = "img-b";

    expect(displayedId.value).toBeUndefined();
    expect(loadedAt.value).toBeUndefined();

    recordGestureEnd();
    expect(gestureTargetId.value).toBeUndefined();
  });

  test("切换到同一目录的另一个会话后，即使图片 ID 相同也不继承", () => {
    const sessionId = ref("session-a");
    const currentImageId = ref<string | null | undefined>("img-1");
    const { displayedId, loadedAt, gestureTargetId, handleImageLoaded, recordGestureEnd } =
      useDisplayedImage(sessionId, currentImageId);

    handleImageLoaded({ id: "img-1", time: Time.now() });
    expect(displayedId.value).toBe("img-1");

    // 同一目录的新会话，首图恰好是同一张图片，但尚未触发加载事件
    sessionId.value = "session-b";

    expect(displayedId.value).toBeUndefined();
    expect(loadedAt.value).toBeUndefined();

    recordGestureEnd();
    expect(gestureTargetId.value).toBeUndefined();
  });

  test("新会话首图加载完成后恢复目标图", () => {
    const sessionId = ref("session-a");
    const currentImageId = ref<string | null | undefined>("img-a");
    const { displayedId, loadedAt, gestureTargetId, handleImageLoaded, recordGestureEnd } =
      useDisplayedImage(sessionId, currentImageId);

    handleImageLoaded({ id: "img-a", time: Time.now() });

    sessionId.value = "session-b";
    currentImageId.value = "img-b";
    expect(displayedId.value).toBeUndefined();

    const imgBLoadedAt = Time.now();
    handleImageLoaded({ id: "img-b", time: imgBLoadedAt });

    expect(displayedId.value).toBe("img-b");
    expect(loadedAt.value).toBe(imgBLoadedAt);

    // 新会话的首图没有上一张图，reaction-time 内仍以它为手势目标
    vi.spyOn(Time, "now").mockReturnValue(imgBLoadedAt.add(50));
    recordGestureEnd();
    expect(gestureTargetId.value).toBe("img-b");
  });

  test("没有任何加载事件时不回退到队列当前图", () => {
    const sessionId = ref("session-a");
    const currentImageId = ref<string | null | undefined>("img-a");
    const { displayedId, loadedAt, gestureTargetId, recordGestureEnd } = useDisplayedImage(
      sessionId,
      currentImageId,
    );

    expect(displayedId.value).toBeUndefined();
    expect(loadedAt.value).toBeUndefined();

    recordGestureEnd();
    expect(gestureTargetId.value).toBeUndefined();
  });
});
// #endregion

// #region 会话内切换图片
describe("useDisplayedImage 会话内切换图片", () => {
  test("新图未加载完成时目标仍为上一张已加载图片", () => {
    const sessionId = "session-1";
    const currentImageId = ref<string | null | undefined>("img-1");
    const { displayedId, loadedAt, gestureTargetId, handleImageLoaded, recordGestureEnd } =
      useDisplayedImage(sessionId, currentImageId);

    handleImageLoaded({ id: "img-1", time: Time.now() });

    // 队列推进到 img-2，但 img-2 尚未加载完成
    currentImageId.value = "img-2";

    expect(displayedId.value).toBe("img-1");
    expect(loadedAt.value).toBeUndefined();

    recordGestureEnd();
    expect(gestureTargetId.value).toBe("img-1");
  });

  test("loadedAt 仅在加载事件属于队列当前图时给出", () => {
    const currentImageId = ref<string | null | undefined>("img-1");
    const { loadedAt, handleImageLoaded } = useDisplayedImage("session-1", currentImageId);

    expect(loadedAt.value).toBeUndefined();

    const img1LoadedAt = Time.now();
    handleImageLoaded({ id: "img-1", time: img1LoadedAt });
    expect(loadedAt.value).toBe(img1LoadedAt);

    // 队列推进后，上一张图的加载时间不再代表当前图的加载完成
    currentImageId.value = "img-2";
    expect(loadedAt.value).toBeUndefined();

    const img2LoadedAt = Time.now();
    handleImageLoaded({ id: "img-2", time: img2LoadedAt });
    expect(loadedAt.value).toBe(img2LoadedAt);
  });
});
// #endregion
