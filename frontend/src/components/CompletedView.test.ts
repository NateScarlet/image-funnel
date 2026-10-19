// #region Mock Setup
import { describe, test, expect, vi, beforeEach, afterEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import type { SessionFragment } from "@/graphql/generated";

const mocks = vi.hoisted(() => ({
  nextDirectoryId: undefined as unknown,
  resolveNextDirectory: vi.fn<() => Promise<string | undefined>>(),
  autoCreateSession: vi.fn<() => Promise<{ id: string } | undefined>>(),
}));

vi.mock("@/composables/useDirectoryProgress", () => ({
  default: () => ({
    nextDirectoryId: mocks.nextDirectoryId,
    resolveNextDirectory: mocks.resolveNextDirectory,
  }),
}));

vi.mock("@/composables/useAutoCreateSession", () => ({
  useAutoCreateSession: () => ({ autoCreateSession: mocks.autoCreateSession }),
}));

import CompletedView from "./CompletedView.vue";
// #endregion

// #region 测试数据与挂载辅助
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
      parentId: "parent-1",
      relPath: "p/dir-1",
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

const CommitFormStub = defineComponent({
  name: "CommitForm",
  emits: ["committed"],
  template: "<div />",
});

async function mountCompleted() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/browse", name: "browse", component: { template: "<div />" } },
      { path: "/session/:id", name: "session", component: { template: "<div />" } },
    ],
  });
  await router.replace({ path: "/session/session-1" });
  await router.isReady();

  const wrapper = mount(CompletedView, {
    props: { session: makeSession() },
    global: {
      plugins: [router],
      stubs: {
        CommitForm: CommitFormStub,
        DirectoryDisplay: true,
        KeptImagesGrid: true,
      },
    },
  });
  await flushPromises();
  return { wrapper, router };
}

/** 模拟提交完成：CommitForm 发出 committed 后经定时器进入自动跳转 */
async function emitCommitted(wrapper: Awaited<ReturnType<typeof mountCompleted>>["wrapper"]) {
  wrapper.findComponent(CommitFormStub).vm.$emit("committed");
  await vi.advanceTimersByTimeAsync(500);
  await flushPromises();
}
// #endregion

describe("CompletedView 提交后的自动跳转", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mocks.nextDirectoryId = ref<string | undefined>(undefined);
    mocks.resolveNextDirectory.mockReset();
    mocks.autoCreateSession.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test("解析出下一个目录时创建会话并跳转到新会话", async () => {
    mocks.nextDirectoryId = ref("dir-2");
    mocks.resolveNextDirectory.mockResolvedValue("dir-2");
    mocks.autoCreateSession.mockResolvedValue({ id: "session-2" });

    const { wrapper, router } = await mountCompleted();
    await emitCommitted(wrapper);

    expect(router.currentRoute.value.path).toBe("/session/session-2");
  });

  test("没有下一个目录时回到首页", async () => {
    mocks.resolveNextDirectory.mockResolvedValue(undefined);

    const { wrapper, router } = await mountCompleted();
    await emitCommitted(wrapper);

    expect(router.currentRoute.value.path).toBe("/");
  });

  test("创建下一会话失败时留在当前页以便重试", async () => {
    mocks.nextDirectoryId = ref("dir-2");
    mocks.resolveNextDirectory.mockResolvedValue("dir-2");
    mocks.autoCreateSession.mockResolvedValue(undefined);

    const { wrapper, router } = await mountCompleted();
    await emitCommitted(wrapper);

    expect(router.currentRoute.value.path).toBe("/session/session-1");
  });

  test("预览未就绪但解析出目标时仍以解析结果创建会话", async () => {
    // 预览值缺失（如统计尚未补齐），但提交时的解析结果已可确定
    mocks.resolveNextDirectory.mockResolvedValue("dir-9");
    mocks.autoCreateSession.mockResolvedValue({ id: "session-9" });

    const { wrapper, router } = await mountCompleted();
    await emitCommitted(wrapper);

    expect(router.currentRoute.value.path).toBe("/session/session-9");
  });
});
