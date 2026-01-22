import { computed, type Ref, toValue, type MaybeRefOrGetter } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import {
  GetSessionDocument,
  SessionUpdatedDocument,
} from "@/graphql/generated";

export default function useSession(
  id: MaybeRefOrGetter<string>,
  options: { loadingCount?: Ref<number> } = {},
) {
  const { data } = useQuery(GetSessionDocument, {
    variables: () => ({ id: toValue(id) }),
    loadingCount: options.loadingCount,
  });

  useSubscription(SessionUpdatedDocument, {
    variables: () => ({ sessionId: toValue(id) }),
  });

  const session = computed(() => data.value?.session);

  return {
    session,
    data,
  };
}
