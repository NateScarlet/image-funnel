import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";
import { HooksDocument, DispatchImageHookDocument } from "@/graphql/generated";
import { useHotkeys } from "@/composables/useHotkeys";
import useStorage from "@/composables/useStorage";

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
  imageIds?: MaybeRefOrGetter<string[]>;
}

/**
 * useImageHooks 用于管理图片的外部脚本钩子派发与执行
 */
export default function useImageHooks(options: UseImageHooksOptions = {}) {
  const hooksLoadingCount = ref(0);
  const { data: hooksData } = useQuery(HooksDocument, {
    loadingCount: hooksLoadingCount,
  });

  const dispatchableHooks = computed(() => {
    return hooksData.value?.hooks.filter((h) => h.canDispatchByImage) || [];
  });

  const isDispatching = ref(false);
  const currentDispatchingHookId = ref("");
  const { showSuccess, showError, showInfo, remove } = useNotification();

  async function dispatch(
    hookId: string,
    hookName: string,
    imageIds: string[],
  ) {
    if (imageIds.length === 0 || isDispatching.value) return;
    isDispatching.value = true;
    currentDispatchingHookId.value = hookId;

    // 显示“正在执行”提示通知，并将自动关闭时间设为 0，防止自动关闭
    const infoNotificationId = showInfo(`正在执行动作 ${hookName}...`, 0);

    try {
      const { error } = await mutate(DispatchImageHookDocument, {
        variables: {
          input: {
            hookId,
            ids: imageIds,
          },
        },
      });

      if (error) {
        showError(`执行动作 ${hookName} 失败：${error.message}`);
      } else {
        lastDispatchedHook.value = { id: hookId, name: hookName };
        const count = imageIds.length;
        if (count === 1) {
          showSuccess(`动作 ${hookName} 已成功触发`);
        } else {
          showSuccess(`已成功对 ${count} 张图片触发动作 ${hookName}`);
        }
      }
    } catch (err) {
      showError(
        `执行动作 ${hookName} 出错：${
          err instanceof Error ? err.message : String(err)
        }`,
      );
    } finally {
      // 移除“正在执行”的通知，恢复派发状态
      remove(infoNotificationId);
      isDispatching.value = false;
      currentDispatchingHookId.value = "";
    }
  }

  const targetImageIds = computed(() => {
    if (options.imageIds) {
      return toValue(options.imageIds);
    }
    return [];
  });

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
        const ids = targetImageIds.value;
        if (ids.length === 0) {
          return;
        }
        await dispatch(
          lastDispatchedHook.value.id,
          lastDispatchedHook.value.name,
          ids,
        );
      },
    },
    {
      description: "重复上一次动作",
      category: "图片操作",
      enabled: computed(() => targetImageIds.value.length > 0),
    },
  );

  return {
    hooksLoadingCount,
    dispatchableHooks,
    isDispatching,
    currentDispatchingHookId,
    dispatch,
  };
}
