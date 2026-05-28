import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import mutate from "@/graphql/utils/mutate";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import {
  BrowseMemosDocument,
  MemoSavedDocument,
  DirEntryDeletedDocument,
  UpdateMemoDocument,
  type MemoFragment,
  type BrowseMemosQueryVariables,
} from "@/graphql/generated";

/**
 * useBrowseMemos 提供目录备忘录（笔记）的查询、实时订阅、数据过滤和隐藏更新功能
 * @param variables 查询变量，支持响应式对象或其 Getter
 * @param options 可选配置，支持传入全局的 loadingCount
 */
export default function useBrowseMemos(
  variables: MaybeRefOrGetter<BrowseMemosQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  // 转换为计算属性，保证响应式更新
  const resolvedVariables = computed(() => toValue(variables));

  // 提取目录 ID，供实时订阅模块过滤使用
  const directoryId = computed(() => resolvedVariables.value.id);

  // 执行 GraphQL 查询获取备忘录列表
  const { data: memosData, query: memosQuery } = useQuery(BrowseMemosDocument, {
    variables: () => resolvedVariables.value,
    loadingCount: options?.loadingCount,
  });

  // 利用 useRelayConnection 管理分页拼接和 fetchMore
  const memosConnection = useRelayConnection(
    () =>
      memosData.value?.node?.__typename === "Directory"
        ? memosData.value.node.memos
        : undefined,
    () => memosQuery,
  );

  // 通过 useLiveConnection 接入实时增量更新与数据动态过滤
  const {
    nodes: memos,
    onSaved: onMemoSaved,
    onDeleted: onMemoDeleted,
  } = useLiveConnection(() => memosConnection.nodes.value, {
    filter: (m) => {
      // 检查变量中是否显式过滤隐藏的备忘录，若排除隐藏且当前备忘录被标记为隐藏，则进行过滤
      const currentVars = resolvedVariables.value;
      if (currentVars.filterBy?.hidden === false && m.hidden) {
        return false;
      }
      return true;
    },
    onNodeDidLeave: (m) => {
      // 当备忘录状态改变且不再符合过滤条件时，从列表中删除
      onMemoDeleted(m);
    },
  });

  // 订阅备忘录的新增与修改事件
  useSubscription(MemoSavedDocument, {
    variables: () => ({
      filterBy: directoryId.value
        ? { directoryId: [directoryId.value] }
        : undefined, // 避免返回 null，使用 undefined
    }),
    onNext: (result) => {
      const savedMemo = result.data?.memoSaved;
      if (savedMemo) {
        onMemoSaved(savedMemo);
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
        const match = memos.value.find(
          (m) => m.relPath === deletedEntry.relPath,
        );
        if (match) {
          onMemoDeleted({ id: match.id });
        }
      }
    },
  });

  // 解析并切换 frontmatter 中的隐藏状态值
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
        const newLines = lines.map((line) => {
          const trimmed = line.trim();
          if (trimmed === "" || trimmed.startsWith("#")) {
            return line;
          }
          const colonIndex = line.indexOf(":");
          if (colonIndex !== -1) {
            const key = line.slice(0, colonIndex).trim().toLowerCase();
            if (key === "hidden" || key === "hide") {
              found = true;
              const indent = line.slice(0, line.indexOf(line.trim()));
              return `${indent}${line.slice(0, colonIndex).trim()}: ${newHidden}`;
            }
          }
          return line;
        });

        if (!found) {
          if (
            newLines.length > 0 &&
            newLines[newLines.length - 1].trim() === ""
          ) {
            newLines[newLines.length - 1] = `hidden: ${newHidden}`;
            newLines.push("");
          } else {
            newLines.push(`hidden: ${newHidden}`);
          }
        }

        const newFrontmatter = newLines.join("\n");
        const result = `---\n${newFrontmatter}---\n${body}`;
        return isCRLF ? result.replace(/\n/g, "\r\n") : result;
      }
    }

    const newFrontmatter = `---\nhidden: ${newHidden}\n---\n`;
    const result = newFrontmatter + normalized;
    return isCRLF ? result.replace(/\n/g, "\r\n") : result;
  }

  // 切换备忘录/笔记的隐藏状态，并触发 Mutation 请求保存
  async function toggleMemoHidden(memoItem: MemoFragment) {
    const newHidden = !memoItem.hidden;
    const newRawContent = toggleFrontmatterHidden(
      memoItem.rawContent,
      newHidden,
    );

    try {
      await mutate(UpdateMemoDocument, {
        variables: {
          id: memoItem.id,
          content: newRawContent,
        },
      });
    } catch (err) {
      console.error("Failed to toggle memo hidden:", err);
    }
  }

  return {
    memos,
    toggleMemoHidden,
    hasNextPage: computed(() => memosConnection.pageInfo.value.hasNextPage),
    fetchMore: memosConnection.fetchMore,
  };
}
