import {
  ref,
  shallowRef,
  watch,
  type Ref,
  type MaybeRefOrGetter,
  toValue,
} from "vue";
import Time from "@/utils/Time";
import useDocumentVisibility from "@/composables/useDocumentVisibility";
import mutate from "@/graphql/utils/mutate";
import {
  MarkImageDocument,
  ImageAction,
  type ImageFragment,
} from "@/graphql/generated";
import Duration from "@/utils/Duration";

export default function useMarkImage(
  sessionId: MaybeRefOrGetter<string>,
  currentImage: Ref<ImageFragment | undefined>,
  imageLoadedAt: MaybeRefOrGetter<Time | undefined>,
) {
  const marking = ref(false);
  const lastMarkedAt = shallowRef(Time.now());
  const { lastBecameVisibleAt } = useDocumentVisibility();

  watch(
    () => currentImage.value?.id,
    () => {
      lastMarkedAt.value = Time.now();
    },
  );

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

  async function mark(action: ImageAction) {
    if (!currentImage.value) return;

    marking.value = true;
    const duration = getDuration();
    lastMarkedAt.value = Time.now();

    try {
      await mutate(MarkImageDocument, {
        variables: {
          input: {
            sessionId: toValue(sessionId),
            imageId: currentImage.value.id,
            action,
            duration: duration.toISOString(),
          },
        },
      });
    } finally {
      marking.value = false;
    }
  }

  return {
    marking,
    mark,
  };
}
