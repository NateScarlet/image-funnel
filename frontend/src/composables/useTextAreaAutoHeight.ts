import type { MaybeRefOrGetter } from "vue";
import { toValue, watch } from "vue";

export default function useTextAreaAutoHeight(
  el: MaybeRefOrGetter<HTMLTextAreaElement | null | undefined>,
  text: MaybeRefOrGetter<string>,
) {
  watch(
    () => toValue(el),
    (el, _, onCleanup) => {
      if (!el) {
        return;
      }
      const originalHeight = el.style.height;
      const originalOverflowY = el.style.overflowY;
      onCleanup(
        watch(
          () => toValue(text),
          () => {
            const scrollHeight = el.scrollHeight;
            const computedStyle = window.getComputedStyle(el);
            const maxHeight = parseFloat(computedStyle.maxHeight) || Infinity;

            if (scrollHeight > maxHeight) {
              el.style.height = `${maxHeight}px`;
              el.style.overflowY = "auto";
            } else {
              el.style.height = `${scrollHeight}px`;
              el.style.overflowY = "hidden";
            }
          },
          { immediate: true },
        ),
      );

      onCleanup(async () => {
        const lastHeight = el.style.height;
        const lastOverflowY = el.style.overflowY;
        await Promise.allSettled(
          el.getAnimations().map((anim) => anim.finished),
        );
        // 仅在没有干扰的情况下还原
        if (
          lastHeight === originalHeight &&
          lastOverflowY === originalOverflowY
        ) {
          el.style.height = originalHeight;
          el.style.overflowY = originalOverflowY;
        }
      });
    },
    { immediate: true },
  );
}
