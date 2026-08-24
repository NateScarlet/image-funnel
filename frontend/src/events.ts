import defineCustomEvent from "@/utils/defineCustomEvent";

export const openImageViewerByFilename = defineCustomEvent<{
  filename: string;
}>();

/** WebSocket 连接建立（含首次连接与重连），无载荷；供版本失配检测等连接时机逻辑订阅 */
export const websocketConnected = defineCustomEvent();
