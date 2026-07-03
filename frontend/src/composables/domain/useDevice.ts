import { ref, computed, watch } from "vue";
import { once } from "es-toolkit";
import stableComputed from "@/composables/stableComputed";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useLiveConnection from "@/composables/useLiveConnection";
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

const init = once(() => {
  const pairingRequests = ref<{ code: string; createdAt: string }[]>([]);

  const { data: authData, refresh: refreshAuthStatus } = useQuery(AuthStatusDocument, {
    fetchPolicy: "network-only",
  });

  const { data: devicesData, refresh: refreshDevices } = useQuery(DevicesDocument, {
    fetchPolicy: "network-only",
  });

  const isTrustedDevice = computed(() => authData.value?.authStatus?.isTrustedDevice);
  const isTrustedIP = computed(() => authData.value?.authStatus?.isTrustedIP);
  const canManageDevices = stableComputed(() => {
    return isTrustedDevice.value === true || isTrustedIP.value === true;
  });

  const { data: pairingRequestsData, refresh: refreshPairingRequests } = useQuery(
    PairingRequestsDocument,
    {
      variables: () => (canManageDevices.value ? {} : undefined),
      fetchPolicy: "network-only",
    },
  );

  watch(
    () => pairingRequestsData.value?.pairingRequests,
    (val) => {
      if (val) {
        pairingRequests.value = [...val];
      }
    },
    { immediate: true },
  );

  const {
    nodes: liveDevices,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => devicesData.value?.devices ?? [], {
    identity: (d: DeviceFragment) => d.id,
    compare: (a: DeviceFragment, b: DeviceFragment) => {
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

  useSubscription(PairingRequestCreatedDocument, {
    onNext(result) {
      if (result.data?.pairingRequestCreated) {
        pairingRequests.value.push(result.data.pairingRequestCreated);
      }
    },
  });

  useSubscription(DeviceSavedDocument, {
    onNext(result) {
      const savedDevice = result.data?.deviceSaved;
      if (savedDevice) {
        client.writeFragment({
          id: client.cache.identify(savedDevice),
          fragment: DeviceFragmentDoc,
          fragmentName: "Device",
          data: savedDevice,
        });
        onSaved(savedDevice);
      }
    },
  });

  useSubscription(DeviceDeletedDocument, {
    onNext(result) {
      const deletedId = result.data?.deviceDeleted;
      if (deletedId) {
        onDeleted({ id: deletedId });
        client.cache.evict({
          id: client.cache.identify({ __typename: "Device", id: deletedId }),
        });
        client.cache.gc();
      }
    },
  });

  return {
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

export default function useDevice() {
  return init();
}
