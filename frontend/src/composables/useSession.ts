import { computed, type Ref, toValue, type MaybeRefOrGetter } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import { SessionDocument, SessionUpdatedDocument } from "@/graphql/generated";

export default function useSession(
  id: MaybeRefOrGetter<string>,
  options: { loadingCount?: Ref<number> } = {},
) {
  const { data } = useQuery(SessionDocument, {
    variables: () => ({ id: toValue(id) }),
    loadingCount: options.loadingCount,
  });

  useSubscription(SessionUpdatedDocument, {
    variables: () => ({ id: toValue(id) }),
  });

  const session = computed(() => data.value?.session);

  return {
    session,
    data,
  };
}
