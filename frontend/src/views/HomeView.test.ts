// #region 导入与 Mock Setup
import { describe, test, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { ref } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import HomeView from "./HomeView.vue";
import { RootDirectoryDocument } from "../graphql/generated";

vi.mock("@/graphql/utils/useQuery", () => ({
  default: vi.fn(),
}));

vi.mock("@/graphql/utils/useSubscription", () => ({
  default: vi.fn(() => ({ [Symbol.dispose]() {} })),
}));

// jsdom 无 IndexedDB：stub 组件间接加载真实 client 模块时会触发缓存恢复报错，
// mock idb-keyval 使加载期副作用静默成功，与本测试关注的导航行为无关
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));

vi.mock("@/composables/domain/useSession", () => ({
  default: () => ({ createSession: vi.fn(async () => null) }),
}));

vi.mock("@/composables/useDirectoryState", () => ({
  useDirectoryState: () => ({
    lastSession: ref(undefined),
    lastSessionState: ref(undefined),
  }),
}));

vi.mock("@/composables/useAutoCreateSession", () => ({
  useAutoCreateSession: () => ({ autoCreateSession: vi.fn(async () => undefined) }),
}));

import useQuery from "@/graphql/utils/useQuery";
// #endregion

// #region 挂载辅助
async function mountHome(directoryId?: string): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: HomeView },
      { path: "/browse", name: "browse", component: { template: "<div />" } },
    ],
  });
  await router.replace(directoryId ? { path: "/", query: { dir: directoryId } } : "/");
  await router.isReady();

  const wrapper = mount(HomeView, {
    global: {
      plugins: [router],
      stubs: {
        NotificationCenterButton: true,
        DeviceManagerButton: true,
        DirectorySelector: true,
        CreateSessionForm: true,
      },
    },
  });
  return { wrapper, router };
}
// #endregion

describe("HomeView 应用名称点击回到无查询参数状态", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation((doc: unknown) => {
      if (doc === RootDirectoryDocument) {
        return { data: ref({ rootDirectory: { id: "root-1" } }) } as ReturnType<typeof useQuery>;
      }
      return { data: ref(undefined) } as ReturnType<typeof useQuery>;
    });
  });

  test("带 dir 查询参数时，点击应用名称后清除全部查询参数", async () => {
    const { wrapper, router } = await mountHome("dir-1");
    expect(router.currentRoute.value.query.dir).toBe("dir-1");

    const titleLink = wrapper.get("h1 a");
    expect(titleLink.text()).toBe("ImageFunnel");
    await titleLink.trigger("click");
    await flushPromises();

    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe("/");
      expect(router.currentRoute.value.query).toEqual({});
    });
  });

  test("无查询参数时，点击应用名称保持在首页且不产生查询参数", async () => {
    const { wrapper, router } = await mountHome();

    await wrapper.get("h1 a").trigger("click");
    await flushPromises();

    expect(router.currentRoute.value.path).toBe("/");
    expect(router.currentRoute.value.query).toEqual({});
  });
});
// #endregion
