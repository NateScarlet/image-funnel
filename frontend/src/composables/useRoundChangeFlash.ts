import { onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from "vue";

/** 换轮提示的时长（毫秒）：淡入 200 + 保持 600 + 淡出 200，总计约 1 秒 */
export const ROUND_FLASH_FADE_IN_MS = 200;
export const ROUND_FLASH_HOLD_MS = 600;
export const ROUND_FLASH_FADE_OUT_MS = 200;

/** 变色阶段（idle 之外）：与视图样式中的 data-round-flash 选择器一一对应 */
export const ROUND_FLASH_TINT_PHASES = ["fade-in", "hold", "fade-out"] as const;

/** 换轮提示的视觉阶段，由会话根元素上的 data-round-flash 消费 */
export type RoundFlashPhase = "idle" | (typeof ROUND_FLASH_TINT_PHASES)[number];

/** 上一次观测到的轮次，必须带上会话作用域才能区分"换轮"与"换会话" */
interface RoundObservation {
  sessionId: string;
  round: number;
}

/**
 * 换轮背景提示。
 *
 * 仅当同一会话内观测到的 `currentRound` 相对上一次观测值增大时才准备提示，
 * 并且要等下一轮首图真正加载完成（`handleImageLoaded`）才播放，
 * 否则换轮瞬间的图片加载会把提示吃掉。
 *
 * - 首次观测（进入会话、恢复已进行到第 N 轮的会话）只建立基线，不触发；
 * - 撤销导致轮次减小时作废尚未播放的信号，也不触发；
 * - 会话切换（含提交后自动进入下一目录的新会话）会清空基线与进行中的提示；
 * - 提示期间轮次再次增大时，从最新时刻重新计时，不叠加计时器。
 *
 * @param sessionId 会话作用域标识，必须与 `currentRound` 取自同一份会话数据
 * @param currentRound 会话的当前轮次，会话数据未就位时为空
 */
export default function useRoundChangeFlash(
  sessionId: MaybeRefOrGetter<string | undefined>,
  currentRound: MaybeRefOrGetter<number | null | undefined>,
) {
  const phase = ref<RoundFlashPhase>("idle");
  // 轮次已增大、但还在等下一轮首图加载完成的会话
  let pendingSessionId: string | undefined;
  let lastObserved: RoundObservation | undefined;
  const timers: number[] = [];

  function clearTimers() {
    for (const timer of timers) {
      clearTimeout(timer);
    }
    timers.length = 0;
  }

  function schedule(delayMs: number, run: () => void) {
    timers.push(window.setTimeout(run, delayMs));
  }

  // #region 视觉时序
  function start() {
    // 重新触发时清掉上一轮计时：提示保持变色状态，并自最新时刻重新计时
    clearTimers();
    phase.value = "fade-in";
    schedule(ROUND_FLASH_FADE_IN_MS, () => {
      phase.value = "hold";
      schedule(ROUND_FLASH_HOLD_MS, () => {
        phase.value = "fade-out";
        schedule(ROUND_FLASH_FADE_OUT_MS, () => {
          phase.value = "idle";
        });
      });
    });
  }
  // #endregion

  // #region 轮次观测
  watch(
    () => [toValue(sessionId), toValue(currentRound)] as const,
    ([scope, round]) => {
      if (scope === undefined || round === undefined || round === null) {
        // 会话数据尚未就位：丢弃基线与待播放信号，避免把恢复会话的首个观测当成换轮
        lastObserved = undefined;
        pendingSessionId = undefined;
        clearTimers();
        phase.value = "idle";
        return;
      }

      const previous = lastObserved;
      if (previous === undefined || previous.sessionId !== scope) {
        // 首次观测或会话切换：只建立基线，不播放
        lastObserved = { sessionId: scope, round };
        pendingSessionId = undefined;
        clearTimers();
        phase.value = "idle";
        return;
      }

      if (round === previous.round) {
        return;
      }

      lastObserved = { sessionId: scope, round };

      if (round < previous.round) {
        // 撤销回到上一轮：不提示，同时作废仍在等待首图加载的换轮信号
        pendingSessionId = undefined;
        return;
      }

      // 轮次增大：等下一轮首图加载完成后再播放
      pendingSessionId = scope;
    },
    { immediate: true, flush: "sync" },
  );
  // #endregion

  // #region 首图加载回调
  function handleImageLoaded() {
    const scope = toValue(sessionId);
    // 待播放信号按会话隔离，避免换会话后由新会话的首图加载触发
    if (scope === undefined || pendingSessionId !== scope) {
      return;
    }
    pendingSessionId = undefined;
    start();
  }
  // #endregion

  onScopeDispose(clearTimers);

  return { phase, handleImageLoaded };
}
