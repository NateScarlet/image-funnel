import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { effectScope, ref, type EffectScope, type MaybeRefOrGetter } from "vue";
import useRoundChangeFlash, {
  ROUND_FLASH_FADE_IN_MS,
  ROUND_FLASH_FADE_OUT_MS,
  ROUND_FLASH_HOLD_MS,
  ROUND_FLASH_TINT_PHASES,
} from "./useRoundChangeFlash";
// 会话视图是本功能唯一的样式消费者：阶段名与时长必须与它保持同一契约
import sessionViewSource from "../views/SessionView.vue?raw";

// 每个用例的 composable 都挂在独立 scope 上，用例结束后统一销毁，
// 避免残留的计时器影响后续用例
const scopes: EffectScope[] = [];

function setup(
  sessionId: MaybeRefOrGetter<string | undefined>,
  currentRound: MaybeRefOrGetter<number | undefined>,
) {
  const scope = effectScope();
  const api = scope.run(() => useRoundChangeFlash(sessionId, currentRound));
  if (!api) {
    throw new Error("effectScope 未能创建 composable");
  }
  scopes.push(scope);
  return api;
}

// 推进完整一轮提示：淡入 200ms / 保持 600ms / 淡出 200ms
function advanceFullCycle() {
  vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS);
  vi.advanceTimersByTime(ROUND_FLASH_HOLD_MS);
  vi.advanceTimersByTime(ROUND_FLASH_FADE_OUT_MS);
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  for (const scope of scopes.splice(0)) {
    scope.stop();
  }
  vi.useRealTimers();
});

describe("useRoundChangeFlash 触发条件", () => {
  test("首次观测（含恢复已进行到第 N 轮的会话）只建立基线，图片加载后也不播放", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(4);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    expect(phase.value).toBe("idle");

    handleImageLoaded();
    expect(phase.value).toBe("idle");
  });

  test("会话数据未就位时不建立基线，数据到达后首次观测同样不播放", () => {
    const sessionId = ref<string | undefined>();
    const currentRound = ref<number | undefined>();
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    // 会话加载完成：同一时刻观测到会话 ID 与第 N 轮
    sessionId.value = "session-a";
    currentRound.value = 5;

    handleImageLoaded();
    expect(phase.value).toBe("idle");
  });

  test("轮次增大后不会立刻播放，要等图片真正加载完成", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    expect(phase.value).toBe("idle");

    handleImageLoaded();
    expect(phase.value).toBe("fade-in");
  });

  test("轮次增大且图片已加载完成后按 200/600/200ms 时序推进并归零", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");

    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS);
    expect(phase.value).toBe("hold");

    vi.advanceTimersByTime(ROUND_FLASH_HOLD_MS);
    expect(phase.value).toBe("fade-out");

    vi.advanceTimersByTime(ROUND_FLASH_FADE_OUT_MS);
    expect(phase.value).toBe("idle");
  });

  test("轮次未变化时图片加载不会播放提示", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(3);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    handleImageLoaded();
    handleImageLoaded();

    expect(phase.value).toBe("idle");
  });
});

describe("useRoundChangeFlash 会话隔离与撤销", () => {
  test("轮次增大产生的待播放信号属于原会话，会话切换后不播放", () => {
    // 会话 ID 与轮次取自同一份会话数据，切换会话时二者原子更新
    const session = ref<{ id: string; round: number } | undefined>({
      id: "session-a",
      round: 1,
    });
    const { phase, handleImageLoaded } = setup(
      () => session.value?.id,
      () => session.value?.round,
    );

    // 旧会话换轮后尚未等到首图加载，就已提交并进入下一目录的新会话
    session.value = { id: "session-a", round: 2 };
    session.value = { id: "session-b", round: 5 };

    handleImageLoaded();
    expect(phase.value).toBe("idle");
  });

  test("会话切换会终止进行中的提示，新会话首图加载不重放", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");

    sessionId.value = "session-b";
    currentRound.value = 0;
    expect(phase.value).toBe("idle");

    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS + ROUND_FLASH_HOLD_MS);
    expect(phase.value).toBe("idle");
  });

  test("撤销导致轮次减小时不播放，之后的轮次增大仍正常播放", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(2);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    // 撤销回到上一轮：轮次减小
    currentRound.value = 1;
    handleImageLoaded();
    expect(phase.value).toBe("idle");

    // 再次完成该轮、进入下一轮时正常播放
    currentRound.value = 2;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");
  });

  test("轮次减小时作废尚未播放的换轮信号", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    // 换轮后、首图加载完成前撤销回到上一轮
    currentRound.value = 2;
    currentRound.value = 1;

    handleImageLoaded();
    expect(phase.value).toBe("idle");
  });
});

