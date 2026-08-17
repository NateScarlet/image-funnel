// #region 导入与 Mock Setup
import { describe, test, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import CommitForm from "./CommitForm.vue";
import {
  CommitChangesDocument,
  SetDirectoryStateDocument,
  type SessionFragment,
} from "@/graphql/generated";
import type mutate from "@/graphql/utils/mutate";

const { defaultStateRef, mockMutate } = vi.hoisted(() => {
  const state = {
    value: undefined as
      | {
          writeActions: {
            keepRating: number | null;
            shelveRating: number | null;
            rejectRating: number | null;
          };
        }
      | undefined,
  };
  const fn = vi.fn<typeof mutate>();
  return { defaultStateRef: state, mockMutate: fn };
});

vi.mock("@mdi/js", () => ({
  mdiLoading: "",
  mdiStar: "",
  mdiStarOutline: "",
}));

vi.mock("@/graphql/utils/mutate", () => ({
  default: mockMutate,
}));

vi.mock("@/composables/usePresets", () => ({
  usePresets: () => ({
    getPreset: () => undefined,
    lastSelectedPresetId: { value: "" },
  }),
}));

vi.mock("@/composables/useDirectoryState", () => ({
  useDirectoryState: () => ({ defaultState: defaultStateRef }),
}));

function makeSession(): SessionFragment {
  return {
    __typename: "Session",
    id: "session-1",
    targetKeep: 5,
    createdAt: "",
    updatedAt: "",
    canCommit: true,
    canUndo: true,
    currentIndex: 0,
    currentSize: 5,
    currentRound: 1,
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
      id: [],
      directoryId: [],
      rating: [],
      label: [],
      query: null,
    },
    stats: {
      __typename: "SessionStats",
      totalCount: 5,
      totalKept: 2,
      totalShelved: 2,
      totalRejected: 1,
      currentRoundRemaining: 0,
      isCompleted: true,
    },
    currentImage: null,
    nextImages: [],
  } as SessionFragment;
}

async function mountForm(options: { stubRatingSelector?: boolean } = {}) {
  return mount(CommitForm, {
    props: { session: makeSession() },
    global: {
      stubs: options.stubRatingSelector ? { RatingSelector: true } : undefined,
    },
  });
}

async function waitForCommitted(wrapper: VueWrapper) {
  await vi.waitFor(
    () => {
      expect(wrapper.emitted("committed")).toBeTruthy();
    },
    { timeout: 2000, interval: 20 },
  );
}

async function mountAndSubmit() {
  const wrapper = await mountForm({ stubRatingSelector: true });
  await wrapper.find("form").trigger("submit");
  await flushPromises();
  await waitForCommitted(wrapper);
  return wrapper;
}
// #endregion

// #region 提交后持久化默认写入操作
describe("CommitForm 提交后持久化默认写入操作", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    defaultStateRef.value = {
      writeActions: { keepRating: 5, shelveRating: 3, rejectRating: 1 },
    };
    mockMutate.mockImplementation(async (doc: unknown) => {
      if (doc === CommitChangesDocument) {
        return {
          data: { commitChanges: { written: 3, matched: 5 } },
          error: undefined,
        };
      }
      if (doc === SetDirectoryStateDocument) {
        return {
          data: {
            setDirectoryState: {
              directory: { id: "dir-1" },
              clientMutationId: null,
            },
          },
        };
      }
      return { data: undefined, error: undefined };
    });
  });

  test("提交成功后把本次实际使用的写入操作持久化为目录默认配置", async () => {
    const wrapper = await mountAndSubmit();

    const calls = mockMutate.mock.calls;
    expect(calls[0]?.[0]).toBe(CommitChangesDocument);
    expect(calls[1]?.[0]).toBe(SetDirectoryStateDocument);
    expect(calls[1]?.[1]).toMatchObject({
      variables: {
        input: {
          id: "dir-1",
          state: {
            default: {
              writeActions: { keepRating: 5, shelveRating: 3, rejectRating: 1 },
            },
          },
        },
      },
    });
    expect(wrapper.emitted("committed")).toHaveLength(1);
  });

  test("用户在表单中清除保留评分后，持久化的是本次实际提交的 null 而非旧默认值", async () => {
    const wrapper = await mountForm();

    // 第一行为保留的星级选择器：默认选中 5 星，勾选中的检查框将其清除为 null（clearable）
    const checkboxes = wrapper.findAll<HTMLInputElement>('input[type="checkbox"]');
    const keepStar5 = checkboxes[5];
    expect((keepStar5.element as HTMLInputElement).checked).toBe(true);
    await keepStar5.setValue(false);

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    await waitForCommitted(wrapper);

    const calls = mockMutate.mock.calls;
    expect(calls[0]?.[1]).toMatchObject({
      variables: {
        input: {
          writeActions: { keepRating: null, shelveRating: 3, rejectRating: 1 },
        },
      },
    });
    expect(calls[1]?.[1]).toMatchObject({
      variables: {
        input: {
          state: {
            default: {
              writeActions: { keepRating: null, shelveRating: 3, rejectRating: 1 },
            },
          },
        },
      },
    });
  });

  test("持久化失败不阻断已成功的提交，错误交由全局 ErrorLink 反馈", async () => {
    mockMutate.mockImplementation(async (doc: unknown) => {
      if (doc === CommitChangesDocument) {
        return {
          data: { commitChanges: { written: 3, matched: 5 } },
          error: undefined,
        };
      }
      throw new Error("persist failed");
    });

    const wrapper = await mountAndSubmit();

    const calls = mockMutate.mock.calls;
    expect(calls[0]?.[0]).toBe(CommitChangesDocument);
    expect(calls[1]?.[0]).toBe(SetDirectoryStateDocument);
    expect(wrapper.text()).toContain("✓ 提交成功");
    expect(wrapper.emitted("committed")).toHaveLength(1);
  });
});
// #endregion
