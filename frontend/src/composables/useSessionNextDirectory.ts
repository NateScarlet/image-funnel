import { computed, nextTick, toValue, type ComputedRef, type MaybeRefOrGetter } from "vue";
import useDirectoryBrowse, { useDirectoryStats } from "./domain/useDirectoryBrowse";
import {
  minUnratedCount,
  showCompletedDirectories,
  showSmallUnrated,
} from "./useDirectorySelectionFilters";
import {
  directoryCursorKey,
  setDirectoryCursor,
  type SessionDirectoryCursor,
} from "./useDirectoryCursor";
import useStorage from "./useStorage";
import {
  findNextDirectory,
  sortDirectoryOrder,
  type DirectoryOrderEntry,
  type DirectorySortKey,
} from "@/utils/directoryOrder";
import { evaluateDirectorySelectionVisibility } from "@/utils/directorySelection";
import type {
  BrowseDirectoriesQueryVariables,
  DirectoryFragment,
  DirectoryStatsFragment,
  SessionFragment,
} from "@/graphql/generated";
import useAsyncTask from "./useAsyncTask";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import ExactSearchMatcher from "@/utils/ExactSearchMatcher";
import basename from "@/utils/basename";

export type SessionDirectory = SessionFragment["directory"];

/** 带排序所需位置信息与筛选判定的目录条目 */
interface NextDirectoryEntry extends DirectoryOrderEntry {
  dir: DirectoryFragment;
  stats: DirectoryStatsFragment | undefined;
  /** 该目录所在页的 forward 请求锚点，用于写入目标目录的游标 */
  pageAfter: string | null;
  visibility: ReturnType<typeof evaluateDirectorySelectionVisibility>;
  matchesSearch: boolean;
}

interface NextDirectoryItem extends NextDirectoryEntry {
  sortKey: DirectorySortKey;
}

/**
 * 计算筛选完成后的「下一个目录」。
 *
 * 与创建筛选会话的列表共享同一条序列：页按服务端分页顺序拼接，页内按排序键排序。
 * 排序键以页锚点开头，因此对已加载目录全局排序一次即得到该顺序。
 * 当前位置来自进入时写入的游标（列表渲染该目录所用的排序键），会话页只读取、不重算。
 */
