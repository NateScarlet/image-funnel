import { describe, expect, it, vi } from "vitest";
import Time from "@/utils/Time";
import useLoadLatencyRecorder, { type RecordActionInput } from "./useLoadLatencyRecorder";
import { byteLength } from "@/utils/loadLatency/segment";
import type { SegmentRecord } from "@/utils/loadLatency/types";

const RUN_ID = "0123456789abcdef0123456789abcdef";
const SESSION = "sess:1";

function input(overrides: Partial<RecordActionInput> = {}): RecordActionInput {
  return {
    sessionId: SESSION,
    round: 1,
    imageId: "img:1",
    dpr: 2,
    viewportWidth: 390,
    viewportHeight: 844,
    autoRejectTimeoutMs: 3000,
    markIntervalMs: 1200,
    loadMs: 900,
    loadedAt: "2026-11-16T10:00:00.000Z",
    markAt: "2026-11-16T10:00:01.000Z",
    requests: [],
    ...overrides,
  };
}

function setup(options: { threshold?: number | null; beaconResult?: boolean } = {}) {
  const submit = vi.fn(async (_runId: string, _segment: string) => "C:/data/load-latency/x.jsonl");
  const beacon = vi.fn(() => options.beaconResult ?? true);
  let tick = 0;
  const clock = () => {
    tick += 1;
    return Time.from(`2026-11-16T10:00:0${tick}.000Z`) ?? Time.now();
  };
  const recorder = useLoadLatencyRecorder({
    thresholdMs: () => ("threshold" in options ? options.threshold : 500),
    submit,
    beacon,
    now: clock,
    createRunId: () => RUN_ID,
  });
  return { recorder, submit, beacon };
}

function parseSegment(submit: ReturnType<typeof vi.fn>, call = 0): SegmentRecord {
  const text = submit.mock.calls[call][1] as string;
  return JSON.parse(text) as SegmentRecord;
}

describe("useLoadLatencyRecorder", () => {
  it("未设置阈值时完全不采集，也不产生写请求", async () => {
    const { recorder, submit } = setup({ threshold: null });
    recorder.record(input());
    await recorder.flush();
    expect(submit).not.toHaveBeenCalled();
    expect(recorder.pendingCount.value).toBe(0);
  });

  it("慢切换提交一段，覆盖从会话开始到该次切换的全部动作（含中间不慢的动作）", async () => {
    const { recorder, submit } = setup({ threshold: 500 });
    recorder.record(input({ imageId: "img:1", loadMs: 120 }));
    recorder.record(input({ imageId: "img:2", loadMs: 90 }));
    recorder.record(input({ imageId: "img:3", loadMs: 900 }));

    await recorder.flush();

    expect(submit).toHaveBeenCalledTimes(1);
    expect(submit.mock.calls[0][0]).toBe(RUN_ID);
    const segment = parseSegment(submit);
    expect(segment.segmentSeq).toBe(1);
    expect(segment.gapBefore).toBe(false);
    expect(segment.actions.map((a) => a.imageId)).toEqual(["img:1", "img:2", "img:3"]);
    expect(recorder.pendingCount.value).toBe(0);
  });

  it("后续慢切换追加新段，动作序号连续且无重复", async () => {
    const { recorder, submit } = setup({ threshold: 500 });
    recorder.record(input({ imageId: "img:1" }));
    await recorder.flush();
    recorder.record(input({ imageId: "img:2" }));
    await recorder.flush();

    const first = parseSegment(submit, 0);
    const second = parseSegment(submit, 1);
    expect(second.segmentSeq).toBe(2);
    expect(second.actions).toHaveLength(1);
    expect(second.actions[0].actionSeq).toBe(first.actions[0].actionSeq + 1);
  });

  it("积压超限时丢弃最老动作并标记 gapBefore", async () => {
    const { recorder, submit } = setup({ threshold: 500 });
    // 用大体积请求把积压顶过 2MB，但每个单动作仍小于上限
    const big = "x".repeat(1024 * 1024);
    const bigRequest = {
      urlShape: "other" as const,
      startedAt: big,
      ttfbMs: 0,
      downloadMs: 0,
      decodeMs: 0,
      bytes: 0,
      retrieval: "unknown" as const,
      serverTiming: [],
    };
    recorder.record(input({ imageId: "img:1", requests: [bigRequest] }));
    recorder.record(input({ imageId: "img:2", requests: [bigRequest] }));
    recorder.record(input({ imageId: "img:3" }));

    await recorder.flush();
    const segment = parseSegment(submit);
    expect(segment.gapBefore).toBe(true);
    expect(segment.actions.every((a) => a.imageId !== "img:1")).toBe(true);
    expect(byteLength(JSON.stringify(segment))).toBeLessThanOrEqual(2 << 20);
  });

  it("会话切换丢弃未提交积压", async () => {
    const { recorder, submit } = setup({ threshold: 500 });
    recorder.record(input({ sessionId: "sess:1", imageId: "img:1" }));
    recorder.record(input({ sessionId: "sess:2", imageId: "img:2" }));
    await recorder.flush();

    const segment = parseSegment(submit);
    expect(segment.actions.map((a) => a.imageId)).toEqual(["img:2"]);
  });

  it("并发 flush 只提交一次，段不重叠也不重传", async () => {
    const { recorder, submit } = setup({ threshold: 500 });
    recorder.record(input({ imageId: "img:1" }));

    await Promise.all([recorder.flush(), recorder.flush()]);

    expect(submit).toHaveBeenCalledTimes(1);
    expect(parseSegment(submit).actions).toHaveLength(1);
  });

  it("会话切换可主动丢弃积压，避免 pagehide 提交旧会话数据", async () => {
    const { recorder, submit, beacon } = setup({ threshold: 500 });
    recorder.record(input({ sessionId: "s1", imageId: "img:1" }));

    recorder.discard();
    recorder.flushOnHide();
    await recorder.flush();

    expect(beacon).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
    expect(recorder.pendingCount.value).toBe(0);
  });

  it("页面隐藏时把积压交给 beacon 兜底，失败则标记缺口而非静默消失", async () => {
    const ok = setup({ threshold: 500, beaconResult: true });
    ok.recorder.record(input());
    ok.recorder.flushOnHide();
    expect(ok.beacon).toHaveBeenCalledTimes(1);
    expect(ok.recorder.pendingCount.value).toBe(0);

    const failed = setup({ threshold: 500, beaconResult: false });
    failed.recorder.record(input());
    failed.recorder.flushOnHide();
    failed.recorder.record(input());
    await failed.recorder.flush();
    expect(parseSegment(failed.submit).gapBefore).toBe(true);
  });
});
