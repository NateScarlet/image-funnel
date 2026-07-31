import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";

import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import {
  NoteSavedDocument,
  DirEntryDeletedDocument,
  UpdateNoteDocument,
  type NoteFragment,
  type BrowseNotesQueryVariables,
} from "@/graphql/generated";
import { throttle } from "es-toolkit";
import { useNoteBrowse } from "./domain/useNote";

export function toggleFrontmatterHidden(raw: string, newHidden: boolean): string {
  const isCRLF = raw.includes("\r\n");
  const normalized = raw.replace(/\r\n/g, "\n");

  if (normalized.startsWith("---\n")) {
    const parts = normalized.split("---\n");
    if (parts.length >= 3) {
      const frontmatter = parts[1];
      const body = parts.slice(2).join("---\n");

      const lines = frontmatter.split("\n");
      let found = false;
      const newLines: string[] = [];

      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed === "" || trimmed.startsWith("#")) {
          newLines.push(line);
          continue;
        }
        const colonIndex = line.indexOf(":");
        if (colonIndex !== -1) {
          const key = line.slice(0, colonIndex).trim().toLowerCase();
          if (key === "hidden" || key === "hide") {
            found = true;
            if (newHidden) {
              const indent = line.slice(0, line.indexOf(line.trim()));
              newLines.push(`${indent}${line.slice(0, colonIndex).trim()}: true`);
            }
            continue;
          }
        }
        newLines.push(line);
      }

      if (!found) {
        if (newHidden) {
          if (newLines.length > 0 && newLines[newLines.length - 1].trim() === "") {
            newLines[newLines.length - 1] = "hidden: true";
            newLines.push("");
          } else {
            newLines.push("hidden: true");
          }
        } else {
          return raw;
        }
      }

      if (newLines.every((l) => l.trim() === "")) {
        const result = body;
        return isCRLF ? result.replace(/\n/g, "\r\n") : result;
      }

      const newFrontmatter = newLines.join("\n");
      const result = `---\n${newFrontmatter}---\n${body}`;
      return isCRLF ? result.replace(/\n/g, "\r\n") : result;
    }
  }

  if (!newHidden) return raw;

  const newFrontmatter = `---\nhidden: true\n---\n`;
  const result = newFrontmatter + normalized;
  return isCRLF ? result.replace(/\n/g, "\r\n") : result;
}

export default function useBrowseNotes(
  variables: MaybeRefOrGetter<BrowseNotesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  const resolvedVariables = computed(() => toValue(variables));
  const directoryId = computed(() => resolvedVariables.value.id);

  const { data: notesData, query: notesQuery } = useNoteBrowse(variables, {
    loadingCount: options?.loadingCount,
  });

  const notesConnection = useRelayConnection(
    () =>
      notesData.value?.node?.__typename === "Directory" ? notesData.value.node.notes : undefined,
    () => notesQuery,
  );

  const {
    nodes: notes,
    onSaved: onNoteSaved,
    onDeleted: onNoteDeleted,
  } = useLiveConnection(() => notesConnection.nodes.value, {
    compare: (a: NoteFragment, b: NoteFragment) => {
      return new Date(b.modTime).getTime() - new Date(a.modTime).getTime();
    },
    filter: (n: NoteFragment) => {
      const currentVars = resolvedVariables.value;
      if (currentVars.filterBy?.hidden === false && n.hidden) {
        return false;
      }
      return true;
    },
    onNodeDidLeave: (n: NoteFragment) => {
      onNoteDeleted(n);
    },
  });

  useSubscription(NoteSavedDocument, {
    variables: () => ({
      filterBy: directoryId.value ? { directoryId: [directoryId.value] } : undefined,
    }),
    onNext: (result) => {
      const savedNote = result.data?.noteSaved;
      if (savedNote) {
        onNoteSaved(savedNote);
      }
    },
  });

  const pendingRelPathDeletion = new Set<string>();
  function doFlushRelPathDeletion() {
    if (pendingRelPathDeletion.size === 0) return;
    for (const note of notes.value) {
      if (pendingRelPathDeletion.has(note.relPath)) {
        onNoteDeleted(note);
      }
    }
    pendingRelPathDeletion.clear();
  }
  const flushRelPathDeletion = throttle(doFlushRelPathDeletion, 1e3, {
    edges: ["leading", "trailing"],
  });

  useSubscription(DirEntryDeletedDocument, {
    variables: () => ({ directoryId: directoryId.value }),
    onNext: (result) => {
      const deletedEntries = result.data?.dirEntryDeleted;
      if (deletedEntries && deletedEntries.length > 0) {
        for (const entry of deletedEntries) {
          pendingRelPathDeletion.add(entry.relPath);
        }
        flushRelPathDeletion();
      }
    },
  });

  async function toggleNoteHidden(noteItem: NoteFragment) {
    const newHidden = !noteItem.hidden;
    const newRawContent = toggleFrontmatterHidden(noteItem.rawContent, newHidden);

    await mutate(UpdateNoteDocument, {
      variables: {
        input: { id: noteItem.id, content: newRawContent },
      },
    });
  }

  return {
    notes,
    toggleNoteHidden,
    hasNextPage: computed(() => notesConnection.pageInfo.value.hasNextPage),
    fetchMore: notesConnection.fetchMore,
  };
}
