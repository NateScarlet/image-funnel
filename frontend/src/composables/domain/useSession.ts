import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import {
  SessionDocument,
  SessionUpdatedDocument,
  CreateSessionDocument,
  SetDirectoryStateDocument,
  MarkImageDocument,
  type ImageFiltersInput,
  type ImageAction,
  type SessionFragment,
} from "@/graphql/generated";

export default function useSession(
  id: MaybeRefOrGetter<string | undefined>,
  options: { loadingCount?: Ref<number> } = {},
) {
  const resolvedId = computed(() => toValue(id));

  const { data } = useQuery(SessionDocument, {
    variables: () => (resolvedId.value ? { id: resolvedId.value } : undefined),
    loadingCount: options.loadingCount,
  });

  useSubscription(SessionUpdatedDocument, {
    variables: () => (resolvedId.value ? { id: resolvedId.value } : undefined),
  });

  const session = computed(() => data.value?.session);

  async function createSession(options: {
    directoryId: string;
    filter: ImageFiltersInput;
    targetKeep: number;
    createActions?: {
      keepRating: number | null;
      shelveRating: number | null;
      rejectRating: number | null;
    };
  }): Promise<SessionFragment | null> {
    const res = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          directoryId: options.directoryId,
          filter: options.filter,
          targetKeep: options.targetKeep,
        },
      },
    });

    if (res.data?.createSession) {
      const sess = res.data.createSession.session;

      if (options.createActions) {
        try {
          await mutate(SetDirectoryStateDocument, {
            variables: {
              input: {
                id: options.directoryId,
                state: {
                  default: {
                    writeActions: options.createActions,
                  },
                },
              },
            },
          });
        } catch {
          // 保存默认写操作失败不影响会话创建
        }
      }

      return sess;
    }
    return null;
  }

  async function markImage(
    imageId: string,
    action: ImageAction,
    duration?: string,
  ) {
    if (!resolvedId.value) return;
    await mutate(MarkImageDocument, {
      variables: {
        input: {
          sessionId: resolvedId.value,
          imageId,
          action,
          duration,
        },
      },
    });
  }

  return { session, data, createSession, markImage };
}
