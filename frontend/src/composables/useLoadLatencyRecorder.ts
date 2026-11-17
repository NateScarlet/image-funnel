import { computed, ref, toValue, type MaybeRefOrGetter } from "vue";
import Time from "@/utils/Time";
import { byteLength, serializeSegment } from "@/utils/loadLatency/segment";
import type { ActionRecord, SegmentRecord } from "@/utils/loadLatency/types";

/** 单段序列化字节上限，与服务端校验保持一致 */
const MAX_SEGMENT_BYTES = 2 << 20;

export type RecordActionInput = Omit<ActionRecord, "actionSeq" | "at">;

export interface LoadLatencyRecorderDeps {
  /** 服务端下发的阈值；≤0、null、undefined 均表示关闭采集 */
  thresholdMs: MaybeRefOrGetter<number | null | undefined>;
  /** 提交一段，返回写入的绝对路径；由调用方注入 GraphQL mutation */
  submit: (runId: string, segment: string) => Promise<string>;
  /** 页面隐藏时的兜底通道，返回是否成功投递 */
  beacon: (runId: string, segment: string) => boolean;
  /** 页面加载时生成；必须由最外层注入，零值会让 runId 非法 */
  createRunId: () => string;
  now?: () => Time;
}

/**
 * 加载延迟归因记录的增量缓冲。
 *
 * 关闭阈值时所有入口都是空操作；开启后按动作积累，慢切换触发提交，
 * 段与段不重叠、不重传；会话切换丢弃积压；体积超限时丢最老动作并标记缺口。
 */
export default function useLoadLatencyRecorder(deps: LoadLatencyRecorderDeps) {
  const enabled = computed(() => {
    const value = toValue(deps.thresholdMs);
    return typeof value === "number" && value > 0;
  });
  const runId = deps.createRunId();
  const now = deps.now ?? (() => Time.now());

  let actions: ActionRecord[] = [];
  let actionSeq = 0;
  let segmentSeq = 0;
  let gapBefore = false;
  let currentSession: string | undefined;
  const pendingCount = ref(0);

  function draftSegment(): SegmentRecord | undefined {
    if (actions.length === 0) {
      return undefined;
    }
    return {
      runId,
      segmentSeq: segmentSeq + 1,
      startedAt: actions[0].at,
      endedAt: now().toISOString(),
      gapBefore,
      actions,
    };
  }

  function syncPending() {
    pendingCount.value = actions.length;
  }

  function record(input: RecordActionInput) {
    if (!enabled.value) {
      return;
    }
    // 会话切换：旧会话的未提交积压对新会话无意义，直接丢弃
    if (currentSession !== input.sessionId) {
      actions = [];
      gapBefore = false;
      currentSession = input.sessionId;
    }
    actions.push({ ...input, actionSeq: ++actionSeq, at: now().toISOString() });

    // 积压按序列化字节计上限；丢弃最老动作并让下一段显式声明缺口
    let segment = draftSegment();
    while (
      segment &&
      byteLength(serializeSegment(segment)) > MAX_SEGMENT_BYTES &&
      actions.length > 1
    ) {
      actions.shift();
      gapBefore = true;
      segment = draftSegment();
    }
    syncPending();
  }

  /** 丢弃未提交积压，供会话切换时主动调用 */
  function discard() {
    actions = [];
    gapBefore = false;
    currentSession = undefined;
    syncPending();
  }

  // 串行化提交：先同步预留本段，再异步等待结果，避免并发 flush 重复 drafting 同一批动作
  let submission: Promise<unknown> = Promise.resolve();
  async function flushOnce(): Promise<string | undefined> {
    if (!enabled.value) {
      return undefined;
    }
    const segment = draftSegment();
    if (!segment) {
      return undefined;
    }
    const reservedActions = actions;
    const reservedGap = gapBefore;
    const reservedSeq = segmentSeq;
    // 同步预留，使此后的 flush / flushOnHide 看到的是新积压
    actions = [];
    gapBefore = false;
    segmentSeq += 1;
    syncPending();
    try {
      return await deps.submit(runId, serializeSegment(segment));
    } catch (err) {
      // 提交失败则回滚到队首，保留等待期间新记录的动作，供后续重试
      actions = [...reservedActions, ...actions];
      gapBefore = reservedGap || gapBefore;
      segmentSeq = reservedSeq;
      syncPending();
      throw err;
    }
  }

  function flush(): Promise<string | undefined> {
    const task = submission.then(() => flushOnce());
    submission = task.catch(() => undefined);
    return task;
  }

  /** 慢切换触发：超过阈值才提交，反之只留在积压里等后续慢动作一并提交 */
  function maybeFlush(loadMs: number): Promise<string | undefined> {
    const threshold = toValue(deps.thresholdMs);
    if (!enabled.value || typeof threshold !== "number" || loadMs <= threshold) {
      return Promise.resolve(undefined);
    }
    return flush();
  }

  function flushOnHide() {
    if (!enabled.value) {
      return;
    }
    const segment = draftSegment();
    if (!segment) {
      return;
    }
    if (!deps.beacon(runId, serializeSegment(segment))) {
      // 投递失败也要显式留下缺口标记，避免读方把后续段误当连续
      gapBefore = true;
    }
    actions = [];
    segmentSeq += 1;
    syncPending();
  }

  return { runId, enabled, pendingCount, record, discard, flush, maybeFlush, flushOnHide };
}
