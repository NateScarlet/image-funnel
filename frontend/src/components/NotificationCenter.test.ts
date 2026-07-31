// #region 导入与 Mock Setup
import { describe, test, expect, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { ref } from "vue";
import NotificationCenter from "./NotificationCenter.vue";

const mockChannels = ref<
  Array<{ channel: string; unreadCount: number; latestNotification: { title: string } | null }>
>([]);
const mockSelectedChannel = ref<string | null>(null);
const mockUnreadCount = ref(0);
const mockSelectedChannelUnreadCount = ref(0);
const mockChannelNotifications = ref<unknown[]>([]);
const mockSelectChannel = vi.fn();

const mockDrawerComponent = {
  template: '<div class="mock-drawer"><slot /></div>',
};

const mockBodyDialogComponent = {
  template: '<div class="mock-body-dialog"><slot /></div>',
};

vi.mock("@/composables/domain/useNotificationCenter", () => ({
  default: () => ({
    channels: mockChannels,
    unreadCount: mockUnreadCount,
    selectedChannel: mockSelectedChannel,
    channelNotifications: mockChannelNotifications,
    selectedChannelUnreadCount: mockSelectedChannelUnreadCount,
    drawer: {
      component: mockDrawerComponent,
      isOpen: ref(true),
      open: vi.fn(),
      close: vi.fn(),
    },
    bodyDialog: {
      component: mockBodyDialogComponent,
      isOpen: ref(false),
      open: vi.fn(),
      close: vi.fn(),
    },
    bodyDialogTitle: ref(""),
    bodyDialogBody: ref(""),
    selectChannel: mockSelectChannel,
  }),
}));

vi.mock("@mdi/js", () => ({
  mdiChevronLeft: "",
  mdiBellOutline: "",
  mdiOpenInNew: "",
  mdiClose: "",
}));
// #endregion

// #region 组件渲染测试
describe("NotificationCenter", () => {
  test("当频道有未读时，直接在列表项左侧渲染橙色未读计数徽章，右侧不渲染未读计数", () => {
    mockChannels.value = [
      {
        channel: "hooks",
        unreadCount: 5,
        latestNotification: { title: "Hook 执行失败" },
      },
    ];
    mockSelectedChannel.value = null;

    const wrapper = mount(NotificationCenter);

    const button = wrapper.find("button");
    expect(button.exists()).toBe(true);

    // 确认已无旧的圆点 indicator
    const dot = button.find(".rounded-full.h-2.w-2");
    expect(dot.exists()).toBe(false);

    // 检查左侧是否包含了带有未读数字 5 的徽章
    const badge = button.find(".bg-secondary-600");
    expect(badge.exists()).toBe(true);
    expect(badge.text()).toBe("5");

    // 确认频道名称所在行右侧不再有另外的未读计数徽章
    const badges = button.findAll(".bg-secondary-600");
    expect(badges.length).toBe(1);
  });

  test("当频道无未读时，左侧不渲染未读计数徽章", () => {
    mockChannels.value = [
      {
        channel: "system",
        unreadCount: 0,
        latestNotification: { title: "系统正常" },
      },
    ];
    mockSelectedChannel.value = null;

    const wrapper = mount(NotificationCenter);

    const button = wrapper.find("button");
    expect(button.exists()).toBe(true);

    const badge = button.find(".bg-secondary-600");
    expect(badge.exists()).toBe(false);
  });
});
// #endregion
