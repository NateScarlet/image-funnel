import type { RetrievalMethod } from "./retrieval";
import type { ServerTimingMetric } from "./serverTiming";

/** 单次图片请求的可观测记录 */
export interface RequestRecord {
  urlShape: "raw" | "width" | "other";
  width?: number;
  startedAt: string;
  ttfbMs: number;
  downloadMs: number;
  decodeMs: number;
  bytes: number;
  retrieval: RetrievalMethod;
  serverTiming: ServerTimingMetric[];
}

/** 一次标记动作及其加载上下文 */
export interface ActionRecord {
  actionSeq: number;
  at: string;
  sessionId: string;
  round: number;
  imageId: string;
  dpr: number;
  viewportWidth: number;
  viewportHeight: number;
  autoRejectTimeoutMs: number;
  markIntervalMs: number;
  loadMs: number;
  loadedAt: string;
  markAt: string;
  requests: RequestRecord[];
}

/** 一段增量：同一 run 内按 segmentSeq 递增，gapBefore 标出其前有动作被丢弃 */
export interface SegmentRecord {
  runId: string;
  segmentSeq: number;
  startedAt: string;
  endedAt: string;
  gapBefore: boolean;
  actions: ActionRecord[];
}
