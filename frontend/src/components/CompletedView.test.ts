// #region 说明
// 覆盖完成视图在自动切换失败时的可见反馈与可重试行为：
// 本地错误（如分页游标未推进）不会经过全局 ErrorLink，必须由组件自己提示；
// Apollo 请求错误已由全局 ErrorLink 提示，组件不得重复提示。
// CommitForm 以桩替代，只用于触发 committed 事件，避免测试依赖真实提交链路。
// #endregion

import { beforeEach, describe, expect, test, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import { CombinedGraphQLErrors } from "@apollo/client/errors";
import CompletedView from "./CompletedView.vue";
import type { SessionFragment } from "../graphql/generated";
import { DirectoryPageWaitCancelledError } from "../utils/directoryPageWaitCancelled";

// #region Mock Setup
const notificationMocks = vi.hoisted(() => ({ showError: vi.fn() }));
const autoCreateMocks = vi.hoisted(() => ({ autoCreateSession: vi.fn() }));

vi.mock("../composables/useNotification", () => ({
  default: () => ({
    notifications: { value: [] },
    show: vi.fn(),
    showError: notificationMocks.showError,
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
  }),
}));

// jsdom 无 IndexedDB：组件间接加载真实 client 模块时会触发缓存恢复报错，与本用例无关
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));

// 自动创建会话涉及真实 GraphQL，本用例只关心失败反馈
vi.mock("../composables/useAutoCreateSession", () => ({
  useAutoCreateSession: () => ({ autoCreateSession: autoCreateMocks.autoCreateSession }),
}));

// 真实 CommitForm 依赖写入操作设置与 GraphQL，替换为「触发 committed 即完成提交」的桩
vi.mock("./CommitForm.vue", async () => {
  const { defineComponent } = await import("vue");
  return {
    default: defineComponent({
      name: "CommitForm",
      props: {
        session: { type: Object, required: true },
        showHeader: { type: Boolean, default: true },
        title: { type: String, default: "" },
      },
      emits: ["committed"],
      setup(_, { emit, expose }) {
        expose({ commit: () => emit("committed"), reset: () => {} });
      },
      template: "<div />",
    }),
  };
});
// #endregion

// #region 测试数据与挂载辅助
/** 保留至少一张图片的会话：跳过「确认排除所有图片」对话框，直接提交 */
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

function mountCompleted(resolveNextDirectory: () => Promise<string | undefined>): {
  wrapper: VueWrapper;
  router: Router;
} {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/session/:id", name: "session", component: { template: "<div />" } },
    ],
  });

  const wrapper = mount(CompletedView, {
    props: {
      session: makeSession(),
      nextDirectoryId: undefined,
      resolveNextDirectory,
    },
    global: {
      plugins: [router],
      stubs: {
        DirectoryDisplay: true,
        KeptImagesGrid: true,
      },
    },
  });

  return { wrapper, router };
}

/** 触发一次提交并等待自动切换的延时，让导航流程跑完 */
async function submitAndSettle(wrapper: VueWrapper): Promise<void> {
  vi.useFakeTimers();
  try {
    (wrapper.vm as unknown as { submit: () => void }).submit();
    await vi.advanceTimersByTimeAsync(500);
    await flushPromises();
  } finally {
    vi.useRealTimers();
  }
}
// #endregion

beforeEach(() => {
  notificationMocks.showError.mockClear();
  autoCreateMocks.autoCreateSession.mockReset();
  // 默认创建失败（返回空），需要成功进入下一目录会话的用例自行覆盖
  autoCreateMocks.autoCreateSession.mockResolvedValue(undefined);
});

describe("CompletedView 自动切换失败反馈", () => {
  test("本地错误可见提示，且释放导航锁后可以重试", async () => {
    const resolveNextDirectory = vi
      .fn<() => Promise<string | undefined>>()
      .mockRejectedValueOnce(new Error("目录分页未能推进：已取页但没有新增目录"))
      .mockResolvedValueOnce(undefined);

    const { wrapper, router } = mountCompleted(resolveNextDirectory);
    const pushSpy = vi.spyOn(router, "push");

    // 第一次失败：用户必须看到提示，而不是停留在完成视图且毫无反馈
    await submitAndSettle(wrapper);
    expect(notificationMocks.showError).toHaveBeenCalledWith(
      "目录分页未能推进：已取页但没有新增目录",
    );
    expect(pushSpy).not.toHaveBeenCalled();

    // 重试：导航锁已释放，第二次触发会再次解析并正常回主页
    await submitAndSettle(wrapper);
    expect(resolveNextDirectory).toHaveBeenCalledTimes(2);
    expect(pushSpy).toHaveBeenCalledWith("/");

    wrapper.unmount();
  });

  test("取页等待被取消时静默中止，不提示也不跳转", async () => {
    const resolveNextDirectory = vi
      .fn<() => Promise<string | undefined>>()
      .mockRejectedValueOnce(
        new DirectoryPageWaitCancelledError("目录取页等待已取消：查询变量已切换"),
      );

    const { wrapper, router } = mountCompleted(resolveNextDirectory);
    const pushSpy = vi.spyOn(router, "push");

    await submitAndSettle(wrapper);
    expect(notificationMocks.showError).not.toHaveBeenCalled();
    expect(pushSpy).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  test("下一个会话创建没有返回结果时提示并允许重试", async () => {
    const resolveNextDirectory = vi
      .fn<() => Promise<string | undefined>>()
      .mockResolvedValue("dir-2");

    const { wrapper, router } = mountCompleted(resolveNextDirectory);
    const pushSpy = vi.spyOn(router, "push");

    // 第一次：创建下一会话返回空，必须提示，不能停在完成视图且毫无反馈
    await submitAndSettle(wrapper);
    expect(notificationMocks.showError).toHaveBeenCalledWith("无法创建下一个筛选会话");
    expect(pushSpy).not.toHaveBeenCalled();

    // 重试：创建成功则进入目标目录会话
    autoCreateMocks.autoCreateSession.mockResolvedValueOnce({ id: "session-2" });
    await submitAndSettle(wrapper);
    expect(resolveNextDirectory).toHaveBeenCalledTimes(2);
    expect(pushSpy).toHaveBeenCalledWith({ name: "session", params: { id: "session-2" } });

    wrapper.unmount();
  });

  test("Apollo 请求错误已由全局 ErrorLink 提示，组件不重复提示", async () => {
    const graphQLError = new CombinedGraphQLErrors({
      data: null,
      errors: [{ message: "服务器错误" }],
    });
    const resolveNextDirectory = vi
      .fn<() => Promise<string | undefined>>()
      .mockRejectedValueOnce(graphQLError);

    const { wrapper } = mountCompleted(resolveNextDirectory);

    await submitAndSettle(wrapper);
    expect(notificationMocks.showError).not.toHaveBeenCalled();

    wrapper.unmount();
  });
});
