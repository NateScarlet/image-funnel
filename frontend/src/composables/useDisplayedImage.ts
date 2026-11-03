import { computed, shallowRef, toValue, type MaybeRefOrGetter } from "vue";
import Time from "@/utils/Time";

/**
 * 人类反应时间阈值（毫秒）。
 * 当新图片显示不足此时间时完成的手势，目标图修正为上一张图，避免误伤刚切换的新图。
 */
export const GESTURE_REACTION_TIME_MS = 100;

export interface ImageLoadedEvent {
  id: string;
  time: Time;
}

export interface GestureEndEvent {
  time: Time;
  imageId: string;
}

/** 加载记录带上产生它的会话，读取时与当前会话比对，实现会话作用域隔离 */
interface LoadedBuffer {
  sessionId: string;
  current: ImageLoadedEvent;
  previous?: ImageLoadedEvent;
}

/** 手势完成事件同样按会话作用域隔离 */
interface GestureEndBuffer {
  sessionId: string;
  event: GestureEndEvent;
}

/**
 * 显示中的图片与手势目标选择。
 *
 * 状态带上产生它的 sessionId，读取时与当前 sessionId 不一致即视为不存在。
 * 这样会话切换（SessionView 复用、仅路由参数变化）后，上一会话残留的加载/手势状态
 * 对新会话一律不可见，也无需 watch sessionId 变化再重置。
 *
 * @param sessionId 当前会话 ID，状态的作用域
 * @param currentImageId 会话队列的当前图片 ID（可能已推进但尚未加载完成）
 * @param reactionTimeMs 人类反应时间阈值（毫秒），默认 100ms
 */
export default function useDisplayedImage(
  sessionId: MaybeRefOrGetter<string>,
  currentImageId: MaybeRefOrGetter<string | null | undefined>,
  reactionTimeMs: number = GESTURE_REACTION_TIME_MS,
) {
  // 加载/手势事件是整体替换的值对象，用 shallowRef 避免 Time 被深层代理，
  // 同时保持时间对象引用稳定（loadedAt 直接交给 useMarkImage 做时长统计）
  const loadedBuffer = shallowRef<LoadedBuffer>();
  const gestureEndBuffer = shallowRef<GestureEndBuffer>();

  const loaded = computed(() => {
    const buffer = loadedBuffer.value;
    return buffer !== undefined && buffer.sessionId === toValue(sessionId) ? buffer : undefined;
  });

  const gestureEnd = computed(() => {
    const buffer = gestureEndBuffer.value;
    return buffer !== undefined && buffer.sessionId === toValue(sessionId)
      ? buffer.event
      : undefined;
  });

  // #region 会话内的图片加载记录
  function handleImageLoaded(event: ImageLoadedEvent | null | undefined) {
    if (!event) return;

    const scope = toValue(sessionId);
    const buffer = loaded.value;

    if (buffer?.current.id === event.id) {
      // 同一张图重复加载，更新加载时间戳
      loadedBuffer.value = { sessionId: scope, current: event, previous: buffer.previous };
      return;
    }

    if (buffer?.previous?.id === event.id) {
      // 撤销（Undo）退回到上一张图时，清理 previous，避免目标图指向已被撤销的图片
      loadedBuffer.value = { sessionId: scope, current: event };
      return;
    }

    // 正常推进：原当前图移入 previous，新图设为 current。
    // 会话切换后 buffer 为 undefined（会话不匹配），因此新会话首图不会继承上一会话。
    loadedBuffer.value = { sessionId: scope, current: event, previous: buffer?.current };
  }
  // #endregion

  // #region 对外暴露的显示状态与手势目标
  const displayedId = computed(() => loaded.value?.current.id);

  const loadedAt = computed(() => {
    const buffer = loaded.value;
    if (!buffer || buffer.current.id !== toValue(currentImageId)) {
      return undefined;
    }
    return buffer.current.time;
  });

  function recordGestureEnd() {
    const currentId = toValue(currentImageId);
    if (!currentId) {
      gestureEndBuffer.value = undefined;
      return;
    }
    gestureEndBuffer.value = {
      sessionId: toValue(sessionId),
      event: { time: Time.now(), imageId: currentId },
    };
  }

  const gestureTargetId = computed(() => {
    const endEvent = gestureEnd.value;
    const buffer = loaded.value;
    // 当前会话没有已加载图片时没有手势目标，不回退到队列当前图片
    if (!endEvent || !buffer) {
      return undefined;
    }

    const loadedEvent = buffer.current;
    if (loadedEvent.id === endEvent.imageId) {
      // 手势完成时（touchend），加载完成的图与当时的会话当前图一致：
      // 显示不足反应时间则修正为上一张图
      const displayDurationMs = endEvent.time.sub(loadedEvent.time);
      if (displayDurationMs < reactionTimeMs && buffer.previous) {
        return buffer.previous.id;
      }
      return endEvent.imageId;
    }

    // 手势完成时会话已切换到新图但新图尚未完成加载：
    // 用户视角依然在刚完成加载的上一张图（loaded.id）上
    return loadedEvent.id;
  });
  // #endregion

  return {
    displayedId,
    loadedAt,
    gestureTargetId,
    handleImageLoaded,
    recordGestureEnd,
  };
}
