import { computed, toValue, type MaybeRefOrGetter } from "vue";
import useDirectories from "./useDirectories";
import { useDirectoryStats } from "./domain/useDirectoryBrowse";
import useAsyncTask from "./useAsyncTask";
import SingleFlightGroup from "@/utils/SingleFlightGroup";
import { evaluateDirectoryCompletion } from "@/utils/directoryCompletion";
import { getNextDirectory } from "@/utils/nextDirectory";

// #region 自动切换场景下的下一个目录
/**
 * 解析自动切换场景下的「下一个目录」：同父目录中当前目录之后第一个未完成目录。
 *
 * 与浏览视图共用 {@link useDirectories} 给出的同级目录顺序，并复用
 * {@link evaluateDirectoryCompletion} 作为唯一完成度判定，不引入第二套口径。
 * 父目录存在后续分页时先加载完整列表，完成度未知的目录先补拉统计，补拉失败按未完成处理，
 * 保证结果与浏览视图一致且不因统计缺失而漏掉待筛选目录。
 */
export default function useDirectoryProgress(
  parentDirectoryId: MaybeRefOrGetter<string>,
  currentDirectoryId: MaybeRefOrGetter<string>,
) {
  const { liveDirectories, sortedDirectories, hasNextPage, fetchMore } = useDirectories(() => ({
    id: toValue(parentDirectoryId),
  }));
  const { getCachedStats, refetchStats } = useDirectoryStats();

  // 以父目录 id 为键单飞串行：并发加载会被 useRelayConnection 的重入保护直接跳过而空转
  const siblingsFlight = new SingleFlightGroup();

  /** 补全父目录的剩余分页与缺失统计，使顺序与完成判定都建立在完整数据上 */
  async function loadAllSiblings(signal?: AbortSignal): Promise<void> {
    await siblingsFlight.do(
      toValue(parentDirectoryId),
      async (flightSignal) => {
        while (!flightSignal.aborted && hasNextPage.value) {
          const loadedCount = liveDirectories.value.length;
          await fetchMore();
          // 已加载数量没有增长说明分页已无法推进，避免变成无上限的请求循环
          if (liveDirectories.value.length <= loadedCount) break;
        }

        // 顺序的排序键含图片数与最新修改时间，统计缺失时当前目录的位置与候选集合
        // 会随统计到达而漂移，因此先把统计补齐再判定
        const pendingStatsIds = liveDirectories.value
          .filter((directory) => !getCachedStats(directory.id))
          .map((directory) => directory.id);
        if (pendingStatsIds.length > 0 && !flightSignal.aborted) {
          await refetchStats(pendingStatsIds, flightSignal);
        }
      },
      signal,
    );
  }

  // 数据就绪前不产出预览目标，避免展示尚未验证的目录
  const { result: siblingsReady } = useAsyncTask({
    args: () => [toValue(parentDirectoryId), liveDirectories.value.length] as const,
    async task(_parentId, _loadedCount, ctx) {
      await loadAllSiblings(ctx.signal());
      return true;
    },
    defaultValue: () => false,
  });

  function scanNextDirectory(): string | undefined {
    const directoryById = new Map(
      liveDirectories.value.map((directory) => [directory.id, directory]),
    );
    return getNextDirectory(
      sortedDirectories.value.map((directory) => directory.id),
      toValue(currentDirectoryId),
      (directoryId) => {
        const directory = directoryById.get(directoryId);
        const stats = getCachedStats(directoryId);
        // 统计未知（含补拉失败）按未完成处理，不阻断自动切换
        if (!directory || !stats) return true;
        return !evaluateDirectoryCompletion(directory, stats).isCompleted;
      },
    );
  }

  /** 下一个待筛选目录的预览值，数据就绪后随统计变化更新 */
  const nextDirectoryId = computed(() => (siblingsReady.value ? scanNextDirectory() : undefined));

  /** 提交后跳转前调用：先补全分页与统计，再返回当前之后第一个未完成目录 */
  async function resolveNextDirectory(): Promise<string | undefined> {
    await loadAllSiblings();
    return scanNextDirectory();
  }

  return { nextDirectoryId, resolveNextDirectory };
}
// #endregion
