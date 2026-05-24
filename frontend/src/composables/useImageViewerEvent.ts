import defineCustomEvent from "@/utils/defineCustomEvent";

/** 打开图片查看器的事件，memoId 优先匹配，回退使用 filename */
export const openImageViewerByMemoIdEvent = defineCustomEvent<{
  memoId: string;
  filename: string;
}>();
