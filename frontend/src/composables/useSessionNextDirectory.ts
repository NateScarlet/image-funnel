import { computed, nextTick, ref, toValue, type ComputedRef, type MaybeRefOrGetter } from "vue";
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

  // #region 导航预览
  // 导航决策会刷新统计，统计更新会驱动预览重算：不冻结就会出现卡片消失/复现或目标来回变化，
  // 最终展示的预览也可能与实际跳转目标不一致。决策期间冻结预览，决策完成后一次性切到判定结果。
  //
  // 冻结值带上产生它的会话。SessionView 在路由参数变化时复用，同一实例跨会话存活：
  // 上一会话冻结到即将进入的目录后，该值对新会话必须不可见，否则新会话的完成页会把
  // 「刚完成的当前目录」当成下一个目录展示，并在决策窗口内保持旧目标后才跳到真实候选。
  const navigationPreview = ref<{ sessionId: string; id: string | undefined }>();

  const frozenPreview = computed(() => {
    const preview = navigationPreview.value;
    return preview !== undefined && preview.sessionId === toValue(sessionId) ? preview : undefined;
  });

  const nextDirectoryId = computed(() =>
    frozenPreview.value ? frozenPreview.value.id : nextDirectory.value?.dir.id,
  );
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

  /**
   * 加载目录统计。
   *
   * 导航决策必须基于最新统计（提交会改变目录的完成度），因此决策时刷新全部已加载目录；
   * 挂载与翻页只需要补齐缺失项。刷新结果返回时已同步进响应式缓存，消费者可以立即读取。
   * 刷新失败必须向上抛出：既不能被当成「这些目录未完成」，也不能被当成「没有下一个目录」。
   */
  async function loadStats(
    directoryIds: string[],
    abortSignal: AbortSignal | undefined,
    refreshAll: boolean,
  ): Promise<void> {
    const ids = refreshAll ? directoryIds : directoryIds.filter((id) => !getCachedStats(id));
    if (ids.length === 0) return;

    const failures = await refetchStats(ids, abortSignal);
    if (failures.length > 0) throw failures[0];
  }

  async function prepare(
    abortSignal?: AbortSignal,
    targetDirectoryId: string = currentId.value,
    refreshAllStats = false,
  ): Promise<void> {
    const currentParentId = parentId.value;
    if (!currentParentId || !targetDirectoryId) return;

    // 统计刷新范围不同必须分开：否则导航会复用到只补齐缺失项的挂载准备，读到提交前的统计
    const statsMode = refreshAllStats ? "all-stats" : "missing-stats";
    await prepareFlights.do(
      `${toValue(sessionId)}:${currentParentId}:${targetDirectoryId}:${statsMode}`,
      async (signal) => {
        await waitForDirectoriesLoaded();
        await nextTick();
        if (signal.aborted || !directoriesData.value) return;

        await loadStats(
          loadedDirectories.value.map((entry) => entry.dir.id),
          signal,
          refreshAllStats,
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
   * 解析导航目标。
   *
   * 决策前后预览冻结在同一次判定结果上：决策会刷新统计，统计更新会驱动预览重算，
   * 不冻结就会出现卡片消失/复现或目标来回变化，且最终展示的预览可能不是实际跳转目标。
   * 任何失败都必须向上抛出，空返回值只表示序列真正耗尽。
   */
  async function resolveNextDirectory(): Promise<string | undefined> {
    if (!parentId.value) return undefined;

    // 冻结与回写都记在发起决策的会话名下：决策期间若已切换到新会话，
    // 结果对新会话不可见，不会被误当成新会话的预览
    const scope = toValue(sessionId);

    // 冻结当前预览；重试时沿用上次冻结的目标，避免重试瞬间预览跳回旧候选
    navigationPreview.value = {
      sessionId: scope,
      id: frozenPreview.value?.id ?? nextDirectory.value?.dir.id,
    };

    try {
      const target = await decideNextDirectory();
      // 预览与决策使用同一次判定结果
      navigationPreview.value = { sessionId: scope, id: target };
      return target;
    } catch (error) {
      // 决策失败：解除冻结，让预览回到实时候选，用户可在提示后重试；
      // 只清除本会话的冻结，不误清其他会话决策窗口内已冻结的预览
      if (navigationPreview.value?.sessionId === scope) {
        navigationPreview.value = undefined;
      }
      throw error;
    }
  }

  /**
   * 判定导航目标：先在已加载目录中按排序键推进；
   * 没有候选时用页尾游标只取下一页，继续寻找，直到找到或序列耗尽。
   */
  async function decideNextDirectory(): Promise<string | undefined> {
    // 提交会改变目录统计，决策前按最新统计重新判定，而不是沿用挂载时的缓存
    await prepare(undefined, currentId.value, true);

    const immediate = nextDirectory.value;
    if (immediate) {
      rememberEntry(immediate);
      return immediate.dir.id;
    }

    while (hasNextPage.value) {
      const loadedCount = loadedDirectories.value.length;
      // 取页返回时合并结果已可见，之后读取的目录就是新页内容
      await fetchMore("forward");

      if (loadedDirectories.value.length <= loadedCount) {
        // 结果已可见却没有新增目录：服务端仍声称有下一页即为游标失效，
        // 必须作为真实错误暴露，不能解释成「没有下一个目录而回主页」
        if (hasNextPage.value) throw new Error("目录分页未能推进：已取页但没有新增目录");
        // 后续页不存在，序列到此耗尽
        break;
      }

      await loadStats(
        loadedDirectories.value.map((entry) => entry.dir.id),
        undefined,
        false,
      );

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
