import { toValue, type MaybeRefOrGetter } from "vue";
import { CombinedGraphQLErrors, LinkError } from "@apollo/client/errors";
import { isDirectoryPageWaitCancelled } from "@/utils/directoryPageWaitCancelled";
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
}

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
    if (navigating) return;

    if (pending) {
      const { timer, session: currentSession } = pending;
      clearTimeout(timer);
      pending = undefined;
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

    try {
      // 提交完成后再决定目标：导航前补齐分页与统计，并按当前状态重新判定完成度
      const directoryId = await deps.resolveNextDirectory();

      if (directoryId) {
        const nextSession = await deps.createSession({
          directoryId,
          filter: currentSession.filter,
          targetKeep: currentSession.targetKeep,
        });

        // 建会话没有返回结果同样是失败：必须提示并释放锁，不能停在完成视图且毫无反馈
        if (!nextSession) throw new Error("无法创建下一个筛选会话");

        await deps.navigateToSession(nextSession.id);
      } else {
        await deps.navigateHome();
      }
    } catch (error) {
      // 取页等待被取消（切换目录/卸载）不是失败，静默中止，不打扰用户
      if (isDirectoryPageWaitCancelled(error)) return;
      // Apollo 请求错误已由全局 ErrorLink 提示；只补充未被其覆盖的本地错误，
      // 否则「分页游标未推进」之类的失败会停留在完成视图且没有任何反馈
      if (CombinedGraphQLErrors.is(error) || LinkError.is(error)) return;

      deps.showLocalError(error instanceof Error ? error.message : String(error));
    } finally {
      // 无论成功失败都释放锁：失败后用户可再次触发重试
      navigating = false;
    }
  }
  // #endregion

  return { switchToNextDirectory };
}
