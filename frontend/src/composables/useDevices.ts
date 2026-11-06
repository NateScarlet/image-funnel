import { ref } from "vue";
import useDevice from "./domain/useDevice";

const visible = ref(false);

export function useDevices() {
  const {
    pairingRequests,
    devices,
    devicesLoaded,
    isTrustedDevice,
    refreshDevices,
    refreshAuthStatus,
    refreshPairingRequests,
    onSaved,
    onDeleted,
  } = useDevice();

  function open() {
    void refreshDevices();
    void refreshPairingRequests();
    visible.value = true;
  }

  function close() {
    visible.value = false;
  }

  return {
    visible,
    devices,
    devicesLoaded,
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
