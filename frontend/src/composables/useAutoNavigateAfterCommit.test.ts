// #region 说明
// 覆盖「提交后切换到下一目录」用例级深模块的编排时序。
// 测试只替换注入的依赖（决策、建会话、路由、本地提示），
// 不挂载完成视图、不 stub 任何 UI 子组件，即可断言延时、单飞、错误分类与重试。
// #endregion

import { beforeEach, describe, expect, test, vi } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { CombinedGraphQLErrors, LinkError, registerLinkError } from "@apollo/client/errors";
import {
  useAutoNavigateAfterCommit,
  type AutoNavigateAfterCommitDeps,
} from "./useAutoNavigateAfterCommit";
import { DirectoryPageWaitCancelledError } from "@/utils/directoryPageWaitCancelled";
import type { SessionFragment } from "@/graphql/generated";

// #region 测试数据与辅助
function makeSession(): SessionFragment {
  return {
    __typename: "Session",
    id: "session-1",
    targetKeep: 0,
    createdAt: "",
    updatedAt: "",
    canCommit: true,
    canUndo: false,
    currentIndex: 0,
    currentSize: 1,
    currentRound: 0,
    currentRoundActions: [],
    directory: {
      __typename: "Directory",
      id: "dir-1",
      parentId: null,
      relPath: "dir-1",
      root: false,
      lastSession: null,
      state: null,
    },
    filter: {
      __typename: "ImageFilters",
      id: null,
      directoryId: null,
      rating: null,
      label: null,
      query: null,
    },
    stats: {
      __typename: "SessionStats",
      totalCount: 1,
      totalKept: 1,
      totalShelved: 0,
      totalRejected: 0,
      currentRoundRemaining: 0,
      isCompleted: true,
    },
    currentImage: null,
    nextImages: [],
  };
}

function makeDeps(
  overrides: Partial<AutoNavigateAfterCommitDeps> = {},
): AutoNavigateAfterCommitDeps {
  return {
    resolveNextDirectory: vi.fn(async () => undefined),
    createSession: vi.fn(async () => ({ id: "session-2" }) as SessionFragment),
    navigateToSession: vi.fn(async () => undefined),
    navigateHome: vi.fn(async () => undefined),
    showLocalError: vi.fn(),
    ...overrides,
  };
}