describe("useRoundChangeFlash 提示期间行为", () => {
  test("提示期间再次换轮时从最新时刻重新计时，不叠加也不提前结束", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");

    // 处于保持阶段时再次换轮并加载首图
    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS + 100);
    expect(phase.value).toBe("hold");

    currentRound.value = 3;
    handleImageLoaded();
    // 保持变色状态重新开始计时（in/hold 均为变色状态，视觉上不中断）
    expect(phase.value).toBe("fade-in");

    // 旧一轮的计时不得提前结束提示
    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS);
    expect(phase.value).toBe("hold");
    vi.advanceTimersByTime(ROUND_FLASH_HOLD_MS);
    expect(phase.value).toBe("fade-out");
    vi.advanceTimersByTime(ROUND_FLASH_FADE_OUT_MS);
    expect(phase.value).toBe("idle");
  });

  test("提示期间加载同轮其它图片不会重新计时或中断", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS + 100);
    expect(phase.value).toBe("hold");

    // 同一轮内的后续图片加载（手动标记或自动排除后切换）
    handleImageLoaded();
    handleImageLoaded();
    expect(phase.value).toBe("hold");

    vi.advanceTimersByTime(ROUND_FLASH_HOLD_MS + ROUND_FLASH_FADE_OUT_MS - 100);
    expect(phase.value).toBe("idle");
  });

  test("淡出阶段再次换轮时平滑重新计时", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS + ROUND_FLASH_HOLD_MS + 100);
    expect(phase.value).toBe("fade-out");

    currentRound.value = 3;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");

    advanceFullCycle();
    expect(phase.value).toBe("idle");
  });

  test("销毁作用域后不再推进计时器", () => {
    const sessionId = ref<string | undefined>("session-a");
    const currentRound = ref<number | undefined>(1);
    const { phase, handleImageLoaded } = setup(sessionId, currentRound);

    currentRound.value = 2;
    handleImageLoaded();
    expect(phase.value).toBe("fade-in");

    for (const scope of scopes.splice(0)) {
      scope.stop();
    }

    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS + ROUND_FLASH_HOLD_MS);
    expect(phase.value).toBe("fade-in");
  });
});

describe("useRoundChangeFlash 与会话视图样式的契约", () => {
  test("每个变色阶段都有对应的 data-round-flash 选择器，且没有多余字面量", () => {
    // 只匹配样式里的属性选择器；模板中的 :data-round-flash="roundFlashPhase" 是动态绑定
    const selectorPhases = [...sessionViewSource.matchAll(/\[data-round-flash="([^"]+)"\]/g)].map(
      (match) => match[1],
    );

    expect(selectorPhases.toSorted()).toEqual([...ROUND_FLASH_TINT_PHASES].toSorted());
  });

  test("样式中的过渡时长与淡入、淡出常量一致", () => {
    // 淡入与淡出共用一个 transition 时长，二者必须相等才能只声明一份
    expect(ROUND_FLASH_FADE_IN_MS).toBe(ROUND_FLASH_FADE_OUT_MS);

    const transition = sessionViewSource.match(/transition: --round-flash-opacity (\d+)ms/);
    expect(transition).not.toBeNull();
    expect(Number(transition?.[1])).toBe(ROUND_FLASH_FADE_IN_MS);
  });
});
