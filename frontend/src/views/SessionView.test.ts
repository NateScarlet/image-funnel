// #region Mock Setup
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { nextTick, ref, type Ref } from "vue";
import type { ImageFragment, SessionFragment } from "@/graphql/generated";
import Time from "@/utils/Time";

const sessionHolder = vi.hoisted(() => ({
  current: undefined as unknown as Ref<SessionFragment | undefined>,
}));

// 会话数据是本用例唯一要控制的外部输入：由用例直接推进轮次与当前图片
function sessionRef(): Ref<SessionFragment | undefined> {
  sessionHolder.current ??= ref<SessionFragment | undefined>(undefined);
  return sessionHolder.current;
}

vi.mock("@/composables/domain/useSession", () => ({
  default: () => ({ session: sessionRef() }),
}));

vi.mock("@/composables/useSessionNextDirectory", () => ({
  default: () => ({
    nextDirectoryId: ref<string | undefined>(undefined),
    resolveNextDirectory: vi.fn(async () => undefined),
  }),
}));

vi.mock("@/composables/useMarkImage", () => ({
  default: () => ({ marking: ref(false), mark: vi.fn() }),
}));

vi.mock("@/composables/useNotification", () => ({
  default: () => ({ show: vi.fn(() => 0), remove: vi.fn() }),
}));

vi.mock("@/composables/useHotkeys", () => ({ useHotkeys: vi.fn() }));

// 本用例不触发任何变更；避免加载真实 Apollo client 及其缓存恢复副作用
vi.mock("../graphql/utils/mutate", () => ({ default: vi.fn(async () => ({ data: undefined })) }));
vi.mock("idb-keyval", () => ({
  get: vi.fn(async () => undefined),
  set: vi.fn(async () => undefined),
}));

// 只保留 ImageViewer 的 image-loaded 上报接口，其余子组件与本用例关注点无关
vi.mock("../components/ImageViewer.vue", () => ({
  default: {
    name: "ImageViewer",
    props: ["image", "sessionId", "preloadImages", "allowPan", "hideImageButton"],
    emits: ["image-loaded"],
    template: '<div class="image-viewer-stub" />',
  },
}));
vi.mock("../components/SessionHeader.vue", () => ({
  default: { name: "SessionHeader", template: "<div />" },
}));
vi.mock("../components/SessionActions.vue", () => ({
  default: { name: "SessionActions", template: "<div />" },
}));
vi.mock("../components/SessionProgressBar.vue", () => ({
  default: { name: "SessionProgressBar", template: "<div />" },
}));
vi.mock("../components/CompletedView.vue", () => ({
  default: { name: "CompletedView", template: "<div />" },
}));
vi.mock("../components/CommitForm.vue", () => ({
  default: { name: "CommitForm", template: "<div />" },
}));
vi.mock("../components/UpdateSessionForm.vue", () => ({
  default: { name: "UpdateSessionForm", template: "<div />" },
}));
vi.mock("../components/SwipeDirectionIndicator.vue", () => ({
  default: { name: "SwipeDirectionIndicator", template: "<div />" },
}));

import SessionView from "./SessionView.vue";
import {
  ROUND_FLASH_FADE_IN_MS,
  ROUND_FLASH_FADE_OUT_MS,
  ROUND_FLASH_HOLD_MS,
} from "@/composables/useRoundChangeFlash";
// #endregion

// #region 测试数据工厂
function makeImage(id: string): ImageFragment {
  return {
    __typename: "Image",
    id,
    filename: `${id}.jpg`,
    url: `/image/${id}`,
    rawURL: `/raw/${id}`,
    url256: `/image/${id}`,
    url512: `/image/${id}`,
    url1024: `/image/${id}`,
    url2048: `/image/${id}`,
    url4096: `/image/${id}`,
    modTime: "2026-01-01T00:00:00.000Z",
    width: 100,
    height: 100,
    size: 1,
    currentRating: 0,
    label: null,
    relPath: `${id}.jpg`,
    note: {
      __typename: "Note",
      id: `note-${id}`,
      relPath: `${id}.txt`,
      content: "",
      rawContent: "",
      hidden: false,
      modTime: "2026-01-01T00:00:00.000Z",
    },
  };
}

function makeSession(options: { round: number; image: ImageFragment | null }): SessionFragment {
  return {
    __typename: "Session",
    id: "session-1",
    targetKeep: 1,
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
    canCommit: true,
    canUndo: false,
    currentIndex: 0,
    currentSize: 1,
    currentRound: options.round,
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
      totalKept: 0,
      totalShelved: 0,
      totalRejected: 0,
      currentRoundRemaining: 1,
      isCompleted: false,
    },
    currentImage: options.image,
    nextImages: [],
  };
}
// #endregion

// #region 挂载辅助
const wrappers: VueWrapper[] = [];

function mountSession(): VueWrapper {
  const wrapper = mount(SessionView, {
    props: { id: "session-1" },
    // 会话不存在分支里的 RouterLink 不参与本用例，模板编译期会解析该组件名，需显式占位
    global: { stubs: { RouterLink: true } },
  });
  wrappers.push(wrapper);
  return wrapper;
}

/** 模拟 ImageViewer 上报图片真正显示出来 */
function emitImageLoaded(wrapper: VueWrapper, id: string) {
  const viewer = wrapper.findComponent({ name: "ImageViewer" });
  if (!viewer.exists()) {
    throw new Error("ImageViewer 未渲染");
  }
  viewer.vm.$emit("image-loaded", { id, time: Time.now() });
}

function roundFlashAttribute(wrapper: VueWrapper) {
  return wrapper.get(".session-root").attributes("data-round-flash");
}
// #endregion

beforeEach(() => {
  vi.useFakeTimers();
  sessionRef().value = makeSession({ round: 1, image: makeImage("img-1") });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  sessionRef().value = undefined;
  vi.useRealTimers();
});

describe("SessionView 换轮提示接线", () => {
  test("恢复一个已进行到第 N 轮的会话时，首图加载不播放提示", () => {
    sessionRef().value = makeSession({ round: 3, image: makeImage("img-1") });

    const wrapper = mountSession();
    expect(roundFlashAttribute(wrapper)).toBe("idle");

    emitImageLoaded(wrapper, "img-1");
    expect(roundFlashAttribute(wrapper)).toBe("idle");
  });

  test("轮次增大后，旧图的加载事件不播放，新一轮首图加载后才播放并推进状态", async () => {
    const wrapper = mountSession();

    // 基线：第 1 轮首图已显示
    emitImageLoaded(wrapper, "img-1");
    expect(roundFlashAttribute(wrapper)).toBe("idle");

    // 本轮最后一张标记完成后后端进入第 2 轮，队列切到 img-2 但图片尚未加载
    sessionRef().value = makeSession({ round: 2, image: makeImage("img-2") });
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("idle");

    // 旧图迟到的加载事件不得提前播放提示
    emitImageLoaded(wrapper, "img-1");
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("idle");

    // 新一轮首图加载完成：开始淡入
    emitImageLoaded(wrapper, "img-2");
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("fade-in");

    vi.advanceTimersByTime(ROUND_FLASH_FADE_IN_MS);
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("hold");

    vi.advanceTimersByTime(ROUND_FLASH_HOLD_MS);
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("fade-out");

    vi.advanceTimersByTime(ROUND_FLASH_FADE_OUT_MS);
    await nextTick();
    expect(roundFlashAttribute(wrapper)).toBe("idle");
  });
});
