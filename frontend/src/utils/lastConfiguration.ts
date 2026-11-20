import type {
  DirectoryFragment,
  DirectoryStateFragment,
  ImageFiltersInput,
} from "@/graphql/generated";
import optionalArray from "./optionalArray";

// #region 类型定义
// 快照的最小传输形状全部从生成类型派生，避免复刻与 GraphQL 同构的字段定义
type GeneratedSessionFilter = NonNullable<DirectoryFragment["lastSession"]>["filter"];
type LastSessionFilter = Partial<Pick<GeneratedSessionFilter, "rating" | "label" | "query">>;

/** 上次会话的字段来源：筛选条件与保留目标 */
interface LastSessionInfo {
  filter?: LastSessionFilter | null;
  targetKeep?: NonNullable<DirectoryFragment["lastSession"]>["targetKeep"];
}

/** 默认写入评分：与 GraphQL 快照及 createSession 入参一致，三个评分值均必须给出 */
export type LastConfigurationWriteActions = Omit<
  NonNullable<NonNullable<DirectoryStateFragment["default"]>["writeActions"]>,
  "__typename"
>;

/** 「上次配置」的全部服务端来源，均为普通数据，无 Vue 响应式依赖 */
export interface LastConfigurationSnapshot {
  /** 服务端仍在的 active 上次会话，优先于状态快照 */
  lastSession?: LastSessionInfo | null;
  /** 目录状态持久化的上次会话快照 */
  lastSessionState?: LastSessionInfo | null;
  /** 默认写入评分配置 */
  defaultState?: {
    writeActions?: LastConfigurationWriteActions | null;
  } | null;
}

/** 场景判别式：首页新建并全部字段、完成切换只并 rating 且带当前会话回退 */
export type LastConfigurationContext =
  | { kind: "home-new" }
  | { kind: "completion"; filter: ImageFiltersInput; targetKeep: number };

/** 已解析的上次配置：展示与建会话共享的单一结果 */
export interface LastConfiguration {
  filter: ImageFiltersInput;
  targetKeep: number;
  writeActions: LastConfigurationWriteActions | undefined;
}
// #endregion

/**
 * 将目录的服务端快照解析为一份「上次配置」。
 *
 * 字段来源、优先级与缺失回退只在此处定义：
 * active 上次会话优先于状态快照，两者皆无时回退到场景上下文；
 * 首页新建合并全部字段并使用空回退，完成切换只合并 rating 并使用当前会话回退；
 * writeActions 取自默认写入评分。展示路径的 keepRating 覆盖不属于此处职责。
 */
export function resolveLastConfiguration(
  snapshot: LastConfigurationSnapshot,
  context: LastConfigurationContext,
): LastConfiguration {
  const lastSession = snapshot.lastSession ?? snapshot.lastSessionState ?? undefined;
  const fallback = context.kind === "completion" ? context.filter : undefined;
  const fallbackTargetKeep = context.kind === "completion" ? context.targetKeep : 0;

  const filter: ImageFiltersInput = {
    rating:
      optionalArray(lastSession?.filter?.rating)?.slice() ??
      optionalArray(fallback?.rating)?.slice(),
  };

  // 完成切换只沿用当前会话的 rating，label/query 不参与合并
  if (context.kind === "home-new") {
    filter.label = optionalArray(lastSession?.filter?.label)?.slice();
    filter.query = lastSession?.filter?.query || undefined;
  }

  return {
    filter,
    targetKeep: lastSession?.targetKeep ?? fallbackTargetKeep,
    writeActions: snapshot.defaultState?.writeActions ?? undefined,
  };
}
