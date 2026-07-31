import { describe, test, expect, vi } from "vitest";
import { ref } from "vue";
import { useAutoCreateSession } from "./useAutoCreateSession";
import * as useDirectoryStateModule from "./useDirectoryState";
import * as useSessionModule from "./domain/useSession";

// #region 自动创建会话测试
describe("useAutoCreateSession", () => {
  test("应当使用用户更新后的会话配置 (lastSession) 创建会话", async () => {
    const mockCreateSession = vi.fn().mockResolvedValue({ id: "new-session-id" });

    vi.spyOn(useSessionModule, "default").mockReturnValue({
      session: ref(undefined),
      data: ref(undefined),
      createSession: mockCreateSession,
      markImage: vi.fn(),
    } as any); // eslint-disable-line @typescript-eslint/no-explicit-any

    // 模拟 lastSessionState 保存初始配置，而 lastSession 保存用户在会话中更新后的最新配置
    const mockLastSessionState = ref({
      id: "old-session",
      filter: { rating: [1] },
      targetKeep: 5,
    });

    const mockLastSession = ref({
      id: "updated-session",
      filter: { rating: [1, 2], label: ["red"], query: "test" },
      targetKeep: 10,
    });

    const mockDefaultState = ref(undefined);

    vi.spyOn(useDirectoryStateModule, "useDirectoryState").mockReturnValue({
      filterRating: ref([]),
      filterLabels: ref([]),
      searchQuery: ref(""),
      imageFilters: ref(undefined),
      showHiddenNotes: ref(false),
      hasActiveFilters: ref(false),
      clearFilters: vi.fn(),
      lastSession: mockLastSession,
      lastSessionState: mockLastSessionState,
      defaultState: mockDefaultState,
    } as any); // eslint-disable-line @typescript-eslint/no-explicit-any

    const { autoCreateSession } = useAutoCreateSession("dir-1", { rating: [] }, 0, {
      mergeAllFilterFields: true,
    });

    await autoCreateSession();

    // 预期创建会话时采用 lastSession 的更新后配置
    expect(mockCreateSession).toHaveBeenCalledWith({
      directoryId: "dir-1",
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
    const mockCreateSession = vi.fn().mockResolvedValue({ id: "new-session-id" });

    vi.spyOn(useSessionModule, "default").mockReturnValue({
      session: ref(undefined),
      data: ref(undefined),
      createSession: mockCreateSession,
      markImage: vi.fn(),
    } as any); // eslint-disable-line @typescript-eslint/no-explicit-any

    const mockLastSession = ref({
      id: "session-1",
      filter: { rating: [0] },
      targetKeep: 4,
    });

    vi.spyOn(useDirectoryStateModule, "useDirectoryState").mockReturnValue({
      filterRating: ref([]),
      filterLabels: ref([]),
      searchQuery: ref(""),
      imageFilters: ref(undefined),
      showHiddenNotes: ref(false),
      hasActiveFilters: ref(false),
      clearFilters: vi.fn(),
      lastSession: mockLastSession,
      lastSessionState: ref(undefined),
      defaultState: ref(undefined),
    } as any); // eslint-disable-line @typescript-eslint/no-explicit-any

    const { autoCreateSession } = useAutoCreateSession("dir-1", { rating: [] }, 0, {
      mergeAllFilterFields: true,
    });

    await autoCreateSession();

    expect(mockCreateSession).toHaveBeenCalledWith({
      directoryId: "dir-1",
      filter: {
        rating: [0],
        label: undefined,
        query: undefined,
      },
      targetKeep: 4,
      createActions: undefined,
    });
  });
});
// #endregion
