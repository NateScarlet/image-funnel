import { computed, toValue, type MaybeRefOrGetter } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import {
  MemoDocument,
  MemoSavedDocument,
  UpdateMemoDocument,
  type MemoQuery,
} from "@/graphql/generated";

/**
 * useMemo 提供备忘录的查询、实时订阅和更新功能
 * @param id 备忘录 ID 或其 Getter
 */
export default function useMemo(id: MaybeRefOrGetter<string | undefined>) {
  const { data } = useQuery(MemoDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { id: v };
    },
  });

  // 监听外部更新
  useSubscription(MemoSavedDocument, {
    variables: () => {
      const v = toValue(id);
      if (!v) return undefined;
      return { filterBy: { id: [v] } };
    },
  });

  const memo = computed(() => {
    const node = (data.value as MemoQuery | undefined)?.node;
    return node?.__typename === "Memo" ? node : undefined;
  });

  const updateMemo = async (content: string) => {
    const v = toValue(id);
    if (!v) return;

    return mutate(UpdateMemoDocument, {
      variables: {
        id: v,
        content,
      },
    });
  };

  return {
    memo,
    updateMemo,
  };
}
