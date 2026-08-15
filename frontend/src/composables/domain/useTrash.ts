import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";
import useRelayConnection from "../useRelayConnection";
import {
  TrashImagesDocument,
  TrashHistoryDocument,
  UndoTrashDocument,
  EmptyTrashDocument,
  type ImageFiltersInput,
} from "@/graphql/generated";
import type { Ref } from "vue";

export default function useTrash(options?: { loadingCount?: Ref<number> }) {
  const { data, query, refresh } = useQuery(TrashHistoryDocument, {
    variables: () => ({ first: 100 }),
    fetchPolicy: "cache-and-network",
    loadingCount: options?.loadingCount,
  });

  // 基于 Relay 连接派生节点列表、分页信息与加载更多能力
  const connection = useRelayConnection(
    () => data.value?.trashHistory,
    () => query,
  );

  async function trashImages(directoryId: string, filterBy: ImageFiltersInput, message?: string) {
    const result = await mutate(TrashImagesDocument, {
      variables: {
        input: {
          directoryId,
          filterBy,
          message,
        },
      },
    });

    const movedCount = result.data?.trashImages.movedCount ?? 0;
    const historyId = result.data?.trashImages.historyId;

    if (result.data?.trashImages) {
      void refresh();
    }

    return { movedCount, historyId };
  }

  async function undo(historyId: string) {
    const res = await mutate(UndoTrashDocument, {
      variables: {
        input: { historyId },
      },
    });
    if (res?.data?.undoTrash) {
      void refresh();
      return res.data.undoTrash;
    }
    return undefined;
  }

  async function empty(minAge: string) {
    const res = await mutate(EmptyTrashDocument, {
      variables: { input: { minAge } },
    });
    if (res?.data?.emptyTrash) {
      void refresh();
      return res.data.emptyTrash;
    }
    return undefined;
  }

  return {
    nodes: connection.nodes,
    pageInfo: connection.pageInfo,
    fetchMore: connection.fetchMore,
    refresh,
    undo,
    empty,
    trashImages,
  };
}
