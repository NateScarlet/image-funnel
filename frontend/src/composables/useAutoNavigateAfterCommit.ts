import { toValue, type MaybeRefOrGetter } from "vue";
import type { CaptureNavigationEvent, NavigationContextSnapshot } from "@/utils/telemetry";
import { classifyNavigationError, sessionNavigationContext } from "@/utils/telemetry";
import type { ImageFiltersInput, SessionFragment } from "@/graphql/generated";

/** 创建下一筛选会话所需的当前会话配置（沿用完成会话的条件，只合并 rating） */
export interface CreateNextSessionInput {
  directoryId: string;
  filter: ImageFiltersInput;
  targetKeep: number;
}

/**
 * 「提交后切换到下一目录」用例的注入依赖。
 * 全部由最外层显式构建，便于替换后断言编排时序而不依赖组件挂载。
 */
export interface AutoNavigateAfterCommitDeps {
  /** 决策下一个目录：取页等待取消以专用错误表达，其余失败向上抛出 */
  resolveNextDirectory: () => Promise<string | undefined>;
  /** 用上次配置创建下一筛选会话；解析为空表示失败 */
  createSession: (input: CreateNextSessionInput) => Promise<SessionFragment | undefined>;
  /** 进入创建好的筛选会话 */
  navigateToSession: (sessionId: string) => Promise<unknown> | void;
  /** 没有下一个目录时回到主页 */
  navigateHome: () => Promise<unknown> | void;
  /** 展示未被全局错误链覆盖的本地失败 */
  showLocalError: (message: string) => void;
  /** 导航链路埋点出口，由最外层注入，深 module 不感知 Sentry */
  capture: CaptureNavigationEvent;
  /** 决策 composable 掌握的游标、统计与决策分支，失败事件也要带上 */
  navigationContext: () => NavigationContextSnapshot;
  /** 导航发起前的视图标识，供外层判断导航是否真的生效 */
  routeSnapshot: () => string;
  /** 导航完成后观察视图是否切换，由最外层注入（含观察窗口），深 module 只消费结果 */
  verifyNavigation: (before: string) => Promise<NavigationOutcome>;
}

/** 路由导航后的实际结果；用于区分"没切换"与"闪烁后回到原视图" */
export type NavigationOutcome = "switched" | "unchanged" | "flicker";

const DEFAULT_DELAY_MS = 500;

/**
 * 提交后自动切换到下一目录的用例级深 module。
 *
 * 一次命令封装完整序列：延时展示提交结果 → 防重锁 → 决策下一个目录 →
 * 用上次配置创建下一会话 → 路由到新会话或主页 → 错误分类与重试。
 * 调用者只发起命令，不感知这六步的顺序与中间结果。
 * 延时未到时再次发起命令会立即执行（供「完成」按钮跳过等待），序列进行中则忽略重复触发。
 */
export function useAutoNavigateAfterCommit(
  session: MaybeRefOrGetter<SessionFragment | null | undefined>,
  deps: AutoNavigateAfterCommitDeps,
) {
  // #region 单飞与延时
  // 延时未到时再次收到命令表示调用方已展示完提交结果（如点了「完成」），
  // 此时跳过剩余延时立即执行同一次切换；序列进行中则忽略重复触发。
  let pending: { timer: ReturnType<typeof setTimeout>; session: SessionFragment } | undefined;
  let navigating = false;

  function switchToNextDirectory(): void {
    if (navigating) {
      const current = toValue(session);
      if (current) {
        deps.capture({
          ...sessionNavigationContext(current),
          ...deps.navigationContext(),
          step: "duplicate-skipped",
        });
      }
      return;
    }

    if (pending) {
      const { timer, session: currentSession } = pending;
      clearTimeout(timer);
      pending = undefined;
      deps.capture({
        ...sessionNavigationContext(currentSession),
        ...deps.navigationContext(),
        step: "delay-skipped",
      });
      void runSequence(currentSession);
      return;
    }

    // 立刻捕获发起命令的会话配置：延时期间若路由已切换，不能把新会话的配置带到目标目录
    const currentSession = toValue(session);
    if (!currentSession) throw new Error("无法创建下一个筛选会话：当前会话不存在");

    const timer = setTimeout(() => {
      pending = undefined;
      void runSequence(currentSession);
    }, DEFAULT_DELAY_MS);
    pending = { timer, session: currentSession };
  }
  // #endregion

  // #region 编排序列
  async function runSequence(currentSession: SessionFragment): Promise<void> {
    if (navigating) return;
    navigating = true;
    // 现场在事件发生时读取：决策推进过程中游标与分支会变化
    const context = () => ({
      ...sessionNavigationContext(currentSession),
      ...deps.navigationContext(),
    });

    const routeBefore = deps.routeSnapshot();

    try {
      deps.capture({ ...context(), step: "navigation-started" });

      // 提交完成后再决定目标：导航前补齐分页与统计，并按当前状态重新判定完成度
      deps.capture({ ...context(), step: "decision-started" });
      const directoryId = await deps.resolveNextDirectory();
      deps.capture({
        ...context(),
        step: "decision-resolved",
        nextDirectoryId: directoryId,
      });

      if (directoryId) {
        const nextSession = await deps.createSession({
          directoryId,
          filter: currentSession.filter,
          targetKeep: currentSession.targetKeep,
        });

        // 建会话没有返回结果同样是失败：必须提示并释放锁，不能停在完成视图且毫无反馈
        if (!nextSession) {
          deps.capture({
            ...context(),
            step: "session-create-failed",
            nextDirectoryId: directoryId,
            errorClass: "local",
            message: "无法创建下一个筛选会话",
          });
          throw new Error("无法创建下一个筛选会话");
        }

        deps.capture({ ...context(), step: "session-created", nextDirectoryId: directoryId });
        await deps.navigateToSession(nextSession.id);
        deps.capture({
          ...context(),
          step: "navigated-to-session",
          nextDirectoryId: directoryId,
        });
      } else {
        await deps.navigateHome();
        deps.capture({ ...context(), step: "navigated-home" });
      }

      // 路由 promise 完成不等于视图真的切换：提交后停在完成视图/闪烁是本 issue 要观测的失败
      const outcome = await deps.verifyNavigation(routeBefore);
      if (outcome !== "switched") {
        deps.capture({
          ...context(),
          step: outcome === "flicker" ? "navigation-flicker" : "navigation-not-switched",
          errorClass: "local",
          message: outcome === "flicker" ? "导航后视图闪烁，回到原视图" : "导航后仍停留在原视图",
        });
      }
    } catch (error) {
      const errorClass = classifyNavigationError(error);

      // 取页等待被取消（切换目录/卸载）不是失败，静默中止，不产生需要开发者处理的事件
      if (errorClass === "cancelled") return;

      deps.capture({
        ...context(),
        step: "error-classified",
        errorClass,
        message: error instanceof Error ? error.message : String(error),
      });
      // Apollo 请求错误已由全局 ErrorLink 提示；只补充未被其覆盖的本地错误，
      // 否则「分页游标未推进」之类的失败会停留在完成视图且没有任何反馈
      if (errorClass === "graphql" || errorClass === "link") return;

      deps.showLocalError(error instanceof Error ? error.message : String(error));
    } finally {
      // 无论成功失败都释放锁：失败后用户可再次触发重试
      navigating = false;
    }
  }
  // #endregion

  return { switchToNextDirectory };
}
