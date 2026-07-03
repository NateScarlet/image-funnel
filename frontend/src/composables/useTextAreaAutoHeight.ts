import type { MaybeRefOrGetter } from "vue";
import { toValue, watch } from "vue";

export default function useTextAreaAutoHeight(
  el: MaybeRefOrGetter<HTMLTextAreaElement | null | undefined>,
  text: MaybeRefOrGetter<string>,
) {
  watch(
    () => toValue(el),
    (targetEl, _, onCleanup) => {
      if (!targetEl) {
        return;
      }
      const originalHeight = targetEl.style.height;
      const originalOverflowY = targetEl.style.overflowY;
      onCleanup(
        watch(
          () => toValue(text),
          () => {
            const { scrollHeight } = targetEl;
            const computedStyle = window.getComputedStyle(targetEl);
            const maxHeight = parseFloat(computedStyle.maxHeight) || Infinity;

            if (scrollHeight > maxHeight) {
              targetEl.style.height = `${maxHeight}px`;
              targetEl.style.overflowY = "auto";
            } else {
              targetEl.style.height = `${scrollHeight}px`;
              targetEl.style.overflowY = "hidden";
            }
          },
          { immediate: true },
        ),
      );

      onCleanup(async () => {
        const lastHeight = targetEl.style.height;
        const lastOverflowY = targetEl.style.overflowY;
        await Promise.allSettled(targetEl.getAnimations().map((anim) => anim.finished));
        // 仅在没有干扰的情况下还原
        if (lastHeight === targetEl.style.height && lastOverflowY === targetEl.style.overflowY) {
          targetEl.style.height = originalHeight;
          targetEl.style.overflowY = originalOverflowY;
        }
      });
    },
    { immediate: true },
  );
}
