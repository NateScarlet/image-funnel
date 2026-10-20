import { computed, nextTick, shallowRef, toValue, watch, type MaybeRefOrGetter } from "vue";
import useDirectoryBrowse, { useDirectoryStats } from "./domain/useDirectoryBrowse";
import {
  minUnratedCount,
  showCompletedDirectories,
  showSmallUnrated,
} from "./useDirectorySelectionFilters";
import { getSessionCursor, setSessionCursor } from "./useDirectoryCursor";
import {
  createDirectoryCursor,
  findNextDirectory,
  sortDirectoryOrder,
  type DirectoryCursor,
} from "@/utils/directoryOrder";
import { evaluateDirectorySelectionVisibility } from "@/utils/directorySelection";
import type { SessionFragment } from "@/graphql/generated";
import useAsyncTask from "./useAsyncTask";
import SingleFlightGroup from "@/utils/SingleFlightGroup";

export type SessionDirectory = SessionFragment["directory"];

/**
 * 计算筛选完成后的「下一个目录」。
 *
 * 当前位置由进入操作时写入 sessionStorage 的游标锚定，而不是按当前状态扫描列表：
 * 当前目录被本次筛选改成完成后会从待筛选列表中消失，但游标仍然指向它原来的位置。
 */
export default function useSessionNextDirectory(
  sessionId: MaybeRefOrGetter<string>,
  currentDirectory: MaybeRefOrGetter<SessionDirectory | undefined>,
) {
  const currentId = computed(() => toValue(currentDirectory)?.id ?? "");
  const parentId = computed(() => toValue(currentDirectory)?.parentId ?? "");

  const {
    liveDirectories,
    hasNextPage,
    fetchMore,
    data: directoriesData,
    query,
  } = useDirectoryBrowse(() => (parentId.value ? { id: parentId.value } : undefined));
  const { getCachedStats, refetchStats } = useDirectoryStats();

  // #region 游标
  const cursor = shallowRef<DirectoryCursor | undefined>(getSessionCursor(toValue(sessionId)));

  watch(
    () => toValue(sessionId),
    (id) => {
      cursor.value = getSessionCursor(id);
    },
  );

  function generateCursor(targetSessionId: string): void {
    // 会话切换后旧准备任务可能仍在收尾，不能把旧位置写入新会话
    if (targetSessionId !== toValue(sessionId)) return;
    if (cursor.value) return;

    // 当前目录必须存在于筛选列表中才能取得排序键，否则视为没有下一个目录
    const currentEntry = visibleDirectories.value.find((entry) => entry.id === currentId.value);
    if (!currentEntry) return;

    const generated = createDirectoryCursor(currentEntry);
    cursor.value = generated;
    setSessionCursor(targetSessionId, generated);
  }
  // #endregion

  // #region 排序与完成度
  // 与创建会话的筛选列表（DirectorySelector）共用可见性与排序口径，
  // 自动切换只在此基础上再排除已达标目录。
  const orderedDirectories = computed(() => {
    const visibilityInput = {
      showCompleted: showCompletedDirectories.value,
      showSmallUnrated: showSmallUnrated.value,
      minUnratedCount: minUnratedCount.value,
    };

    return sortDirectoryOrder(
      liveDirectories.value.map((dir) => {
        const stats = getCachedStats(dir.id);
        const visibility = evaluateDirectorySelectionVisibility(dir, stats, visibilityInput);
        return { id: dir.id, dir, stats, isForcedShown: visibility.isForcedShown, visibility };
      }),
    );
  });

  const visibleDirectories = computed(() =>
    orderedDirectories.value.filter((entry) => entry.visibility.isShown),
  );

  // 自动切换只进入未完成目录；游标仍可从筛选列表中的已完成目录取得
  const uncompletedDirectories = computed(() =>
    visibleDirectories.value.filter((entry) => !entry.visibility.completion.isCompleted),
  );

  const nextDirectory = computed(() => {
    const currentCursor = cursor.value;
    if (!currentCursor) return undefined;

    return findNextDirectory(uncompletedDirectories.value, currentCursor, currentId.value);
  });

  const nextDirectoryId = computed(() => nextDirectory.value?.dir.id);
  // #endregion

  // #region 数据准备
  // 多次准备（挂载 + 提交）共用同一趟加载，避免重复分页与统计请求
  const prepareFlights = new SingleFlightGroup();

  /** 等待父目录首页查询完成；查询失败时向上抛出，避免把失败误当成「没有下一个目录」 */
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

  /** 分页加载直到满足条件；父目录可能超过一页，导航必须基于完整列表 */
  async function loadPagesUntil(
    isSatisfied: () => boolean,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await waitForDirectoriesLoaded();
    // 等待 useQuery 的订阅把结果同步到响应式 data
    await nextTick();
    if (abortSignal?.aborted || !directoriesData.value) return;

    while (!isSatisfied() && hasNextPage.value) {
      if (abortSignal?.aborted) return;

      const loadedCount = liveDirectories.value.length;
      await fetchMore();
      // 分页无法推进时停止，避免无上限请求循环
      if (abortSignal?.aborted || liveDirectories.value.length <= loadedCount) return;
    }
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

  /**
   * 迟到生成游标。只为当前目录补拉统计信息，尽快完成锚定，
   * 避免在全部统计加载完成前会话已被筛完而导致位置丢失。
   */
  async function ensureCursor(
    abortSignal: AbortSignal | undefined,
    targetSessionId: string,
  ): Promise<void> {
    if (cursor.value || !currentId.value) return;

    await loadPagesUntil(
      () => liveDirectories.value.some((dir) => dir.id === currentId.value),
      abortSignal,
    );
    if (abortSignal?.aborted) return;

    await loadUnknownStats([currentId.value], abortSignal);
    if (abortSignal?.aborted) return;

    generateCursor(targetSessionId);
  }

  async function prepare(
    abortSignal?: AbortSignal,
    targetSessionId: string = toValue(sessionId),
  ): Promise<void> {
    const currentParentId = parentId.value;
    if (!currentParentId) return;

    await prepareFlights.do(
      `${targetSessionId}:${currentParentId}:${currentId.value}`,
      async (signal) => {
        // 先锚定当前位置，再补齐候选目录所需的完整列表与统计信息
        await ensureCursor(signal, targetSessionId);
        if (signal.aborted) return;

        await loadPagesUntil(() => !hasNextPage.value, signal);
        if (signal.aborted) return;

        await loadUnknownStats(
          liveDirectories.value.map((dir) => dir.id),
          signal,
        );
        generateCursor(targetSessionId);
      },
      abortSignal,
    );
  }

  useAsyncTask({
    // 挂载与目录切换时准备数据；组件卸载或参数变化时中止尚未完成的准备
    args: () =>
      parentId.value ? [toValue(sessionId), parentId.value, currentId.value] : undefined,
    task: (sessionIdValue, _parentId, _currentId, ctx) => prepare(ctx.signal(), sessionIdValue),
  });
  // #endregion

  /**
   * 解析导航目标。提交后调用，保证在决定跳转前完成分页与统计信息准备，
   * 避免因数据尚未加载而误判为没有下一个目录。
   */
  async function resolveNextDirectory(): Promise<string | undefined> {
    if (!parentId.value) return undefined;

    await prepare();

    return nextDirectoryId.value;
  }

  return {
    nextDirectory,
    nextDirectoryId,
    resolveNextDirectory,
  };
}
