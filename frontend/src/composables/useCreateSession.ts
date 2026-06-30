import { ref } from "vue";
import mutate from "../graphql/utils/mutate";
import {
  CreateSessionDocument,
  SetDirectoryStateDocument,
  type ImageFiltersInput,
  type SessionFragment,
} from "../graphql/generated";
import { updateLastSession } from "./useDirectoryState";

export function useCreateSession() {
  const creating = ref(false);

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
    creating.value = true;
    try {
      const { data } = await mutate(CreateSessionDocument, {
        variables: {
          input: {
            directoryId: options.directoryId,
            filter: options.filter,
            targetKeep: options.targetKeep,
          },
        },
      });

      if (data?.createSession) {
        const session = data.createSession.session;
        updateLastSession(session);

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
          } catch (err) {
            console.error(
              `Failed to save createActions for ${options.directoryId}:`,
              err,
            );
          }
        }

        return session;
      }
    } finally {
      creating.value = false;
    }
    return null;
  }

  return {
    createSession,
    creating,
  };
}
