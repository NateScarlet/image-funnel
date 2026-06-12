import "core-js/actual/disposable-stack";
import { ref, shallowRef, type MaybeRefOrGetter, toValue } from "vue";
import Time from "@/utils/Time";
import useDocumentVisibility from "@/composables/useDocumentVisibility";
import mutate from "@/graphql/utils/mutate";
import { MarkImageDocument, ImageAction } from "@/graphql/generated";
import Duration from "@/utils/Duration";
import useNotification from "@/composables/useNotification";

export default function useMarkImage(
  sessionId: MaybeRefOrGetter<string>,
  imageLoadedAt: MaybeRefOrGetter<Time | undefined>,
) {
  const marking = ref(false);
  const lastMarkedAt = shallowRef(Time.now());
  const { lastBecameVisibleAt } = useDocumentVisibility();
  const { show, remove } = useNotification();

  function getDuration(): Duration {
    const now = Time.now();
    const times: (Time | undefined)[] = [
      lastMarkedAt.value,
      lastBecameVisibleAt.value,
      toValue(imageLoadedAt) ?? Time.now(), // 如果图片未加载完成，时长为0
    ];
    const start = Time.max(times);
    // 如果开始时间晚于当前时间（例如刚刚切换图片还未加载完成），时长为0
    if (start && start.compare(now) > 0) {
      return Duration.fromMilliseconds(0);
    }
    if (!start) {
      return Duration.fromMilliseconds(0);
    }
    return Duration.fromMilliseconds(now.sub(start));
  }

  async function mark(imageId: string, action: ImageAction) {
    if (marking.value) {
      return;
    }
    marking.value = true;
    const duration = getDuration();
    lastMarkedAt.value = Time.now();

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

    await mutate(MarkImageDocument, {
      variables: {
        input: {
          sessionId: toValue(sessionId),
          imageId,
          action,
          duration: duration.toISOString(),
        },
      },
    });
  }

  return {
    marking,
    mark,
  };
}
