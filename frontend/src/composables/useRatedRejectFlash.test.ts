import { describe, test, expect } from "vitest";
import { ref } from "vue";
import useRatedRejectFlash from "./useRatedRejectFlash";
import { ImageAction } from "@/graphql/generated";

describe("useRatedRejectFlash", () => {
  test("REJECT + 评分>0 时生成一次含正确评分与递增序号的信号", () => {
    const { signal, flash } = useRatedRejectFlash("session-a");

    flash(ImageAction.REJECT, 3);
    expect(signal.value).toEqual({ seq: 1, rating: 3 });

    flash(ImageAction.REJECT, 3);
    expect(signal.value).toEqual({ seq: 2, rating: 3 });
  });

  test("REJECT + 评分=0 时不生成信号", () => {
    const { signal, flash } = useRatedRejectFlash("session-a");

    flash(ImageAction.REJECT, 0);
    expect(signal.value).toBeUndefined();
  });

  test("非 REJECT 动作（KEEP/SHELVE）不生成信号", () => {
    const { signal, flash } = useRatedRejectFlash("session-a");

    flash(ImageAction.KEEP, 3);
    flash(ImageAction.SHELVE, 3);
    expect(signal.value).toBeUndefined();
  });

  test("连续触发已评分 REJECT 时序号严格递增（含同评分重播）", () => {
    const { signal, flash } = useRatedRejectFlash("session-a");

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 1, rating: 5 });

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 2, rating: 5 });

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 3, rating: 5 });
  });

  test("未触发动作混入已触发动作时序号不因未触发而错乱", () => {
    const { signal, flash } = useRatedRejectFlash("session-a");

    flash(ImageAction.REJECT, 4);
    expect(signal.value?.seq).toBe(1);

    // 中间是无评分或非 REJECT，不应改变 seq
    flash(ImageAction.REJECT, 0);
    flash(ImageAction.KEEP, 4);
    expect(signal.value?.seq).toBe(1);

    // 之后再次有效触发，seq 继续严格递增
    flash(ImageAction.REJECT, 4);
    expect(signal.value?.seq).toBe(2);
  });

  test("会话切换后残留信号不可见，新会话触发时生成新信号", () => {
    const sessionId = ref("session-a");
    const { signal, flash } = useRatedRejectFlash(sessionId);

    // 旧会话中触发动画（模拟会话最后一张图被排除，动画被中断前信号残留）
    flash(ImageAction.REJECT, 3);
    expect(signal.value).toEqual({ seq: 1, rating: 3 });

    // 提交后切换到下一目录会话（SessionView 实例复用，仅 sessionId 变化）
    sessionId.value = "session-b";
    // 残留信号不得在新会话中可见（防止重放动画）
    expect(signal.value).toBeUndefined();

    // 新会话中真正执行排除操作时正常触发
    flash(ImageAction.REJECT, 2);
    expect(signal.value).toEqual({ seq: 2, rating: 2 });
  });

  test("clear 清除信号", () => {
    const { signal, flash, clear } = useRatedRejectFlash("session-a");

    flash(ImageAction.REJECT, 3);
    expect(signal.value).toBeDefined();

    clear();
    expect(signal.value).toBeUndefined();
  });
});
