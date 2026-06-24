import { computed, toValue, type MaybeRefOrGetter } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import {
  NoteDocument,
  NoteSavedDocument,
  UpdateNoteDocument,
  type NoteQuery,
} from "@/graphql/generated";

/**
 * useNote 提供笔记的查询、实时订阅和更新功能
 * @param id 笔记 ID 或其 Getter
 */
export default function useNote(id: MaybeRefOrGetter<string | undefined>) {
  const { data } = useQuery(NoteDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { id: v };
    },
  });

  // 监听外部更新
  useSubscription(NoteSavedDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { filterBy: { id: [v] } };
    },
  });

  const note = computed(() => {
    const node = (data.value as NoteQuery | undefined)?.node;
    return node?.__typename === "Note" ? node : undefined;
  });

  const updateNote = async (content: string) => {
    const v = toValue(id);
    if (!v) return;

    return mutate(UpdateNoteDocument, {
      variables: {
        id: v,
        content,
      },
    });
  };

  return {
    note,
    updateNote,
  };
}