/** 触发一次命令并跑完延时与后续序列 */
async function fireAndSettle(switchToNextDirectory: () => void): Promise<void> {
  vi.useFakeTimers();
  try {
    switchToNextDirectory();
    await vi.advanceTimersByTimeAsync(500);
    await flushPromises();
  } finally {
    vi.useRealTimers();
  }
}
// #endregion

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("useAutoNavigateAfterCommit 提交后切换编排", () => {
  test("命令在延时后顺序执行决策、建会话与路由", async () => {
    const order: string[] = [];
    const deps = makeDeps({
      resolveNextDirectory: vi.fn(async () => {
        order.push("resolve");
        return "dir-2";
      }),
      createSession: vi.fn(async () => {
        order.push("create");
        return { id: "session-2" } as SessionFragment;
      }),
      navigateToSession: vi.fn(async () => {
        order.push("navigate");
      }),
    });
    const session = makeSession();
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(session, deps);

    vi.useFakeTimers();
    try {
      switchToNextDirectory();
      await vi.advanceTimersByTimeAsync(499);
      expect(order).toEqual([]);

      await vi.advanceTimersByTimeAsync(1);
      await flushPromises();
      expect(order).toEqual(["resolve", "create", "navigate"]);
    } finally {
      vi.useRealTimers();
    }

    expect(deps.createSession).toHaveBeenCalledWith({
      directoryId: "dir-2",
      filter: session.filter,
      targetKeep: session.targetKeep,
    });
    expect(deps.navigateToSession).toHaveBeenCalledWith("session-2");
    expect(deps.navigateHome).not.toHaveBeenCalled();
  });

  test("延时内再次触发跳过等待立即执行，序列进行中重复触发只启动一次序列", async () => {
    let resolveDecision!: (id: string | undefined) => void;
    const deps = makeDeps({
      resolveNextDirectory: vi.fn(
        () =>
          new Promise<string | undefined>((resolve) => {
            resolveDecision = resolve;
          }),
      ),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    vi.useFakeTimers();
    try {
      switchToNextDirectory();
      expect(deps.resolveNextDirectory).not.toHaveBeenCalled();

      // 延时未到时再次触发（「完成」按钮）：跳过剩余等待，立即执行同一次切换
      switchToNextDirectory();
      expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(1);

      // 原定时器已清除，推进时间不会再启动第二次序列
      await vi.advanceTimersByTimeAsync(500);
      expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(1);

      // 序列进行中（决策尚未返回）再次触发仍被忽略
      switchToNextDirectory();
      expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(1);

      resolveDecision("dir-2");
      await flushPromises();
      expect(deps.createSession).toHaveBeenCalledTimes(1);
      expect(deps.navigateToSession).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  test("本地失败后释放锁，重试可成功回主页", async () => {
    const deps = makeDeps({
      resolveNextDirectory: vi
        .fn<() => Promise<string | undefined>>()
        .mockRejectedValueOnce(new Error("目录分页未能推进：已取页但没有新增目录"))
        .mockResolvedValueOnce(undefined),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.showLocalError).toHaveBeenCalledWith("目录分页未能推进：已取页但没有新增目录");
    expect(deps.navigateHome).not.toHaveBeenCalled();

    await fireAndSettle(switchToNextDirectory);
    expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(2);
    expect(deps.navigateHome).toHaveBeenCalledTimes(1);
  });

  test("目录取页等待取消时静默中止，不提示也不跳转", async () => {
    const deps = makeDeps({
      resolveNextDirectory: vi
        .fn<() => Promise<string | undefined>>()
        .mockRejectedValueOnce(
          new DirectoryPageWaitCancelledError("目录取页等待已取消：查询变量已切换"),
        ),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.showLocalError).not.toHaveBeenCalled();
    expect(deps.navigateHome).not.toHaveBeenCalled();
    expect(deps.navigateToSession).not.toHaveBeenCalled();
  });

  test("Apollo 请求错误已由全局 ErrorLink 提示，不在本地重复提示", async () => {
    const deps = makeDeps({
      resolveNextDirectory: vi.fn<() => Promise<string | undefined>>().mockRejectedValueOnce(
        new CombinedGraphQLErrors({
          data: null,
          errors: [{ message: "服务器错误" }],
        }),
      ),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.showLocalError).not.toHaveBeenCalled();
    expect(deps.navigateHome).not.toHaveBeenCalled();
  });

  test("Link 链错误已由全局 ErrorLink 提示，不在本地重复提示", async () => {
    const linkError = new Error("网络请求失败");
    registerLinkError(linkError);
    expect(LinkError.is(linkError)).toBe(true);

    const deps = makeDeps({
      resolveNextDirectory: vi
        .fn<() => Promise<string | undefined>>()
        .mockRejectedValueOnce(linkError),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.showLocalError).not.toHaveBeenCalled();
    expect(deps.navigateHome).not.toHaveBeenCalled();
  });

  test("没有下一个目录时回到主页且不建会话", async () => {
    const deps = makeDeps();
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(1);
    expect(deps.createSession).not.toHaveBeenCalled();
    expect(deps.navigateHome).toHaveBeenCalledTimes(1);
  });

  test("建会话返回空时提示并允许重试进入目标会话", async () => {
    const deps = makeDeps({
      resolveNextDirectory: vi.fn(async () => "dir-2"),
      createSession: vi
        .fn<() => Promise<SessionFragment | undefined>>()
        .mockResolvedValueOnce(undefined)
        .mockResolvedValueOnce({ id: "session-2" } as SessionFragment),
    });
    const { switchToNextDirectory } = useAutoNavigateAfterCommit(makeSession(), deps);

    await fireAndSettle(switchToNextDirectory);
    expect(deps.showLocalError).toHaveBeenCalledWith("无法创建下一个筛选会话");
    expect(deps.navigateToSession).not.toHaveBeenCalled();

    await fireAndSettle(switchToNextDirectory);
    expect(deps.resolveNextDirectory).toHaveBeenCalledTimes(2);
    expect(deps.navigateToSession).toHaveBeenCalledWith("session-2");
  });
});
