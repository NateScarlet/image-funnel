// #region 导入与 Mock
import { describe, test, expect, vi, beforeEach, afterEach } from "vitest";
import { h, ref } from "vue";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import type { DeviceFragment } from "@/graphql/generated";

// 测试通过替换依赖实现（composable / GraphQL 工具）来驱动组件，而非依赖真实网络与浏览器 API
const mockDevices = ref<DeviceFragment[]>([]);
const mockDevicesLoaded = ref(true);
const mockPairingRequests = ref<{ code: string; createdAt: string }[]>([]);
const mockIsTrustedDevice = ref<boolean | undefined>(undefined);
const mockRefreshDevices = vi.fn();
const mockRefreshAuthStatus = vi.fn();
const mockCloseGlobal = vi.fn();

vi.mock("@/composables/useDevices", () => ({
  useDevices: () => ({
    visible: ref(false),
    devices: mockDevices,
    devicesLoaded: mockDevicesLoaded,
    pairingRequests: mockPairingRequests,
    isTrustedDevice: mockIsTrustedDevice,
    refreshDevices: mockRefreshDevices,
    refreshAuthStatus: mockRefreshAuthStatus,
    close: mockCloseGlobal,
  }),
}));

const mockAuthenticate = vi.fn();
const mockPairingCode = ref("");
const mockAuthError = ref("");
const mockAuthLoading = ref(false);

vi.mock("@/composables/useAuthenticate", () => ({
  useAuthenticate: () => ({
    authenticate: mockAuthenticate,
    pairingCode: mockPairingCode,
    errorMsg: mockAuthError,
    isActionLoading: mockAuthLoading,
  }),
}));

// 抽屉本身与设备管理逻辑无关，mock 成直接渲染插槽的容器
vi.mock("@/composables/useModalDrawer", () => ({
  default: () => ({
    visible: ref(true),
    open: vi.fn(),
    close: vi.fn(),
    component: {
      name: "MockDrawer",
      setup(_props: unknown, { slots }: { slots: { default?: () => unknown } }) {
        return () => h("div", slots.default?.() as never);
      },
    },
  }),
}));

const mockMutate = vi.fn();
vi.mock("@/graphql/utils/mutate", () => ({
  default: (...args: unknown[]) => mockMutate(...args),
}));

const mockForgetTokens = vi.fn();
vi.mock("@/graphql/tokenManager", () => ({
  forgetTokens: () => mockForgetTokens(),
}));

import DeviceManager from "./DeviceManager.vue";
// #endregion

function makeDevice(overrides?: Partial<DeviceFragment>): DeviceFragment {
  return {
    __typename: "Device",
    id: "device-1",
    name: "Android (Firefox)",
    createdAt: "2026-08-01T00:00:00.000Z",
    lastLoginAt: "2026-08-01T00:00:00.000Z",
    lastLoginIp: "100.125.71.62",
    isCurrent: false,
    ...overrides,
  };
}

function findDeleteButtons(wrapper: VueWrapper) {
  return wrapper.findAll('button[title="删除设备"]');
}

beforeEach(() => {
  mockDevices.value = [];
  mockDevicesLoaded.value = true;
  mockPairingRequests.value = [];
  mockIsTrustedDevice.value = undefined;
  mockPairingCode.value = "";
  mockAuthError.value = "";
  mockAuthLoading.value = false;
  mockMutate.mockReset().mockResolvedValue({ data: {} });
  mockForgetTokens.mockReset();
  mockRefreshDevices.mockReset();
  mockRefreshAuthStatus.mockReset();
  vi.stubGlobal(
    "confirm",
    vi.fn(() => true),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// #region 注册当前设备的入口可见性
describe("DeviceManager 注册入口", () => {
  test("当前设备已在已配对列表中时不显示注册入口", () => {
    mockIsTrustedDevice.value = true;
    mockDevices.value = [makeDevice({ id: "device-current", isCurrent: true })];

    const wrapper = mount(DeviceManager);

    expect(wrapper.text()).not.toContain("注册当前设备");
  });

  test("当前设备不在列表中时显示注册入口", () => {
    mockIsTrustedDevice.value = false;
    mockDevices.value = [makeDevice({ id: "device-1" })];

    const wrapper = mount(DeviceManager);

    expect(wrapper.text()).toContain("注册当前设备");
  });

  test("会话仍被视为可信设备但当前设备已不在列表中时，显示注册入口并说明原因", () => {
    // 删除当前设备后，浏览器仍持有未过期的访问令牌，此时既没有当前设备也没有注册入口，用户无从下手
    mockIsTrustedDevice.value = true;
    mockDevices.value = [makeDevice({ id: "device-1" }), makeDevice({ id: "device-2" })];

    const wrapper = mount(DeviceManager);

    expect(wrapper.text()).toContain("注册当前设备");
    expect(wrapper.text()).toContain("已被删除");
  });

  test("注册产生配对请求时展示配对码", () => {
    mockIsTrustedDevice.value = false;
    mockPairingCode.value = "123456";

    const wrapper = mount(DeviceManager);

    expect(wrapper.text()).toContain("123456");
  });

  test("设备列表尚未加载完成时不显示注册入口", () => {
    // 列表加载中同样是空数组，此时不能断言当前设备未注册，否则每次打开抽屉都会闪一下注册入口
    mockIsTrustedDevice.value = true;
    mockDevicesLoaded.value = false;

    const wrapper = mount(DeviceManager);

    expect(wrapper.text()).not.toContain("注册当前设备");
  });
});
// #endregion

// #region 删除设备后的本机会话处理
describe("DeviceManager 删除设备", () => {
  test("删除当前设备后清除本机令牌", async () => {
    mockIsTrustedDevice.value = true;
    mockDevices.value = [makeDevice({ id: "device-current", isCurrent: true })];

    const wrapper = mount(DeviceManager);
    await findDeleteButtons(wrapper)[0].trigger("click");
    await flushPromises();

    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate.mock.calls[0][1]).toEqual({ variables: { input: { id: "device-current" } } });
    expect(mockForgetTokens).toHaveBeenCalledTimes(1);
  });

  test("删除其他设备时保留本机令牌", async () => {
    mockIsTrustedDevice.value = true;
    mockDevices.value = [
      makeDevice({ id: "device-current", isCurrent: true }),
      makeDevice({ id: "device-other" }),
    ];

    const wrapper = mount(DeviceManager);
    await findDeleteButtons(wrapper)[1].trigger("click");
    await flushPromises();

    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate.mock.calls[0][1]).toEqual({ variables: { input: { id: "device-other" } } });
    expect(mockForgetTokens).not.toHaveBeenCalled();
  });

  test("取消确认时既不删除设备也不清除本机令牌", async () => {
    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );
    mockIsTrustedDevice.value = true;
    mockDevices.value = [makeDevice({ id: "device-current", isCurrent: true })];

    const wrapper = mount(DeviceManager);
    await findDeleteButtons(wrapper)[0].trigger("click");
    await flushPromises();

    expect(mockMutate).not.toHaveBeenCalled();
    expect(mockForgetTokens).not.toHaveBeenCalled();
  });
});
// #endregion
