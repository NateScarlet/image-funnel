import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import {
  BrowseNotesDocument,
  NoteSavedDocument,
  DirEntryDeletedDocument,
  UpdateNoteDocument,
  type NoteFragment,
  type BrowseNotesQueryVariables,
} from "@/graphql/generated";

/**
 * useBrowseNotes 提供目录笔记（笔记）的查询、实时订阅、数据过滤和隐藏更新功能
 * @param variables 查询变量，支持响应式对象或其 Getter
 * @param options 可选配置，支持传入全局的 loadingCount
 */
export default function useBrowseNotes(
  variables: MaybeRefOrGetter<BrowseNotesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  // 转换为计算属性，保证响应式更新
  const resolvedVariables = computed(() => toValue(variables));

  // 提取目录 ID，供实时订阅模块过滤使用
  const directoryId = computed(() => resolvedVariables.value.id);

  // 执行 GraphQL 查询获取笔记列表
  const { data: notesData, query: notesQuery } = useQuery(BrowseNotesDocument, {
    variables: () => resolvedVariables.value,
    loadingCount: options?.loadingCount,
  });

  // 利用 useRelayConnection 管理分页拼接和 fetchMore
  const notesConnection = useRelayConnection(
    () =>
      notesData.value?.node?.__typename === "Directory"
        ? notesData.value.node.notes
        : undefined,
    () => notesQuery,
  );

  // 通过 useLiveConnection 接入实时增量更新与数据动态过滤
  const {
    nodes: notes,
    onSaved: onNoteSaved,
    onDeleted: onNoteDeleted,
  } = useLiveConnection(() => notesConnection.nodes.value, {
    filter: (n) => {
      // 检查变量中是否显式过滤隐藏的笔记，若排除隐藏且当前笔记被标记为隐藏，则进行过滤
      const currentVars = resolvedVariables.value;
      if (currentVars.filterBy?.hidden === false && n.hidden) {
        return false;
      }
      return true;
    },
    onNodeDidLeave: (n) => {
      // 当笔记状态改变且不再符合过滤条件时，从列表中删除
      onNoteDeleted(n);
    },
  });

  // 订阅笔记的新增与修改事件
  useSubscription(NoteSavedDocument, {
    variables: () => ({
      filterBy: directoryId.value
        ? { directoryId: [directoryId.value] }
        : undefined, // 避免返回 null，使用 undefined
    }),
    onNext: (result) => {
      const savedNote = result.data?.noteSaved;
      if (savedNote) {
        onNoteSaved(savedNote);
      }
    },
  });

  // 订阅文件/目录的删除事件
  useSubscription(DirEntryDeletedDocument, {
    variables: () => {
      return { directoryId: directoryId.value || undefined };
    },
    onNext: (result) => {
      const deletedEntry = result.data?.dirEntryDeleted;
      if (deletedEntry) {
        const match = notes.value.find(
          (n) => n.relPath === deletedEntry.relPath,
        );
        if (match) {
          onNoteDeleted({ id: match.id });
        }
      }
    },
  });

  // 解析并切换 frontmatter 中的隐藏状态值
  // 取消隐藏时移除字段；若所有字段都移除则删除整个 frontmatter
  function toggleFrontmatterHidden(raw: string, newHidden: boolean): string {
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
                // 隐藏：更新字段值为 true
                const indent = line.slice(0, line.indexOf(line.trim()));
                newLines.push(
                  `${indent}${line.slice(0, colonIndex).trim()}: true`,
                );
              }
              // 取消隐藏：跳过该行，不添加
              continue;
            }
          }
          newLines.push(line);
        }

        if (!found) {
          if (newHidden) {
            // 不存在 hidden 字段且需要隐藏，添加字段
            if (
              newLines.length > 0 &&
              newLines[newLines.length - 1].trim() === ""
            ) {
              newLines[newLines.length - 1] = `hidden: true`;
              newLines.push("");
            } else {
              newLines.push(`hidden: true`);
            }
          } else {
            // 不存在 hidden 字段且不需要隐藏，无需修改
            return raw;
          }
        }

        // 所有行都为空时，移除整个 frontmatter
        if (newLines.every((l) => l.trim() === "")) {
          const result = body;
          return isCRLF ? result.replace(/\n/g, "\r\n") : result;
        }

        const newFrontmatter = newLines.join("\n");
        const result = `---\n${newFrontmatter}---\n${body}`;
        return isCRLF ? result.replace(/\n/g, "\r\n") : result;
      }
    }

    // 无 frontmatter：取消隐藏无需操作，隐藏则创建 frontmatter
    if (!newHidden) {
      return raw;
    }

    const newFrontmatter = `---\nhidden: true\n---\n`;
    const result = newFrontmatter + normalized;
    return isCRLF ? result.replace(/\n/g, "\r\n") : result;
  }

  // 切换笔记的隐藏状态，并触发 Mutation 请求保存
  async function toggleNoteHidden(noteItem: NoteFragment) {
    const newHidden = !noteItem.hidden;
    const newRawContent = toggleFrontmatterHidden(
      noteItem.rawContent,
      newHidden,
    );

    try {
      await mutate(UpdateNoteDocument, {
        variables: {
          id: noteItem.id,
          content: newRawContent,
        },
      });
    } catch (err) {
      console.error("Failed to toggle note hidden:", err);
    }
  }

  return {
    notes,
    toggleNoteHidden,
    hasNextPage: computed(() => notesConnection.pageInfo.value.hasNextPage),
    fetchMore: notesConnection.fetchMore,
  };
}
