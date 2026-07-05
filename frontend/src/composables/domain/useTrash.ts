import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";
import {
  TrashImagesDocument,
  TrashHistoryDocument,
  UndoTrashDocument,
  EmptyTrashDocument,
  type ImageFiltersInput,
} from "@/graphql/generated";

export default function useTrash() {
  const { data, refresh } = useQuery(TrashHistoryDocument, {
    variables: () => ({ first: 100 }),
    fetchPolicy: "cache-and-network",
  });

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
      variables: { minAge },
    });
    if (res?.data?.emptyTrash) {
      void refresh();
      return res.data.emptyTrash;
    }
    return undefined;
  }

  return { data, refresh, undo, empty, trashImages };
}
