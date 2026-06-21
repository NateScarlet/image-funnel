import {
  TrashImagesDocument,
  type ImageFiltersInput,
} from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";
import useTrashHistory from "@/composables/useTrashHistory";

/**
 * useTrashImages 封装将图片移入暂存区（回收站）的通用逻辑：
 * 1. 调用 TrashImages mutation
 * 2. 刷新回收站历史
 * 3. 显示带撤销按钮的成功通知
 */
export default function useTrashImages() {
  const { show: showNotification } = useNotification();
  const { refresh: refreshTrashHistory, undo: undoTrash } = useTrashHistory();

  /**
   * 根据筛选条件将图片移入暂存区
   * @param directoryId 当前目录 ID
   * @param filterBy 筛选条件（如按 ID 列表、星级等）
   * @returns 移动数量和历史 ID
   */
  async function trashImages(directoryId: string, filterBy: ImageFiltersInput) {
    const result = await mutate(TrashImagesDocument, {
      variables: {
        input: {
          directoryId,
          filterBy,
        },
      },
    });

    const movedCount = result.data?.trashImages.movedCount ?? 0;
    const historyId = result.data?.trashImages.historyId;

    void refreshTrashHistory();

    showNotification(
      `成功将 ${movedCount} 张图片及其配套文件移到暂存区`,
      "success",
      10000,
      historyId
        ? {
            text: "撤销",
            onClick: (closeNotification) => {
              undoTrash(historyId);
              closeNotification();
            },
          }
        : undefined,
    );

    return { movedCount, historyId };
  }

  return { trashImages };
}
