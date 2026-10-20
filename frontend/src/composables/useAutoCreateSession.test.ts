import { beforeEach, describe, expect, test, vi } from "vitest";
import { ref } from "vue";
import { useAutoCreateSession } from "./useAutoCreateSession";
import * as useSessionModule from "./domain/useSession";
import { DirectoryLastSessionDocument, DirectoryStateDocument } from "@/graphql/generated";

// #region Mock Setup
const { queryMock } = vi.hoisted(() => ({ queryMock: vi.fn() }));

vi.mock("@/graphql/utils/query", () => ({
  default: queryMock,
}));
// #endregion

// #region 测试数据工厂
function mockCreateSession() {
  const createSession = vi.fn().mockResolvedValue({ id: "new-session-id" });

  vi.spyOn(useSessionModule, "default").mockReturnValue({
    session: ref(undefined),
    data: ref(undefined),
    createSession,
    markImage: vi.fn(),
  } as never);

  return createSession;
}

interface DirectoryConfig {
  /** DirectoryState.lastSession，目录没有活跃会话时的配置快照 */
  lastSessionState?: unknown;
  /** Directory.lastSession，当前活跃会话 */
  lastSession?: unknown;
  default?: unknown;
}

/** 按文档分别返回目录状态与上次会话查询结果 */
function mockDirectoryConfig(config: DirectoryConfig) {
  queryMock.mockImplementation(async (document: unknown) => {
    if (document === DirectoryStateDocument) {
      return {
        data: {
          node: {
            __typename: "Directory",
            id: "dir-next",
            state: {
              __typename: "DirectoryState",
              updatedAt: "2026-10-21T00:00:00Z",
              browse: null,
              lastSession: config.lastSessionState ?? null,
              default: config.default ?? null,
            },
          },
        },
      };
    }
    if (document === DirectoryLastSessionDocument) {
      return {
        data: {
          node: {
            __typename: "Directory",
            id: "dir-next",
            lastSession: config.lastSession ?? null,
          },
        },
      };
    }
    throw new Error("unexpected query document");
  });
}
// #endregion

describe("useAutoCreateSession", () => {
  beforeEach(() => {
    queryMock.mockReset();
  });

  test("应当使用目标目录上次会话更新后的配置 (lastSession) 创建会话", async () => {
    const createSession = mockCreateSession();

    // lastSessionState 保存初始配置，而 lastSession 保存用户在会话中更新后的最新配置
    mockDirectoryConfig({
      lastSessionState: { id: "old-session", filter: { rating: [1] }, targetKeep: 5 },
      lastSession: {
        id: "updated-session",
        filter: { rating: [1, 2], label: ["red"], query: "test" },
        targetKeep: 10,
      },
    });

    const { autoCreateSession } = useAutoCreateSession("dir-next", { rating: [] }, 0, {
      mergeAllFilterFields: true,
    });

    await autoCreateSession();

    expect(createSession).toHaveBeenCalledWith({
      directoryId: "dir-next",
      filter: {
        rating: [1, 2],
        label: ["red"],
        query: "test",
      },
      targetKeep: 10,
      createActions: undefined,
    });
  });

  test("当无 label 过滤器时不应传递空数组 label: []（防止后端匹配空集合）", async () => {
    const createSession = mockCreateSession();

    mockDirectoryConfig({
      lastSession: { id: "session-1", filter: { rating: [0] }, targetKeep: 4 },
    });

    const { autoCreateSession } = useAutoCreateSession("dir-next", { rating: [] }, 0, {
      mergeAllFilterFields: true,
    });

    await autoCreateSession();

    expect(createSession).toHaveBeenCalledWith({
      directoryId: "dir-next",
      filter: {
        rating: [0],
        label: undefined,
        query: undefined,
      },
      targetKeep: 4,
      createActions: undefined,
    });
  });

  test("目标目录刚确定时，应等待该目录配置查询返回后再创建会话", async () => {
    const createSession = mockCreateSession();

    let resolveLastSession!: (value: unknown) => void;
    const lastSessionResult = new Promise((resolve) => {
      resolveLastSession = resolve;
    });

    queryMock.mockImplementation(async (document: unknown) => {
      if (document === DirectoryStateDocument) {
        return { data: { node: { __typename: "Directory", id: "dir-next", state: null } } };
      }
      if (document === DirectoryLastSessionDocument) {
        return lastSessionResult;
      }
      throw new Error("unexpected query document");
    });

    const { autoCreateSession } = useAutoCreateSession(
      "dir-next",
      // 刚完成会话的条件：不能被目标目录尚未返回的配置顶替
      { rating: [1] },
      3,
      { mergeAllFilterFields: false },
    );

    const pending = autoCreateSession();
    await Promise.resolve();

    expect(queryMock).toHaveBeenCalledWith(DirectoryLastSessionDocument, {
      variables: { id: "dir-next" },
    });
    expect(createSession).not.toHaveBeenCalled();

    resolveLastSession({
      data: {
        node: {
          __typename: "Directory",
          id: "dir-next",
          lastSession: { filter: { rating: [4] }, targetKeep: 7 },
        },
      },
    });
    await pending;

    expect(createSession).toHaveBeenCalledWith({
      directoryId: "dir-next",
      filter: { rating: [4] },
      targetKeep: 7,
      createActions: undefined,
    });
  });
});
