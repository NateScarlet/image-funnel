import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import Time from "@/utils/Time";

/**
 * 人类反应时间阈值（毫秒）。
 * 当新图片显示不足此时间时开始的手势，目标图修正为上一张图，避免误伤刚切换的新图。
 */
export const GESTURE_REACTION_TIME_MS = 100;

export interface ImageLoadedEvent {
  id: string;
  time: Time;
}

export interface GestureStartEvent {
  time: Time;
  imageId: string;
}

/**
 * 手势目标图选择 Composable
 *
 * @param currentImageIdGetter 当前会话中最新的图片 ID
 * @param reactionTimeMs 人类反应时间阈值（毫秒），默认 100ms
 */
export default function useGestureTarget(
  currentImageIdGetter: MaybeRefOrGetter<string | null | undefined>,
  reactionTimeMs: number = GESTURE_REACTION_TIME_MS,
) {
  const currentLoaded = ref<ImageLoadedEvent>();
  const previousLoaded = ref<ImageLoadedEvent>();
  const gestureStartEvent = ref<GestureStartEvent>();

  // #region 记录图片加载事件
  function handleImageLoaded(event: ImageLoadedEvent | null | undefined) {
    if (!event) return;

    if (currentLoaded.value?.id === event.id) {
      // 同一张图重复加载，更新加载时间戳
      currentLoaded.value = event;
      return;
    }

    if (previousLoaded.value?.id === event.id) {
      // 撤销（Undo）退回到上一张图时，清理 previousLoaded，避免目标图指向已被撤销的图片
      currentLoaded.value = event;
      previousLoaded.value = undefined;
      return;
    }

    // 正常推进：原当前图移入 previousLoaded，新图设为 currentLoaded
    previousLoaded.value = currentLoaded.value;
    currentLoaded.value = event;
  }

  function recordGestureStart() {
    const currentId = toValue(currentImageIdGetter);
    if (!currentId) {
      gestureStartEvent.value = undefined;
      return;
    }
    gestureStartEvent.value = {
      time: Time.now(),
      imageId: currentId,
    };
  }

  const gestureTargetId = computed(() => {
    const startEvent = gestureStartEvent.value;
    if (!startEvent) {
      return undefined;
    }

    const loaded = currentLoaded.value;
    if (!loaded) {
      return startEvent.imageId;
    }

    if (loaded.id === startEvent.imageId) {
      // 手势开始时，加载完成的图与当时的会话当前图一致
      const displayDurationMs = startEvent.time.sub(loaded.time);
      if (displayDurationMs < reactionTimeMs && previousLoaded.value?.id) {
        return previousLoaded.value.id;
      }
      return startEvent.imageId;
    }

    // 手势开始时，会话已切换到新图但新图尚未完成加载：
    // 用户视角依然在刚完成加载的上一张图（loaded.id）上
    return loaded.id;
  });

  return {
    currentLoaded,
    previousLoaded,
    gestureStartEvent,
    gestureTargetId,
    handleImageLoaded,
    recordGestureStart,
  };
}
