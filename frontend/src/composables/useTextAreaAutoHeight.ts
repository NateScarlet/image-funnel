import type { Ref, ShallowRef } from "vue";
import { nextTick, watch } from "vue";

export default function useTextAreaAutoHeight(
  el: Readonly<ShallowRef<HTMLTextAreaElement | null | undefined>>,
  text: Ref<string>,
) {
  watch(
    [el, text],
    ([elv, tv], _, onCleanup) => {
      if (elv && tv) {
        nextTick(() => {
          const originalHeight = elv.style.height;
          const originalOverflowY = elv.style.overflowY;

          const scrollHeight = elv.scrollHeight;
          const computedStyle = window.getComputedStyle(elv);
          const maxHeight = parseFloat(computedStyle.maxHeight) || Infinity;

          if (scrollHeight > maxHeight) {
            elv.style.height = `${maxHeight}px`;
            elv.style.overflowY = "auto";
          } else {
            elv.style.height = `${scrollHeight}px`;
            elv.style.overflowY = "hidden";
          }

          onCleanup(() => {
            elv.style.height = originalHeight;
            elv.style.overflowY = originalOverflowY;
          });
        });
      }
    },
    { immediate: true },
  );
}
