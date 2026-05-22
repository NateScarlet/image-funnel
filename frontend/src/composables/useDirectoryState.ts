import { computed, toValue, ref, type MaybeRefOrGetter } from "vue";
import useStorage from "./useStorage";
import useQuery from "../graphql/utils/useQuery";
import {
  DirectoryLastSessionDocument,
  SessionDocument,
  type ImageFiltersInput,
  type MemoFiltersInput,
  type SessionFragment,
} from "../graphql/generated";
import optionalArray from "../utils/optionalArray";

// #region 类型定义
export interface DirectoryState {
  browse?: {
    filterBy?: ImageFiltersInput;
    filterMemoBy?: MemoFiltersInput;
  };
  lastSession?: {
    id: string;
    filter: ImageFiltersInput;
    targetKeep: number;
  };
  updatedAt: number;
}
// #endregion

const MAX_STATES_COUNT = 50;

// 共享的 LocalStorage 状态模型
export const { model: states, flush: commitState } = useStorage<
  Record<string, DirectoryState | undefined>
>(localStorage, "directory_state_f6857b6e8ad4", () => ({}));

// #region 内部辅助方法
/**
 * 统一更新目录状态
 */
function updateDirectoryState(
  dirId: string,
  edit: (e: DirectoryState) => void,
) {
  const currentStates = states.value || {};
  const dirState = currentStates[dirId] || {
    updatedAt: Date.now(),
  };

  // 执行调用方提供的修改逻辑
  edit(dirState);

  // 统一整理与压缩空对象状态
  compactDirectoryState(dirState);

  dirState.updatedAt = Date.now();
  currentStates[dirId] = dirState;

  // 若无 browse 且无 lastSession，则彻底移除该目录的记录
  if (!dirState.browse && !dirState.lastSession) {
    currentStates[dirId] = undefined;
  }

  // 限制保留状态的上限，保持存储空间精简
  const entries = Object.entries(currentStates).filter(
    ([, v]) => v !== undefined,
  ) as [string, DirectoryState][];
  if (entries.length > MAX_STATES_COUNT) {
    entries.sort((a, b) => b[1].updatedAt - a[1].updatedAt);
    for (let i = MAX_STATES_COUNT; i < entries.length; i++) {
      currentStates[entries[i][0]] = undefined;
    }
  }

  commitState();
}

/**
 * 整理目录状态中的空对象，节省 LocalStorage 空间
 */
function compactDirectoryState(state: DirectoryState) {
  if (state.browse) {
    if (state.browse.filterBy) {
      // 检查 filterBy 对象的属性值中是否存在有效筛选值
      const hasFilter = Object.values(state.browse.filterBy).some(
        (v) => v !== undefined,
      );
      if (!hasFilter) {
        // 无有效值时，如果有 lastSession，保留空对象以防止回退到上次会话的默认值；
        // 若无 lastSession，则彻底置为 undefined
        state.browse.filterBy = state.lastSession ? {} : undefined;
      }
    }

    // 若图片筛选与备忘录筛选均空，则清除整个 browse 对象
    if (
      state.browse.filterBy === undefined &&
      state.browse.filterMemoBy === undefined
    ) {
      state.browse = undefined;
    }
  }
}

/**
 * 更新上一次会话信息，对外公开
 */
export function updateLastSession(session: SessionFragment) {
  const dirId = session.directory.id;
  updateDirectoryState(dirId, (state) => {
    const ratingVal = optionalArray(session.filter?.rating);
    const labelVal = optionalArray(session.filter?.label);
    const queryVal = session.filter?.query || undefined;

    state.lastSession = {
      id: session.id,
      filter: {
        rating: ratingVal,
        label: labelVal,
        query: queryVal,
      },
      targetKeep: session.targetKeep,
    };

    // 创建或更新会话时，清除本地图片筛选缓存，使其能够继承最新会话配置
    if (state.browse) {
      state.browse.filterBy = undefined;
      if (state.browse.filterMemoBy === undefined) {
        state.browse = undefined;
      }
    }
  });
}
// #endregion

