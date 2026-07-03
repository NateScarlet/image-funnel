import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import {
  NoteDocument,
  NoteSavedDocument,
  UpdateNoteDocument,
  BrowseNotesDocument,
} from "@/graphql/generated";
import type { BrowseNotesQueryVariables } from "@/graphql/generated";

export default function useNote(id: MaybeRefOrGetter<string | undefined>) {
  const { data } = useQuery(NoteDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { id: v };
    },
  });

  useSubscription(NoteSavedDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { filterBy: { id: [v] } };
    },
  });

  const note = computed(() => {
    const node = data.value?.node;
    return node?.__typename === "Note" ? node : undefined;
  });

  async function updateNote(content: string) {
    const v = toValue(id);
    if (!v) return;
    await mutate(UpdateNoteDocument, {
      variables: { id: v, content },
    });
  }

  return { note, updateNote };
}

export function useNoteBrowse(
  variables: MaybeRefOrGetter<BrowseNotesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  return useQuery(BrowseNotesDocument, {
    variables: () => toValue(variables),
    loadingCount: options?.loadingCount,
  });
}
