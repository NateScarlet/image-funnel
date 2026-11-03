// #region 说明
// SessionView 在路由参数变化时复用（KeepAlive 保持 ImageViewer 实例）。
// 提交后切换到同一目录的下一会话时，新会话首图可能与上一会话图片 ID 完全相同：
// src 与 data-image-id 都不变，浏览器不会再次触发 load 事件。
// 若查看器不重新上报 image-loaded，新会话的显示中图片始终为空，
// 方向键 / 操作按钮 / 滑动都没有标记目标。
// #endregion

import { describe, expect, test, vi } from "vitest";
import { KeepAlive, defineComponent, h, ref } from "vue";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import type { ImageFragment } from "@/graphql/generated";

// #region Mock Setup
// 查看器及其子组件依赖 GraphQL 与运行时上下文；本用例只关注加载事件的上报时机
vi.mock("@/graphql/utils/useQuery", async () => {
  const { shallowRef } = await import("vue");
  return {
    default: vi.fn(() => ({
      data: shallowRef(undefined),
      query: {},
      refresh: vi.fn(),
    })),
  };
});
vi.mock("@/graphql/utils/useSubscription", () => ({
  default: vi.fn(() => ({ [Symbol.dispose]() {} })),
}));
// jsdom 无 IndexedDB：真实 client 模块加载期会触发缓存恢复报错，与本用例无关
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));
vi.mock("@/composables/useOpenDir", () => ({
  useOpenDir: () => ({ revealInExplorer: vi.fn() }),
}));
// 预加载与加载事件上报无关；jsdom 的 fetch 无法解析相对 URL，会污染测试输出
vi.mock("@/composables/useAsyncTask", () => ({
  default: vi.fn(() => ({ [Symbol.dispose]() {} })),
}));
// #endregion

import ImageViewer from "./ImageViewer.vue";

// #region 测试数据与挂载辅助
function makeImage(id: string): ImageFragment {
  return {
    __typename: "Image",
    id,
    filename: `${id}.jpg`,
    url: `/image/${id}`,
    rawURL: `/image/${id}?raw`,
    url256: `/image/${id}?w=256`,
    url512: `/image/${id}?w=512`,
    url1024: `/image/${id}?w=1024`,
    url2048: `/image/${id}?w=2048`,
    url4096: `/image/${id}?w=4096`,
    modTime: "2026-01-01T00:00:00Z",
    width: 100,
    height: 100,
    size: 1,
    currentRating: 0,
    label: null,
    relPath: `${id}.jpg`,
    note: {
      __typename: "Note",
      id: `note-${id}`,
      relPath: `${id}.xmp`,
      content: "",
      rawContent: "",
      hidden: false,
      modTime: "2026-01-01T00:00:00Z",
    },
  };
}

/** 模拟 SessionView 的 KeepAlive 结构：会话切换时复用同一个 ImageViewer 实例 */
function mountViewer(imageId: string, sessionId: string) {
  const currentImage = ref<ImageFragment | undefined>(makeImage(imageId));
  const currentSessionId = ref(sessionId);

  const Harness = defineComponent({
    setup() {
      return () =>
        h(KeepAlive, null, () =>
          currentImage.value
            ? h(ImageViewer, {
                image: currentImage.value,
                sessionId: currentSessionId.value,
                preloadImages: [],
              })
            : undefined,
        );
    },
  });

  const wrapper = mount(Harness);
  return { wrapper, currentImage, currentSessionId, viewer: wrapper.findComponent(ImageViewer) };
}

/** jsdom 不会真正加载图片，手动指定 img 的加载状态并触发对应事件 */
async function setImgComplete(wrapper: VueWrapper, complete: boolean) {
  const img = wrapper.find("img");
  Object.defineProperty(img.element, "complete", { configurable: true, value: complete });
  await img.trigger("load");
}
// #endregion

describe("ImageViewer 会话切换后的加载上报", () => {
  test("KeepAlive 复用且图片 ID 相同时，切换会话后重新上报 image-loaded", async () => {
    const { wrapper, currentSessionId, viewer } = mountViewer("img-1", "session-a");

    // 首图加载完成：上一会话拿到显示中的图片
    await setImgComplete(wrapper, true);
    expect(viewer.emitted("image-loaded")).toHaveLength(1);

    // 切换到同一目录的下一会话：首图 ID 相同，src 不变，不会再触发 load
    currentSessionId.value = "session-b";
    await flushPromises();

    const events = viewer.emitted("image-loaded");
    expect(events).toHaveLength(2);
    expect(events?.[1]?.[0]).toMatchObject({ id: "img-1" });

    // 组件实例被 KeepAlive 复用，而不是重新挂载
    expect(wrapper.findComponent(ImageViewer).vm.$.uid).toBe(viewer.vm.$.uid);

    wrapper.unmount();
  });

  test("切换会话后图片不同时，仍等待新图加载完成才上报", async () => {
    const { wrapper, currentImage, currentSessionId, viewer } = mountViewer("img-1", "session-a");

    await setImgComplete(wrapper, true);
    expect(viewer.emitted("image-loaded")).toHaveLength(1);

    // 新会话首图不同：src 变化，加载尚未完成
    Object.defineProperty(wrapper.find("img").element, "complete", {
      configurable: true,
      value: false,
    });
    currentSessionId.value = "session-b";
    currentImage.value = makeImage("img-2");
    await flushPromises();

    // 不能把尚未加载的新图当作已显示图片上报
    expect(viewer.emitted("image-loaded")).toHaveLength(1);

    await setImgComplete(wrapper, true);
    const events = viewer.emitted("image-loaded");
    expect(events).toHaveLength(2);
    expect(events?.[1]?.[0]).toMatchObject({ id: "img-2" });

    wrapper.unmount();
  });
});
