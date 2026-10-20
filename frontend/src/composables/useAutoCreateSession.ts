import { toValue, type MaybeRefOrGetter } from "vue";
import useSession from "./domain/useSession";
import query from "@/graphql/utils/query";
import {
  DirectoryLastSessionDocument,
  DirectoryStateDocument,
  type ImageFiltersInput,
} from "@/graphql/generated";
import optionalArray from "@/utils/optionalArray";

// #region 目录配置读取
/**
 * 命令式读取目录的状态快照与上次会话配置。
 * 目标目录可能在调用前一刻才确定（如完成会话后自动切换），此时响应式查询尚未返回，
 * 不能依赖 useDirectoryState 的缓存，必须直接查询并等待结果。
 */
async function fetchDirectorySessionConfig(directoryId: string) {
  const [stateResult, lastSessionResult] = await Promise.all([
    query(DirectoryStateDocument, { variables: { id: directoryId } }),
    query(DirectoryLastSessionDocument, { variables: { id: directoryId } }),
  ]);

  const stateNode = stateResult.data?.node;
  const lastSessionNode = lastSessionResult.data?.node;
  const serverState = stateNode?.__typename === "Directory" ? stateNode.state : undefined;

  return {
    lastSession:
      lastSessionNode?.__typename === "Directory"
        ? (lastSessionNode.lastSession ?? undefined)
        : undefined,
    lastSessionState: serverState?.lastSession,
    defaultState: serverState?.default,
  };
}
// #endregion

// #region useAutoCreateSession Composable
/**
 * 根据目录的上次配置自动推导 filter/targetKeep/createActions 并创建会话。
 * 将 CompletedView 中「会话提交后自动切换下一目录」的创建逻辑提取为共享实现，
 * 同时供首页「开始新筛选」复用。
 *
 * 合并策略：以目录的 lastSession 快照为准，缺失项回退到传入的 fallback 值，
 * createActions 取目录 default.writeActions。
 */
export function useAutoCreateSession(
  directoryId: MaybeRefOrGetter<string>,
  fallbackFilter: MaybeRefOrGetter<ImageFiltersInput>,
  fallbackTargetKeep: MaybeRefOrGetter<number>,
  options?: { mergeAllFilterFields?: boolean },
) {
  const { createSession } = useSession("");

  /**
   * 自动创建会话：合并 lastSession 快照与 fallback 值，调用 createSession
   */
  async function autoCreateSession() {
    const dirId = toValue(directoryId);
    if (!dirId) return undefined;

    const { lastSession, lastSessionState, defaultState } =
      await fetchDirectorySessionConfig(dirId);
    const filter = toValue(fallbackFilter);
    const targetKeep = toValue(fallbackTargetKeep);
    const mergeAll = options?.mergeAllFilterFields !== false;

    // 以目录 active lastSession 或 state 中的 lastSession 快照为准，缺失项回退到传入的 fallback，应用最小参数原则（不透传空切片）
    const lastFilter = lastSession?.filter ?? lastSessionState?.filter;
    const finalFilter: ImageFiltersInput = {
      rating: optionalArray(lastFilter?.rating)?.slice() ?? optionalArray(filter.rating)?.slice(),
    };

    // CompletedView 仅合并 rating，首页合并所有 filter 字段
    if (mergeAll) {
      finalFilter.label =
        optionalArray(lastFilter?.label)?.slice() ?? optionalArray(filter.label)?.slice();
      finalFilter.query = lastFilter?.query || filter.query || undefined;
    }
    const finalTargetKeep = lastSession?.targetKeep ?? lastSessionState?.targetKeep ?? targetKeep;

    return await createSession({
      directoryId: dirId,
      filter: finalFilter,
      targetKeep: finalTargetKeep,
      createActions: defaultState?.writeActions ?? undefined,
    });
  }

  return {
    autoCreateSession,
  };
}
// #endregion
