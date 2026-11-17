import "core-js/actual/disposable-stack";
import { ref, shallowRef, toValue, type MaybeRefOrGetter } from "vue";
import Time from "@/utils/Time";
import useDocumentVisibility from "@/composables/useDocumentVisibility";
import type { ImageAction } from "@/graphql/generated";
import Duration from "@/utils/Duration";
import useNotification from "@/composables/useNotification";
import useSession from "./domain/useSession";

/** 一次标记供归因记录使用的上下文；原始间隔与扣除值都保留，便于换口径重算 */
export interface MarkContext {
  sessionId: string;
  imageId: string;
  markIntervalMs: number;
  autoRejectTimeoutMs: number;
  loadMs: number;
  loadedAt: Time | undefined;
  markAt: Time;
}

export interface UseMarkImageOptions {
  /** 当时的自动排除设定值（毫秒）；未开启时传 0 */
  autoRejectTimeoutMs?: MaybeRefOrGetter<number>;
  onMark?: (context: MarkContext) => void;
}

export default function useMarkImage(
  sessionId: MaybeRefOrGetter<string>,
  imageLoadedAt: MaybeRefOrGetter<Time | undefined>,
  options: UseMarkImageOptions = {},
) {
  const marking = ref(false);
  const lastMarkedAt = shallowRef(Time.now());
  const { lastBecameVisibleAt } = useDocumentVisibility();
  const { show, remove } = useNotification();
  const { markImage: domainMarkImage } = useSession(sessionId);

  function getDuration(now: Time = Time.now()): Duration {
    const times: (Time | undefined)[] = [
      lastMarkedAt.value,
      lastBecameVisibleAt.value,
      toValue(imageLoadedAt) ?? Time.now(),
    ];
    const start = Time.max(times);
    if (start && start.compare(now) > 0) {
      return Duration.fromMilliseconds(0);
    }
    if (!start) {
      return Duration.fromMilliseconds(0);
    }
    return Duration.fromMilliseconds(now.sub(start));
  }

  async function mark(imageId: string | undefined, action: ImageAction) {
    // 没有显示中的图片即没有目标：直接丢弃，不进入标记中状态也不发请求
    if (!imageId) {
      return;
    }
    if (marking.value) {
      return;
    }
    marking.value = true;
    const now = Time.now();
    // 口径：加载耗时 = 相邻两次标记间隔 − 当时的自动排除设定值，两者都交给记录侧
    const markIntervalMs = Math.max(0, now.sub(lastMarkedAt.value));
    const autoRejectTimeoutMs = toValue(options.autoRejectTimeoutMs) ?? 0;
    const loadMs = Math.max(0, markIntervalMs - autoRejectTimeoutMs);
    const duration = getDuration(now);
    lastMarkedAt.value = now;
    options.onMark?.({
      sessionId: toValue(sessionId),
      imageId,
      markIntervalMs,
      autoRejectTimeoutMs,
      loadMs,
      loadedAt: toValue(imageLoadedAt),
      markAt: now,
    });

    using stack = new DisposableStack();
    stack.defer(() => {
      marking.value = false;
    });

    stack.adopt(
      setTimeout(() => {
        const id = show("正在保存标记，请稍候...", "info", 0);
        stack.adopt(id, remove);
      }, 800),
      clearTimeout,
    );

    await domainMarkImage(imageId, action, duration.toISOString());
  }

  return {
    marking,
    mark,
  };
}
