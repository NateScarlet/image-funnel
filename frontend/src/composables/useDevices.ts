import { ref } from "vue";
import useDevice from "./domain/useDevice";

export function useDevices() {
  const visible = ref(false);
  const {
    pairingRequests,
    devices,
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
