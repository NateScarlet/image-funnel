import { describe, test, expect } from "vitest";
import useRatedRejectFlash from "./useRatedRejectFlash";
import { ImageAction } from "@/graphql/generated";

describe("useRatedRejectFlash", () => {
  test("REJECT + 评分>0 时生成一次含正确评分与递增序号的信号", () => {
    const { signal, flash } = useRatedRejectFlash();

    flash(ImageAction.REJECT, 3);
    expect(signal.value).toEqual({ seq: 1, rating: 3 });

    flash(ImageAction.REJECT, 3);
    expect(signal.value).toEqual({ seq: 2, rating: 3 });
  });

  test("REJECT + 评分=0 时不生成信号", () => {
    const { signal, flash } = useRatedRejectFlash();

    flash(ImageAction.REJECT, 0);
    expect(signal.value).toBeUndefined();
  });

  test("非 REJECT 动作（KEEP/SHELVE）不生成信号", () => {
    const { signal, flash } = useRatedRejectFlash();

    flash(ImageAction.KEEP, 3);
    flash(ImageAction.SHELVE, 3);
    expect(signal.value).toBeUndefined();
  });

  test("连续触发已评分 REJECT 时序号严格递增（含同评分重播）", () => {
    const { signal, flash } = useRatedRejectFlash();

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 1, rating: 5 });

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 2, rating: 5 });

    flash(ImageAction.REJECT, 5);
    expect(signal.value).toEqual({ seq: 3, rating: 5 });
  });

  test("未触发动作混入已触发动作时序号不因未触发而错乱", () => {
    const { signal, flash } = useRatedRejectFlash();

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
});
