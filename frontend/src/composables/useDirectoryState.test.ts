import { describe, test, expect, vi } from "vitest";
import { ref } from "vue";
import { useDirectoryState } from "./useDirectoryState";
import { DirectoryStateDocument, DirectoryLastSessionDocument } from "../graphql/generated";

vi.mock("../graphql/utils/useQuery", () => {
  return {
    default: vi.fn(),
  };
});

import useQuery from "../graphql/utils/useQuery";

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

    const { lastSession } = useDirectoryState("dir-1");

    expect(lastSession.value).toEqual(activeSessionObj);
  });
});