// #region 主钩子导出
export function useDirectoryState(directoryId: MaybeRefOrGetter<string>) {
  function getDirState() {
    const dirId = toValue(directoryId);
    return states.value?.[dirId];
  }

  /**
   * 获取当前生效的图片筛选配置。
   * 1. 本地临时编辑中的筛选（browse.filterBy，即使是空对象）优先级最高。
   * 2. 本地记录的上次会话 filter 优先级第二。
   * 3. 服务端查询到的最新会话 filter 优先级第三（多一级回退）。
   */
  function getImageFilters(): ImageFiltersInput | undefined {
    const dirState = getDirState();
    if (dirState?.browse?.filterBy) {
      return dirState.browse.filterBy;
    }
    if (dirState?.lastSession?.filter) {
      return dirState.lastSession.filter;
    }

    const serverSess = serverLastSession.value;
    if (serverSess?.filter) {
      return {
        rating: optionalArray(serverSess.filter.rating),
        label: optionalArray(serverSess.filter.label),
        query: serverSess.filter.query || undefined,
      };
    }

    return undefined;
  }

  // 评星过滤器，默认使用上次会话设置（若未修改过任何筛选条件）
  const filterRating = computed<number[]>({
    get() {
      return getImageFilters()?.rating || [];
    },
    set(val) {
      updateDirectoryState(toValue(directoryId), (state) => {
        if (!state.browse) {
          state.browse = {};
        }
        state.browse.filterBy = {
          rating: optionalArray(val),
          label: optionalArray(state.browse.filterBy?.label),
          query: state.browse.filterBy?.query || undefined,
        };
      });
    },
  });

  // 颜色标签过滤器，默认使用上次会话设置（若未修改过任何筛选条件）
  const filterLabels = computed<string[]>({
    get() {
      return getImageFilters()?.label || [];
    },
    set(val) {
      updateDirectoryState(toValue(directoryId), (state) => {
        if (!state.browse) {
          state.browse = {};
        }
        state.browse.filterBy = {
          rating: optionalArray(state.browse.filterBy?.rating),
          label: optionalArray(val),
          query: state.browse.filterBy?.query || undefined,
        };
      });
    },
  });

  // 搜索关键字，默认使用上次会话设置（若未修改过任何筛选条件）
  const searchQuery = computed<string>({
    get() {
      return getImageFilters()?.query || "";
    },
    set(val) {
      updateDirectoryState(toValue(directoryId), (state) => {
        if (!state.browse) {
          state.browse = {};
        }
        state.browse.filterBy = {
          rating: optionalArray(state.browse.filterBy?.rating),
          label: optionalArray(state.browse.filterBy?.label),
          query: val || undefined,
        };
      });
    },
  });

  // 备忘录是否显示隐藏项，默认值为 false
  const showHiddenMemos = computed<boolean>({
    get() {
      const dirState = getDirState();
      return dirState?.browse?.filterMemoBy?.hidden === true;
    },
    set(val) {
      updateDirectoryState(toValue(directoryId), (state) => {
        if (val) {
          if (!state.browse) {
            state.browse = {};
          }
          state.browse.filterMemoBy = { hidden: true };
        } else {
          if (state.browse) {
            state.browse.filterMemoBy = undefined;
          }
        }
      });
    },
  });

  const hasActiveFilters = computed(() => {
    return (
      filterRating.value.length > 0 ||
      filterLabels.value.length > 0 ||
      searchQuery.value.trim() !== ""
    );
  });

  function clearFilters() {
    updateDirectoryState(toValue(directoryId), (state) => {
      if (!state.browse) {
        state.browse = {};
      }
      state.browse.filterBy = {};
    });
  }

  // #region 上次会话校验与回退机制
  // 1. 本地会话的加载状态计数
  const localSessionLoadingCount = ref(0);

  // 2. 优先对本地存储的会话发起网络校验以确认其存在性
  const { data: localSessionCheckData } = useQuery(SessionDocument, {
    variables: () => {
      const id = getDirState()?.lastSession?.id;
      if (!id) {
        return undefined;
      }
      return { id };
    },
    loadingCount: localSessionLoadingCount,
  });

  // 3. 从服务器异步查询当前目录的最新会话信息（仅在本地无会话或本地会话已被删除时发起查询）
  const { data: lastSessionData } = useQuery(DirectoryLastSessionDocument, {
    variables: () => {
      const id = toValue(directoryId);
      if (!id) {
        return undefined;
      }
      // 如果处于本地查询校验中，则挂起服务端上次会话的查询
      if (localSessionLoadingCount.value > 0) {
        return undefined;
      }
      // 如果本地会话校验通过并存在，则不需要查询服务端上次会话
      if (localSessionCheckData.value?.session) {
        return undefined;
      }
      return { id };
    },
  });

  const serverLastSession = computed(() => {
    const node = lastSessionData.value?.node;
    return node?.__typename === "Directory" ? node.lastSession : undefined;
  });

  // 4. 校验通过的本地上次会话
  const verifiedLocalSession = computed(() => {
    const localSess = getDirState()?.lastSession;
    if (!localSess) {
      return undefined;
    }
    if (localSessionCheckData.value?.session?.id === localSess.id) {
      return localSessionCheckData.value.session;
    }
    return undefined;
  });

  // 5. 最终合并的上次会话，优先本地，回退服务端
  const lastSession = computed(() => {
    if (verifiedLocalSession.value) {
      return verifiedLocalSession.value;
    }
    return serverLastSession.value;
  });
  // #endregion

  return {
    filterRating,
    filterLabels,
    searchQuery,
    showHiddenMemos,
    hasActiveFilters,
    clearFilters,
    lastSession,
  };
}
// #endregion
