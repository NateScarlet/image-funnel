// #region 导入与 Mock
import { describe, test, expect, vi, beforeEach } from "vitest";
import { ref, nextTick } from "vue";
import { mount, type VueWrapper } from "@vue/test-utils";
import TrashHistoryButton from "./TrashHistoryButton.vue";
import type { TrashHistoryQuery } from "@/graphql/generated";

type TrashHistoryItem = TrashHistoryQuery["trashHistory"]["nodes"][number];

const mockNodes = ref<TrashHistoryItem[]>([]);
const mockPageInfo = ref({
  __typename: "PageInfo",
  hasNextPage: false,
  hasPreviousPage: false,
  startCursor: null as string | null,
  endCursor: null as string | null,
});
const mockFetchMore = vi.fn();
const mockUndo = vi.fn();
const mockEmpty = vi.fn();
const mockShowSuccess = vi.fn();
const mockCurrentTime = ref(new Date("2026-08-15T00:00:00.000Z"));

vi.mock("@/composables/domain/useTrash", () => ({
  default: () => ({
    nodes: mockNodes,
    pageInfo: mockPageInfo,
    fetchMore: mockFetchMore,
    undo: mockUndo,
    empty: mockEmpty,
    trashImages: vi.fn(),
  }),
}));

vi.mock("@/composables/useNotification", () => ({
  default: () => ({ showSuccess: mockShowSuccess }),
}));

vi.mock("@/composables/useStorage", () => ({
  default: () => ({ model: ref("P7D"), flush: vi.fn() }),
}));

vi.mock("@/composables/useCurrentTime", () => ({
  default: () => ({ currentTime: mockCurrentTime, refreshOn: vi.fn() }),
}));

vi.mock("./AppDropdown.vue", () => ({
  default: {
    template: `
      <div class="mock-dropdown">
        <slot name="trigger" :isOpen="false" :toggle="() => undefined" />
        <slot name="content" :close="() => undefined" />
      </div>
    `,
  },
}));

vi.mock("@mdi/js", () => ({
  mdiDelete: "",
  mdiDeleteSweep: "",
  mdiFileImage: "",
  mdiLoading: "",
}));
// #endregion

function makeItem(overrides?: Partial<TrashHistoryItem>): TrashHistoryItem {
  return {
    __typename: "TrashHistoryItem",
    id: "item-1",
    totalFileSize: 1024,
    totalFileCount: 1,
    trashedAt: "2026-08-01T00:00:00.000Z",
    imageCount: 1,
    associatedFileCount: 0,
    srcRelPath: "some/dir",
    message: null,
    coverImage: null,
    ...overrides,
  };
}

function findCleanupButton(wrapper: VueWrapper) {
  const btn = wrapper.findAll("button").find((b) => b.text().includes("清理"));
  if (!btn) {
    throw new Error("清理按钮未找到");
  }
  return btn;
}

function scrollContainerToEnd(wrapper: VueWrapper) {
  const el = wrapper.find(".max-h-64").element as HTMLElement;
  Object.defineProperty(el, "scrollHeight", { value: 200, configurable: true });
  Object.defineProperty(el, "clientHeight", { value: 100, configurable: true });
  Object.defineProperty(el, "scrollTop", { value: 100, configurable: true });
  el.dispatchEvent(new Event("scroll"));
}

beforeEach(() => {
  mockNodes.value = [];
  mockPageInfo.value.hasNextPage = false;
  mockFetchMore.mockClear();
});

// #region 清理按钮状态与文案
describe("TrashHistoryButton", () => {
  test("无过期项且无下一页时按钮禁用且文案不带 ≥", () => {
    mockNodes.value = [makeItem({ trashedAt: "2026-08-10T00:00:00.000Z" })];
    mockPageInfo.value.hasNextPage = false;

    const wrapper = mount(TrashHistoryButton);
    const btn = findCleanupButton(wrapper);

    expect(btn.text()).toContain("清理 0 B");
    expect(btn.text()).not.toContain("≥");
    expect(btn.classes()).toContain("pointer-events-none");
  });

  test("无过期项但有下一页时按钮可点击且文案带 ≥", () => {
    mockNodes.value = [makeItem({ trashedAt: "2026-08-10T00:00:00.000Z" })];
    mockPageInfo.value.hasNextPage = true;

    const wrapper = mount(TrashHistoryButton);
    const btn = findCleanupButton(wrapper);

    expect(btn.text()).toContain("清理 ≥0 B");
    expect(btn.classes()).not.toContain("pointer-events-none");
  });

  test("有过期项且无下一页时按钮可点击且文案不带 ≥", () => {
    mockNodes.value = [makeItem({ trashedAt: "2026-08-01T00:00:00.000Z", totalFileSize: 1024 })];
    mockPageInfo.value.hasNextPage = false;

    const wrapper = mount(TrashHistoryButton);
    const btn = findCleanupButton(wrapper);

    expect(btn.text()).toContain("清理 1 KB");
    expect(btn.text()).not.toContain("≥");
    expect(btn.classes()).not.toContain("pointer-events-none");
  });
});
// #endregion

// #region 无限滚动加载
describe("TrashHistoryButton 无限滚动", () => {
  test("滚动到底部且有下一页时触发加载更多", async () => {
    mockNodes.value = [makeItem({ trashedAt: "2026-08-10T00:00:00.000Z" })];
    mockPageInfo.value.hasNextPage = true;

    const wrapper = mount(TrashHistoryButton);
    await nextTick();
    scrollContainerToEnd(wrapper);

    expect(mockFetchMore).toHaveBeenCalledTimes(1);
  });

  test("滚动到底部但无下一页时不触发加载更多", async () => {
    mockNodes.value = [makeItem({ trashedAt: "2026-08-10T00:00:00.000Z" })];
    mockPageInfo.value.hasNextPage = false;

    const wrapper = mount(TrashHistoryButton);
    await nextTick();
    scrollContainerToEnd(wrapper);

    expect(mockFetchMore).not.toHaveBeenCalled();
  });
});
// #endregion
