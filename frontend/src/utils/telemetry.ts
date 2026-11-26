import * as Sentry from "@sentry/vue";
import type { App } from "vue";
import { CombinedGraphQLErrors, LinkError } from "@apollo/client/errors";
import { isDirectoryPageWaitCancelled } from "@/utils/directoryPageWaitCancelled";
import type { SessionFragment } from "@/graphql/generated";
import { newSalt, scrubEvent } from "./telemetryScrub";

// #region 埋点契约
/** 只上报需要开发者处理的步骤；其余步骤只写面包屑 */
export const REPORTABLE_STEPS = [
  "navigation-not-switched",
  "navigation-flicker",
  "error-classified",
] as const;
type ReportableStep = (typeof REPORTABLE_STEPS)[number];

export type NavigationStep =
  | "duplicate-skipped"
  | "delay-skipped"
  | "navigation-started"
  | "decision-started"
  | "decision-resolved"
  | "decision-failed"
  | "pagination-fetched"
  | "pagination-stalled"
  | "stats-refreshed"
  | "stats-refresh-failed"
  | "session-created"
  | "session-create-failed"
  | "navigated-to-session"
  | "navigated-home"
  | ReportableStep;

export type NavigationErrorClass = "cancelled" | "graphql" | "link" | "local";
export type DecisionBranch = "immediate" | "paged" | "exhausted" | "not-started";

export interface NavigationCursorContext {
  after?: string;
  sortKey?: string;
  query?: string;
}

export interface NavigationStatsEntry {
  id: string;
  isCompleted: boolean;
  imageCount?: number;
  subdirectoryCount?: number;
}

export interface NavigationStatsSnapshot {
  totalCount: number;
  totalKept: number;
  totalShelved: number;
  totalRejected: number;
  currentRoundRemaining: number;
  isCompleted: boolean;
}

/** 提交后导航链路的一次埋点。所有上下文字段都是可选的，由各 seam 填充自己掌握的部分 */
export interface NavigationEvent {
  step: NavigationStep;
  sessionId?: string;
  directoryId?: string;
  directoryRelPath?: string;
  nextDirectoryId?: string;
  cursor?: NavigationCursorContext;
  statsSnapshot?: NavigationStatsSnapshot;
  stats?: NavigationStatsEntry[];
  decisionBranch?: DecisionBranch;
  errorClass?: NavigationErrorClass;
  message?: string;
  loadedCount?: number;
  hasNextPage?: boolean;
}

export type CaptureNavigationEvent = (event: NavigationEvent) => void;

/** 下游决策 composable 暴露给编排 composable 的现场快照，失败事件也要带上 */
export type NavigationContextSnapshot = Pick<
  NavigationEvent,
  "cursor" | "stats" | "decisionBranch"
>;

/** 从当前会话组装会话级上下文；所有埋点出口共用一份，避免各 seam 拼出不同形状 */
export function sessionNavigationContext(session: SessionFragment): Partial<NavigationEvent> {
  const { stats } = session;
  return {
    sessionId: session.id,
    directoryId: session.directory.id,
    directoryRelPath: session.directory.relPath,
    statsSnapshot: stats
      ? {
          totalCount: stats.totalCount,
          totalKept: stats.totalKept,
          totalShelved: stats.totalShelved,
          totalRejected: stats.totalRejected,
          currentRoundRemaining: stats.currentRoundRemaining,
          isCompleted: stats.isCompleted,
        }
      : undefined,
  };
}

/** 失败分类在两处埋点共用，避免同一错误在不同 seam 被归为不同类 */
export function classifyNavigationError(error: unknown): NavigationErrorClass {
  if (isDirectoryPageWaitCancelled(error)) return "cancelled";
  if (CombinedGraphQLErrors.is(error)) return "graphql";
  if (LinkError.is(error)) return "link";
  return "local";
}

export interface MarkPerformanceEvent {
  sessionId: string;
  round: number;
  imageId: string;
  loadMs: number;
  markIntervalMs: number;
  autoRejectTimeoutMs: number;
  dpr: number;
  viewportWidth: number;
  viewportHeight: number;
}
// #endregion

/**
 * Apollo Client 取消在途 query 时，AbortError 会以未处理 rejection 被全局 handler 捕获。
 * 这是预期的取消信号而非需要开发者处理的错误，发送前直接丢弃；
 * 同时限定 mechanism 与文案，避免误伤显式上报的其他 AbortError。
 */
function isCancellationNoise(event: {
  exception?: {
    values?: { type?: string; value?: string; mechanism?: { type?: string } }[];
  };
}): boolean {
  const value = event.exception?.values?.[0];
  return (
    value?.type === "AbortError" &&
    value.value === "The operation was aborted." &&
    value.mechanism?.type === "auto.browser.global_handlers.onunhandledrejection"
  );
}

/**
 * 从 meta 下发的 DSN 初始化 SDK。空串表示关闭：不调用 init，后续 capture 自动成为空操作。
 * 客户端不感知自托管模式，始终使用 SDK 默认 transport 直发 DSN 指定的服务。
 */
export function initTelemetry(dsn: string, app: App): void {
  if (!dsn) return;
  // 脱敏在发送前完成：原始 ID/路径只存在于客户端内存，不会进入 envelope。
  // release 用构建版本号，避免把已修复旧版本的同类问题误判为当前回归。
  Sentry.init({
    dsn,
    release: __APP_VERSION__,
    integrations: [Sentry.vueIntegration({ app, attachProps: false })],
    beforeSend: (event) => (isCancellationNoise(event) ? null : scrubEvent(event, newSalt())),
  });
}

/** 导航链路埋点：以步骤 + 错误分类作为指纹，把同类复现聚到一起 */
export function captureNavigationEvent(event: NavigationEvent): void {
  const level = event.errorClass ? "error" : "info";
  const fingerprint = event.errorClass
    ? ["post-commit-navigation", event.step, event.errorClass]
    : ["post-commit-navigation", event.step];
  const message = `post-commit-navigation:${event.step}`;

  // 成功导航（navigated-to-session / navigated-home）只写面包屑，不发事件——
  // 本链路用于错误观测，不是用户行为分析。
  if (!(REPORTABLE_STEPS as readonly string[]).includes(event.step)) {
    Sentry.addBreadcrumb({
      category: "post-commit-navigation",
      message,
      level,
      data: { ...event },
    });
    return;
  }

  // 业务状态只在面包屑里；事件 extra 只保留判定结果本身
  Sentry.captureMessage(message, {
    level,
    fingerprint,
    extra: { step: event.step, errorClass: event.errorClass },
  });
}

/** 原加载延迟数据改以 Sentry 事件走同一条上报链路；只有超过服务端阈值的才上报 */
export function captureMarkPerformance(
  event: MarkPerformanceEvent,
  thresholdMs: number | null | undefined,
): void {
  if (!thresholdMs || thresholdMs <= 0 || event.loadMs <= thresholdMs) return;
  Sentry.captureMessage("mark-performance", {
    level: "info",
    fingerprint: ["mark-performance"],
    extra: { ...event },
  });
}
