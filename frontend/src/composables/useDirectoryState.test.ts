import { describe, test, expect, vi } from "vitest";
import { ref } from "vue";
import { useDirectoryState } from "./useDirectoryState";
import { DirectoryStateDocument, DirectoryLastSessionDocument } from "../graphql/generated";

vi.mock("../graphql/utils/useQuery", () => {
  return {
    default: vi.fn(),
  };
});

vi.mock("../graphql/utils/mutate", () => ({
  default: vi.fn().mockResolvedValue(undefined),
}));

import useQuery from "../graphql/utils/useQuery";

/** 同时喂入目录状态与 active 会话两份查询结果 */
function mockDirectoryQueries(stateData: unknown, lastSessionData: unknown) {
  vi.mocked(useQuery).mockImplementation((doc: unknown) => {
    if (doc === DirectoryStateDocument) {
      return { data: ref(stateData) } as ReturnType<typeof useQuery>;
    }
    if (doc === DirectoryLastSessionDocument) {
      return { data: ref(lastSessionData) } as ReturnType<typeof useQuery>;
    }
    return { data: ref(undefined) } as ReturnType<typeof useQuery>;
  });
}

function stateNode(state: unknown) {
  return { node: { __typename: "Directory", state } };
}

function lastSessionNode(lastSession: unknown) {
  return { node: { __typename: "Directory", lastSession } };
}

describe("useDirectoryState - lastSession", () => {
  test("当服务端没有活跃会话时，即使 serverState 存在 lastSession，lastSession 也应为 undefined", () => {
    const mockStateData = ref({
      node: {
        __typename: "Directory",
        state: {
          updatedAt: "2026-08-01T12:00:00Z",
          lastSession: {
            id: "old-dead-session",
            filter: { rating: [1] },
            targetKeep: 5,
          },
        },
      },
    });

    const mockLastSessionData = ref({
      node: {
        __typename: "Directory",
        lastSession: null,
      },
    });

    vi.mocked(useQuery).mockImplementation((doc: unknown) => {
      if (doc === DirectoryStateDocument) {
        return { data: mockStateData } as ReturnType<typeof useQuery>;
      }
      if (doc === DirectoryLastSessionDocument) {
        return { data: mockLastSessionData } as ReturnType<typeof useQuery>;
      }
      return { data: ref(undefined) } as ReturnType<typeof useQuery>;
    });

    const { lastSession, lastSessionState } = useDirectoryState("dir-1");

    expect(lastSessionState.value).toEqual({
      id: "old-dead-session",
      filter: { rating: [1] },
      targetKeep: 5,
    });
    // 关键断言：服务端无 activeSession 时，lastSession 必须为 undefined，不得从 state 容错兜底
    expect(lastSession.value).toBeUndefined();
  });

  test("当服务端存在活跃会话时，lastSession 应当返回服务端的 Session 对象", () => {
    const mockStateData = ref({
      node: {
        __typename: "Directory",
        state: undefined,
      },
    });

    const activeSessionObj = {
      id: "active-session-123",
      updatedAt: "2026-08-05T20:00:00Z",
      filter: { id: "f1", directoryId: "dir-1", rating: [5] },
      targetKeep: 10,
    };

    const mockLastSessionData = ref({
      node: {
        __typename: "Directory",
        lastSession: activeSessionObj,
      },
    });

    vi.mocked(useQuery).mockImplementation((doc: unknown) => {
      if (doc === DirectoryStateDocument) {
        return { data: mockStateData } as ReturnType<typeof useQuery>;
      }
      if (doc === DirectoryLastSessionDocument) {
        return { data: mockLastSessionData } as ReturnType<typeof useQuery>;
      }
      return { data: ref(undefined) } as ReturnType<typeof useQuery>;
    });

    const { lastSession } = useDirectoryState("dir-2");

    expect(lastSession.value).toEqual(activeSessionObj);
  });
});

// #region 展示用筛选解析
describe("useDirectoryState - imageFilters", () => {
  test("本地暂存编辑优先级最高，且暂存为空时也不回退到服务端", () => {
    mockDirectoryQueries(
      stateNode({
        updatedAt: "2026-10-01T00:00:00Z",
        browse: { filterBy: { rating: [1] } },
        lastSession: { id: "s", filter: { rating: [2] }, targetKeep: 5 },
        default: null,
      }),
      lastSessionNode(null),
    );

    const { imageFilters, filterRating } = useDirectoryState("dir-staged");
    filterRating.value = [9];

    expect(imageFilters.value).toEqual({ rating: [9], label: undefined, query: undefined });
  });

  test("已持久化的浏览配置优先于上次配置", () => {
    mockDirectoryQueries(
      stateNode({
        updatedAt: "2026-10-01T00:00:00Z",
        browse: { filterBy: { rating: [1], label: ["browse"], query: "bq" } },
        lastSession: {
          id: "s",
          filter: { rating: [2], label: ["last"], query: "lq" },
          targetKeep: 5,
        },
        default: null,
      }),
      lastSessionNode(null),
    );

    const { imageFilters } = useDirectoryState("dir-browse");

    expect(imageFilters.value).toEqual({ rating: [1], label: ["browse"], query: "bq" });
  });

  test("默认写入评分的 keepRating 覆盖展示用的 rating，其余字段沿用上次配置", () => {
    mockDirectoryQueries(
      stateNode({
        updatedAt: "2026-10-01T00:00:00Z",
        browse: null,
        lastSession: {
          id: "s",
          filter: { rating: [1], label: ["x"], query: "q" },
          targetKeep: 5,
        },
        default: {
          writeActions: { keepRating: 7, shelveRating: 3, rejectRating: 1 },
        },
      }),
      lastSessionNode(null),
    );

    const { imageFilters } = useDirectoryState("dir-keep-rating");

    expect(imageFilters.value).toEqual({ rating: [7], label: ["x"], query: "q" });
  });

  test("没有上次配置时返回 undefined", () => {
    mockDirectoryQueries(
      stateNode({
        updatedAt: "2026-10-01T00:00:00Z",
        browse: null,
        lastSession: null,
        default: null,
      }),
      lastSessionNode(null),
    );

    const { imageFilters } = useDirectoryState("dir-empty");

    expect(imageFilters.value).toBeUndefined();
  });
});
// #endregion
