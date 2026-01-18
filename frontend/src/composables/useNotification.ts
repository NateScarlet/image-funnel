import { ref } from "vue";

export type NotificationType = "error" | "success" | "info" | "warning";

export interface Notification {
  id: string;
  type: NotificationType;
  message: string;
  duration?: number;
}

const notifications = ref<Notification[]>([]);

export default function useNotification() {
  function show(
    message: string,
    type: NotificationType = "info",
    duration = 3000,
  ) {
    const id = Date.now().toString();
    const notification: Notification = {
      id,
      type,
      message,
      duration,
    };

    notifications.value.push(notification);

    if (duration > 0) {
      setTimeout(() => {
        remove(id);
      }, duration);
    }

    return id;
  }

  function showError(message: string, duration = 5000) {
    return show(message, "error", duration);
  }

  function showSuccess(message: string, duration = 3000) {
    return show(message, "success", duration);
  }

  function showInfo(message: string, duration = 3000) {
    return show(message, "info", duration);
  }

  function showWarning(message: string, duration = 3000) {
    return show(message, "warning", duration);
  }

  function remove(id: string) {
    const index = notifications.value.findIndex((n) => n.id === id);
    if (index !== -1) {
      notifications.value.splice(index, 1);
    }
  }

  function clear() {
    notifications.value = [];
  }

  return {
    notifications,
    show,
    showError,
    showSuccess,
    showInfo,
    showWarning,
    remove,
    clear,
  };
}
