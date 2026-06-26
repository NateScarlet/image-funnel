import { computed, toValue, ref, type MaybeRefOrGetter } from "vue";
import useQuery from "../graphql/utils/useQuery";
import mutate from "../graphql/utils/mutate";
import {
  DirectoryLastSessionDocument,
  DirectoryStateDocument,
  SetDirectoryStateDocument,
  type ImageFiltersInput,
  type NoteFiltersInput,
  type SessionFragment,
  type DirectoryStateInput,
} from "../graphql/generated";
import optionalArray from "../utils/optionalArray";
import { debounce } from "es-toolkit";

// #region 类型定义
export interface DirectoryState {
  filterBy?: ImageFiltersInput;
  filterNoteBy?: NoteFiltersInput;
}
// #endregion

// 暂存当前激活目录的局部临时编辑（如点击星级等暂存值），以规避防抖网络同步期间的 UI 闪烁。
const currentBrowseBuffer = ref<{ id: string; browse: DirectoryState }>({
  id: "",
  browse: {},
});

const debouncers = new Map<string, ReturnType<typeof debounce>>();

/**
 * 更新上一次会话信息，由于由服务端进行会话生命周期托管，此接口为空实现
 */
export function updateLastSession(session: SessionFragment) {
  void session;
}

// #region 主钩子导出
export function useDirectoryState(directoryId: MaybeRefOrGetter<string>) {
  const dirIdRef = computed(() => toValue(directoryId));

  // 声明式获取/重置当前激活目录的暂存值，取代命令式的 watch 目录切换
  const currentBrowse = computed({
    get(): DirectoryState {
      return currentBrowseBuffer.value.id === dirIdRef.value
        ? currentBrowseBuffer.value.browse
        : {};
    },
    set(val: DirectoryState) {
      currentBrowseBuffer.value = {
        id: dirIdRef.value,
        browse: val,
      };
    },
  });

  // 异步查询服务端中当前目录的状态
  const { data: stateData } = useQuery(DirectoryStateDocument, {
    variables: () => {
      const id = dirIdRef.value;
      return id ? { id } : undefined;
    },
  });

  const serverState = computed(() => {
    const node = stateData.value?.node;
    return node?.__typename === "Directory" ? node.state : undefined;
  });

  const lastSessionState = computed(() => serverState.value?.lastSession);

  // 从服务器异步查询当前目录的最新会话信息
  const { data: lastSessionData } = useQuery(DirectoryLastSessionDocument, {
    variables: () => {
      const id = dirIdRef.value;
      return id ? { id } : undefined;
    },
  });

  const lastSession = computed(() => {
    const node = lastSessionData.value?.node;
    const activeSession =
      node?.__typename === "Directory" ? node.lastSession : undefined;
    if (activeSession) {
      return activeSession;
    }
    const stateSession = serverState.value?.lastSession;
    if (stateSession) {
      return {
        id: stateSession.id,
        filter: {
          id: "",
          directoryId: dirIdRef.value,
          rating: stateSession.filter.rating,
          label: stateSession.filter.label,
          query: stateSession.filter.query || undefined,
        },
        targetKeep: stateSession.targetKeep,
        updatedAt: serverState.value?.updatedAt || "",
      };
    }
    return undefined;
  });

  /**
   * 统一更新暂存状态
   */
  function updateDirectoryState(edit: (e: DirectoryState) => void) {
    const dirState = { ...currentBrowse.value };
    edit(dirState);
    compactDirectoryState(dirState);
    currentBrowse.value = dirState;
    triggerSaveState(dirIdRef.value);
  }

  /**
   * 整理暂存中的空对象
   */
  function compactDirectoryState(state: DirectoryState) {
    if (state.filterBy) {
      const hasFilter = Object.values(state.filterBy).some(
        (v) => v !== undefined,
      );
      if (!hasFilter) {
        state.filterBy = undefined;
      }
    }
  }

  /**
   * 触发防抖同步到服务端
   */
  function triggerSaveState(dirId: string) {
    let fn = debouncers.get(dirId);
    if (!fn) {
      fn = debounce(async (stateToSave: DirectoryState) => {
        const inputState: DirectoryStateInput = {
          browse:
            stateToSave.filterBy || stateToSave.filterNoteBy
              ? {
                  filterBy: stateToSave.filterBy
                    ? {
                        rating: stateToSave.filterBy.rating,
                        label: stateToSave.filterBy.label,
                        query: stateToSave.filterBy.query,
                      }
                    : undefined,
                  filterNoteBy: stateToSave.filterNoteBy
                    ? {
                        hidden: stateToSave.filterNoteBy.hidden,
                      }
                    : undefined,
                }
              : undefined,
        };

        try {
          await mutate(SetDirectoryStateDocument, {
            variables: {
              input: {
                id: dirId,
                state: inputState,
              },
            },
          });
        } catch (err) {
          console.error(
            `Failed to sync directory state to server for ${dirId}:`,
            err,
          );
        }
      }, 500);
      debouncers.set(dirId, fn);
    }
    fn(currentBrowse.value);
  }

  /**
   * 获取当前生效的图片筛选配置。
   * 1. 本地临时编辑暂存中的筛选（browse.filterBy，即使是空对象）优先级最高。
   * 2. 服务端查询到的最新配置优先级第二。
   * 3. 服务端查询到的最新会话 filter 优先级第三（回退）。
   */
  function getImageFilters(): ImageFiltersInput | undefined {
    const dirState = currentBrowse.value;
    if (dirState.filterBy) {
      return dirState.filterBy;
    }

    const sState = serverState.value;
    if (sState?.browse?.filterBy) {
      return {
        rating: optionalArray(sState.browse.filterBy.rating),
        label: optionalArray(sState.browse.filterBy.label),
        query: sState.browse.filterBy.query || undefined,
      };
    }

    const lastSessState = serverState.value?.lastSession;
    if (lastSessState) {
      const keepRating =
        lastSessState.commitActions?.keepRating ??
        lastSessState.createActions?.keepRating;
      const baseFilter = lastSessState.filter;
      return {
        rating:
          keepRating != null
            ? [keepRating]
            : baseFilter
              ? optionalArray(baseFilter.rating)
              : [],
        label: baseFilter ? optionalArray(baseFilter.label) : [],
        query: baseFilter?.query || undefined,
      };
    }

    const serverSess = lastSession.value;
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
      updateDirectoryState((state) => {
        state.filterBy = {
          rating: optionalArray(val),
          label: optionalArray(state.filterBy?.label),
          query: state.filterBy?.query || undefined,
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
      updateDirectoryState((state) => {
        state.filterBy = {
          rating: optionalArray(state.filterBy?.rating),
          label: optionalArray(val),
          query: state.filterBy?.query || undefined,
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
      updateDirectoryState((state) => {
        state.filterBy = {
          rating: optionalArray(state.filterBy?.rating),
          label: optionalArray(state.filterBy?.label),
          query: val || undefined,
        };
      });
    },
  });

  // 笔记是否显示隐藏项，默认值为 false
  const showHiddenNotes = computed<boolean>({
    get() {
      const dirState = currentBrowse.value;
      if (dirState.filterNoteBy) {
        return dirState.filterNoteBy.hidden === true;
      }
      const sState = serverState.value;
      if (sState?.browse?.filterNoteBy) {
        return sState.browse.filterNoteBy.hidden === true;
      }
      return false;
    },
    set(val) {
      updateDirectoryState((state) => {
        state.filterNoteBy = val ? { hidden: true } : undefined;
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
    updateDirectoryState((state) => {
      state.filterBy = {};
    });
  }

  return {
    filterRating,
    filterLabels,
    searchQuery,
    showHiddenNotes,
    hasActiveFilters,
    clearFilters,
    lastSession,
    lastSessionState,
  };
}