export default function useSessionNextDirectory(
  sessionId: MaybeRefOrGetter<string>,
  currentDirectory: MaybeRefOrGetter<SessionDirectory | undefined>,
) {
  const currentId = computed(() => toValue(currentDirectory)?.id ?? "");
  const parentId = computed(() => toValue(currentDirectory)?.parentId ?? "");

  // #region 游标与查询
  // 按目录 ID 响应式读取：目录切换时 useStorage 会重新读取对应键
  const { model: cursor } = useStorage<SessionDirectoryCursor>(sessionStorage, () =>
    directoryCursorKey(currentId.value),
  );

  // 没有游标时省略分页参数，等价于序列起点；搜索词随游标传递，保证页划分与列表一致
  const queryVariables = computed<BrowseDirectoriesQueryVariables | undefined>(() => {
    if (!parentId.value) return undefined;

    const variables: BrowseDirectoriesQueryVariables = { id: parentId.value };
    const currentCursor = cursor.value;
    if (currentCursor?.after) variables.after = currentCursor.after;
    if (currentCursor?.query) variables.filterBy = { query: currentCursor.query };
    return variables;
  });

  const {
    directories: loadedDirectories,
    hasNextPage,
    fetchMore,
    data: directoriesData,
    query,
  } = useDirectoryBrowse(queryVariables);
  const { getCachedStats, refetchStats } = useDirectoryStats();
  // #endregion

  // #region 排序与可见性
  // 与创建筛选会话的列表（DirectorySelector）共用同一套判定：
  // 筛选开关、强制显示与 basename 搜索一致，自动切换只额外排除已达标目录。
  // 候选来自已加载目录；所在页尚未加载的目录没有位置，不参与排序与导航。
  const orderedDirectories = computed<NextDirectoryItem[]>(() => {
    const visibilityInput = {
      showCompleted: showCompletedDirectories.value,
      showSmallUnrated: showSmallUnrated.value,
      minUnratedCount: minUnratedCount.value,
    };
    const matcher = new ExactSearchMatcher(cursor.value?.query ?? "");

    return sortDirectoryOrder<NextDirectoryEntry>(
      loadedDirectories.value.map(({ dir, pageAfter, pageAnchor, serverCursor }) => {
        const stats = getCachedStats(dir.id);
        const visibility = evaluateDirectorySelectionVisibility(dir, stats, visibilityInput);
        return {
          id: dir.id,
          dir,
          stats,
          pageAfter,
          pageAnchor,
          serverCursor,
          isForcedShown: visibility.isForcedShown,
          visibility,
          matchesSearch: matcher.match(basename(dir.relPath)),
        };
      }),
      "oldest-first",
    );
  });

  // 搜索态下列表忽略筛选开关、只保留搜索结果；非搜索态沿用筛选开关的可见性
  const visibleDirectories = computed<NextDirectoryItem[]>(() =>
    orderedDirectories.value.filter((entry) =>
      cursor.value?.query ? entry.matchesSearch : entry.visibility.isShown,
    ),
  );

  // 自动切换只进入未完成目录
  const uncompletedDirectories = computed<NextDirectoryItem[]>(() =>
    visibleDirectories.value.filter((entry) => !entry.visibility.completion.isCompleted),
  );

  /** 当前位置之后的第一个候选；没有游标时位置就是序列起点 */
  function firstCandidate(): NextDirectoryItem | undefined {
    const sortKey = cursor.value?.sortKey;
    if (!sortKey) return uncompletedDirectories.value[0];
    return findNextDirectory(uncompletedDirectories.value, sortKey);
  }

  const nextDirectory: ComputedRef<NextDirectoryItem | undefined> = computed(firstCandidate);

  const nextDirectoryId = computed(() => nextDirectory.value?.dir.id);
  // #endregion

  // #region 数据准备
  // 多次准备（挂载 + 提交）共用同一趟加载，避免重复分页与统计请求
  const prepareFlights = new SingleFlightGroup();

  /** 等待父目录当前页查询完成；查询失败时向上抛出，避免把失败误当成「没有下一个目录」 */
  function waitForDirectoriesLoaded(): Promise<void> {
    const current = query.getCurrentResult();
    if (current.data) return Promise.resolve();
    if (current.error) return Promise.reject(current.error);

    return new Promise((resolve, reject) => {
      const subscription = query.subscribe({
        next(result) {
          if (result.data) {
            subscription.unsubscribe();
            resolve();
          } else if (result.error) {
            subscription.unsubscribe();
            reject(result.error);
          }
        },
        error(err) {
          subscription.unsubscribe();
          reject(err);
        },
      });
    });
  }

  /** 补拉未知的目录统计信息；拉取失败时保持未知，按未完成处理 */
  async function loadUnknownStats(
    directoryIds: string[],
    abortSignal?: AbortSignal,
  ): Promise<void> {
    const unknownIds = directoryIds.filter((id) => !getCachedStats(id));
    if (unknownIds.length === 0) return;

    await refetchStats(unknownIds, abortSignal);
    // 等待 Apollo 缓存订阅把结果同步到 statsCache
    await nextTick();
  }

  async function prepare(
    abortSignal?: AbortSignal,
    targetDirectoryId: string = currentId.value,
  ): Promise<void> {
    const currentParentId = parentId.value;
    if (!currentParentId || !targetDirectoryId) return;

    await prepareFlights.do(
      `${toValue(sessionId)}:${currentParentId}:${targetDirectoryId}`,
      async (signal) => {
        await waitForDirectoriesLoaded();
        await nextTick();
        if (signal.aborted || !directoriesData.value) return;

        // 导航只需已加载目录的统计信息；缺失的目录按未完成处理
        await loadUnknownStats(
          loadedDirectories.value.map((entry) => entry.dir.id),
          signal,
        );
      },
      abortSignal,
    );
  }

  useAsyncTask({
    // 挂载与目录切换时准备数据；组件卸载或参数变化时中止尚未完成的准备
    args: () =>
      parentId.value ? [toValue(sessionId), parentId.value, currentId.value] : undefined,
    task: (_sessionId, _parentId, currentIdValue, ctx) => prepare(ctx.signal(), currentIdValue),
  });
  // #endregion

  /** 把目标目录的进入位置写入游标，供它自己的会话直接复用同一页与排序键 */
  function rememberEntry(entry: NextDirectoryItem): void {
    setDirectoryCursor(entry.dir.id, {
      after: entry.pageAfter,
      sortKey: entry.sortKey,
      query: cursor.value?.query ?? null,
    });
  }

  /**
   * 解析导航目标：先在已加载目录中按排序键推进；
   * 没有候选时用页尾游标只取下一页，继续寻找，直到找到或序列耗尽。
   */
  async function resolveNextDirectory(): Promise<string | undefined> {
    if (!parentId.value) return undefined;

    await prepare();

    const immediate = nextDirectory.value;
    if (immediate) {
      rememberEntry(immediate);
      return immediate.dir.id;
    }

    while (hasNextPage.value) {
      const loadedCount = loadedDirectories.value.length;
      await fetchMore("forward");
      await nextTick();
      // 分页无法推进时停止，避免无上限请求循环
      if (loadedDirectories.value.length <= loadedCount) return undefined;

      await loadUnknownStats(loadedDirectories.value.map((entry) => entry.dir.id));

      const found = firstCandidate();
      if (found) {
        rememberEntry(found);
        return found.dir.id;
      }
    }

    return undefined;
  }

  return {
    nextDirectory,
    nextDirectoryId,
    resolveNextDirectory,
  };
}
