import { toValue, type MaybeRefOrGetter } from "vue";
import useSession from "./domain/useSession";
import query from "@/graphql/utils/query";
import { DirectoryLastSessionDocument, DirectoryStateDocument } from "@/graphql/generated";
import { resolveLastConfiguration, type LastConfigurationContext } from "@/utils/lastConfiguration";

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
 * 根据目录的上次配置创建会话。
 *
 * 字段合并与回退全部委托给 resolveLastConfiguration：本 composable 只负责
 * 「目标目录刚确定时等待配置查询返回」的时序，以及把解析结果交给会话创建。
 * 场景差异由 context 判别式表达，不再有布尔开关。
 */
export function useAutoCreateSession(
  directoryId: MaybeRefOrGetter<string>,
  context: MaybeRefOrGetter<LastConfigurationContext>,
) {
  const { createSession } = useSession("");

  /**
   * 自动创建会话：等待目标目录配置查询返回，解析后调用 createSession
   */
  async function autoCreateSession() {
    const dirId = toValue(directoryId);
    if (!dirId) return undefined;

    const snapshot = await fetchDirectorySessionConfig(dirId);
    const { filter, targetKeep, writeActions } = resolveLastConfiguration(
      snapshot,
      toValue(context),
    );

    return await createSession({
      directoryId: dirId,
      filter,
      targetKeep,
      createActions: writeActions,
    });
  }

  return {
    autoCreateSession,
  };
}
// #endregion
