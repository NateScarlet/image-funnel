import { ref, computed, watch } from "vue";
import { once } from "es-toolkit";
import stableComputed from "@/composables/stableComputed";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useLiveConnection from "./useLiveConnection";
import client from "@/graphql/client";
import {
  DevicesDocument,
  PairingRequestCreatedDocument,
  AuthStatusDocument,
  PairingRequestsDocument,
  DeviceFragmentDoc,
  DeviceSavedDocument,
  DeviceDeletedDocument,
} from "@/graphql/generated";
import type { DeviceFragment } from "@/graphql/generated";

export type Device = DeviceFragment;

// 模块内部唯一的单次初始化逻辑，用于拉取基础数据和建立 WebSocket 订阅。
// 通过 once 缓存其返回值（包含共享的 Ref 与方法），避免使用模块级全局变量。
const init = once(() => {
  const visible = ref(false);
  const pairingRequests = ref<{ code: string; createdAt: string }[]>([]);

  // 查询当前登录认证状态
  const { data: authData, refresh: refreshAuthStatus } = useQuery(
    AuthStatusDocument,
    { fetchPolicy: "network-only" },
  );

  // 查询所有已连接/配对的设备列表
  const { data: devicesData, refresh: refreshDevices } = useQuery(
    DevicesDocument,
    { fetchPolicy: "network-only" },
  );

  // 当前客户端是否为受信任设备
  const isTrustedDevice = computed(
    () => authData.value?.authStatus?.isTrustedDevice,
  );
  // 当前客户端是否通过受信任 IP 访问
  const isTrustedIP = computed(() => authData.value?.authStatus?.isTrustedIP);

  // 计算当前客户端是否有设备管理权限（受信任设备或受信任IP访问）
  const canManageDevices = stableComputed(() => {
    return isTrustedDevice.value === true || isTrustedIP.value === true;
  });

  // 在当前客户端具备管理权限时，主动拉取已挂起的配对请求
  const { data: pairingRequestsData, refresh: refreshPairingRequests } =
    useQuery(PairingRequestsDocument, {
      variables: () => {
        if (!canManageDevices.value) {
          return undefined;
        }
        return {};
      },
      fetchPolicy: "network-only",
    });

  // 同步配对请求列表到局部变量
  watch(
    () => pairingRequestsData.value?.pairingRequests,
    (val) => {
      if (val) {
        pairingRequests.value = [...val];
      }
    },
    { immediate: true },
  );

  // 接入 live connection 机制进行实时缓存操作
  const {
    nodes: liveDevices,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => devicesData.value?.devices ?? [], {
    identity: (d) => d.id,
    compare: (a, b) => {
      return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
    },
    subscribe: (item, callback) => {
      const observable = client.watchFragment<DeviceFragment>({
        fragment: DeviceFragmentDoc,
        fragmentName: "Device",
        from: item,
      });
      const sub = observable.subscribe((result) => {
        if (result.complete && result.data) {
          callback(result.data);
        }
      });
      return () => sub.unsubscribe();
    },
  });

  // 监听新设备发起配对请求的 WebSocket 订阅
  useSubscription(PairingRequestCreatedDocument, {
    onNext(result) {
      if (result.data?.pairingRequestCreated) {
        pairingRequests.value.push(result.data.pairingRequestCreated);
        visible.value = true; // 收到配对请求时全局自动弹出抽屉展示
      }
    },
  });

  // 订阅设备保存事件
  useSubscription(DeviceSavedDocument, {
    onNext(result) {
      const savedDevice = result.data?.deviceSaved;
      if (savedDevice) {
        // 更新/写入缓存，以供 client.watchFragment 监测
        client.writeFragment({
          id: client.cache.identify(savedDevice),
          fragment: DeviceFragmentDoc,
          fragmentName: "Device",
          data: savedDevice,
        });
        // 加入或更新到列表
        onSaved(savedDevice);
      }
    },
  });

  // 订阅设备删除事件
  useSubscription(DeviceDeletedDocument, {
    onNext(result) {
      const deletedId = result.data?.deviceDeleted;
      if (deletedId) {
        // 从 live connection 中删除
        onDeleted({ id: deletedId });

        // 同时也尝试从 cache 中驱逐，避免内存泄露
        client.cache.evict({
          id: client.cache.identify({ __typename: "Device", id: deletedId }),
        });
        client.cache.gc();
      }
    },
  });

  return {
    visible,
    devices: liveDevices,
    pairingRequests,
    isTrustedDevice,
    refreshDevices,
    refreshAuthStatus,
    refreshPairingRequests,
    onSaved,
    onDeleted,
  };
});

/**
 * 在任意 Vue 组件中使用的设备管理器逻辑 Hook
 */
export function useDevices() {
  const {
    visible,
    devices,
    pairingRequests,
    isTrustedDevice,
    refreshDevices,
    refreshAuthStatus,
    refreshPairingRequests,
    onSaved,
    onDeleted,
  } = init();

  function open() {
    refreshDevices();
    refreshPairingRequests();
    visible.value = true;
  }

  function close() {
    visible.value = false;
  }

  return {
    visible,
    devices,
    pairingRequests,
    isTrustedDevice,
    refreshDevices,
    refreshAuthStatus,
    open,
    close,
    onSaved,
    onDeleted,
  };
}
