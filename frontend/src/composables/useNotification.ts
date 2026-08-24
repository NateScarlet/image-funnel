import { ref } from "vue";

export type NotificationType = "error" | "success" | "info" | "warning";

// 前端 toast 与后端通知解耦：由注入方（如 useNotificationCenter）提供抽象行为，
// UI 只负责渲染并触发 controller，不感知通知 ID、URL 等后端数据
export interface NotificationController {
  /** 用户主动关闭 toast 时的业务操作（如持久化服务端关闭状态） */
  dismiss?: () => void;
  /** 打开详情（如 window.open），存在则 UI 渲染"查看详情"按钮 */
  openDetails?: () => void;
  /** 覆盖 openDetails 动作按钮的文案（默认"查看详情"），用于语义不同的动作（如"立即刷新"） */
  openDetailsText?: string;
}

export interface Notification {
  id: number;
  type: NotificationType;
  message: string;
  body?: string;
  duration?: number;
  persistent?: boolean;
  controller?: NotificationController;
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
    body?: string,
    persistent?: boolean,
    controller?: NotificationController,
  ) {
    const id = nextId;
    nextId++;
    const notification: Notification = {
      id,
      type,
      message,
      body,
      duration,
      persistent,
      controller,
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
    return show(message, "error", duration, body);
  }

  function showSuccess(message: string, duration = 3000, body?: string) {
    return show(message, "success", duration, body);
  }

  function showInfo(message: string, duration = 3000, body?: string) {
    return show(message, "info", duration, body);
  }

  function showWarning(message: string, duration = 3000, body?: string) {
    return show(message, "warning", duration, body);
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
