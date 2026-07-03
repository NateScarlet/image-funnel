import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFiltersInput } from "@/graphql/generated";
import useNotification from "@/composables/useNotification";
import { useHotkeys } from "@/composables/useHotkeys";
import useStorage from "@/composables/useStorage";
import useImage from "./domain/useImage";

interface LastHook {
  id: string;
  name: string;
}

// 全局记录上一次成功的动作，保证跨组件实例共享
const { model: lastDispatchedHook } = useStorage<LastHook>(
  localStorage,
  "last_dispatched_hook_f3a2b",
);

export interface UseImageHooksOptions {
  selectedFilterBy?: MaybeRefOrGetter<ImageFiltersInput | undefined>;
}

/**
 * useImageHooks 用于管理图片的外部脚本钩子派发与执行
 */
export default function useImageHooks(options: UseImageHooksOptions = {}) {
  const { dispatchableHooks, dispatch: domainDispatch, hooksLoadingCountRef } = useImage();

  const isDispatching = ref(false);
  const currentDispatchingHookId = ref("");
  const { showSuccess, showError, showInfo, remove } = useNotification();

  async function dispatch(hookId: string, hookName: string, filterBy: ImageFiltersInput) {
    if (isDispatching.value) return;
    isDispatching.value = true;
    currentDispatchingHookId.value = hookId;

    // 显示“正在执行”提示通知，并将自动关闭时间设为 0，防止自动关闭
    const infoNotificationId = showInfo(`正在执行动作 ${hookName}...`, 0);

    try {
      await domainDispatch(hookId, filterBy);
      lastDispatchedHook.value = { id: hookId, name: hookName };
      const ids = filterBy.id;
      if (ids && Array.isArray(ids)) {
        const count = ids.length;
        if (count === 1) {
          showSuccess(`动作 ${hookName} 已成功触发`);
        } else {
          showSuccess(`已成功对 ${count} 张图片触发动作 ${hookName}`);
        }
      } else {
        showSuccess(`已成功触发动作 ${hookName}`);
      }
    } finally {
      remove(infoNotificationId);
      isDispatching.value = false;
      currentDispatchingHookId.value = "";
    }
  }

  // 绑定 F4 快捷键重复上一次动作
  useHotkeys(
    {
      f4: async (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (!lastDispatchedHook.value) {
          showError("没有可以重复的上一个动作");
          return;
        }
        const filter = toValue(options.selectedFilterBy);
        if (!filter) {
          return;
        }
        await dispatch(lastDispatchedHook.value.id, lastDispatchedHook.value.name, filter);
      },
    },
    {
      description: "重复上一次动作",
      category: "图片操作",
      enabled: computed(() => {
        return !!toValue(options.selectedFilterBy);
      }),
    },
  );

  return {
    hooksLoadingCount: hooksLoadingCountRef,
    dispatchableHooks,
    isDispatching,
    currentDispatchingHookId,
    dispatch,
  };
}
