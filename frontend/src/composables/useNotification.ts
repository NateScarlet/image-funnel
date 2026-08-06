import { ref } from "vue";

export type NotificationType = "error" | "success" | "info" | "warning";

export interface NotificationAction {
  text: string;
  onClick: (close: () => void) => void;
}

export interface Notification {
  id: number;
  type: NotificationType;
  message: string;
  body?: string;
  duration?: number;
  actions?: NotificationAction[];
  persistent?: boolean;
}

// #region 全局状态
const notifications = ref<Notification[]>([]);

let nextId = 0;
// #endregion

export default function useNotification() {
  // #region 显示通知
  function show(
    message: string,
    type: NotificationType = "info",
    duration = 3000,
    actions?: NotificationAction[],
    body?: string,
    persistent?: boolean,
  ) {
    const id = nextId;
    nextId++;
    const notification: Notification = {
      id,
      type,
      message,
      body,
      duration,
      actions,
      persistent,
    };

    notifications.value.push(notification);

    if (duration > 0) {
      setTimeout(() => {
        remove(id);
      }, duration);
    }

    return id;
  }

  function showError(message: string, duration = 5000, body?: string) {
    return show(message, "error", duration, undefined, body);
  }

  function showSuccess(message: string, duration = 3000, body?: string) {
    return show(message, "success", duration, undefined, body);
  }

  function showInfo(message: string, duration = 3000, body?: string) {
    return show(message, "info", duration, undefined, body);
  }

  function showWarning(message: string, duration = 3000, body?: string) {
    return show(message, "warning", duration, undefined, body);
  }
  // #endregion

  // #region 管理通知
  function remove(id: number) {
    const index = notifications.value.findIndex((n) => n.id === id);
    if (index !== -1) {
      notifications.value.splice(index, 1);
    }
  }

  function clear() {
    notifications.value = notifications.value.filter((n) => n.persistent);
  }
  // #endregion

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
