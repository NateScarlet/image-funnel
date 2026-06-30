import {
  TrashHistoryDocument,
  UndoTrashDocument,
  EmptyTrashDocument,
} from "@/graphql/generated";
import useStorage from "@/composables/useStorage";
import useNotification from "@/composables/useNotification";
import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";

// 使用 useStorage 组合式函数来持久化并共享用户选择的保留期限 (minAge)
export const { model: trashMinAge, flush: saveMinAge } = useStorage<string>(
  localStorage,
  "trash_min_age_duration_t4g7k9",
  () => "P7D", // 默认保留 7 天 (P7D)
);

export default function useTrashHistory() {
  const { showSuccess, showError } = useNotification();

  // 查询回收站历史记录，支持 Relay 游标分页
  const { data, refresh } = useQuery(TrashHistoryDocument, {
    variables: () => ({ first: 100 }),
    fetchPolicy: "cache-and-network",
  });

  // 撤销删除，将图片放回原处
  async function undo(historyId: string) {
    try {
      const res = await mutate(UndoTrashDocument, {
        variables: {
          input: { historyId },
        },
      });
      if (res?.data?.undoTrash) {
        const { restoredCount, conflictCount, conflictDirName } =
          res.data.undoTrash;
        if (conflictCount > 0) {
          showSuccess(
            `成功还原了 ${restoredCount} 张图片，另有 ${conflictCount} 个文件存在冲突，已移入对应的 ${conflictDirName} 目录下，请手动处理`,
          );
        } else {
          showSuccess(`成功还原了 ${restoredCount} 张图片及其配套文件`);
        }
        void refresh();
      }
    } catch (err: unknown) {
      showError(
        err instanceof Error ? err.message : "还原失败，可能有文件冲突",
      );
    }
  }

  // 清空回收站
  async function empty() {
    try {
      const res = await mutate(EmptyTrashDocument, {
        variables: {
          minAge: trashMinAge.value,
        },
      });
      if (res?.data?.emptyTrash) {
        const clearedCount = res.data.emptyTrash.clearedCount;
        showSuccess(`已成功清理 ${clearedCount} 项历史图片及其伴随文件`);
        void refresh();
      }
    } catch (err: unknown) {
      showError(
        err instanceof Error ? err.message : "清理失败，可能有同名文件冲突",
      );
    }
  }

  return {
    data,
    refresh,
    undo,
    empty,
  };
}
