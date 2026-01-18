import type { MaybeRefOrGetter } from "vue";
import { ref, watch } from "vue";
import addResizeListener from "@/utils/addResizeListener";
import isWatchSource from "@/utils/isWatchSource";

export default function useElementSize(
  el: MaybeRefOrGetter<Element | null | undefined>,
) {
  const borderBoxWidth = ref(0);
  const borderBoxHeight = ref(0);
  const contentBoxWidth = ref(0);
  const contentBoxHeight = ref(0);
  const scrollWidth = ref(0);
  const scrollHeight = ref(0);

  const update = (el: Element, entry?: ResizeObserverEntry) => {
    scrollWidth.value = el.scrollWidth;
    scrollHeight.value = el.scrollHeight;
    // 元素被切割为多片段时，暂时只使用首个片段的大小
    const borderBox = entry?.borderBoxSize[0];
    const contentBox = entry?.contentBoxSize[0];
    if (borderBox && contentBox) {
      borderBoxWidth.value = borderBox.inlineSize;
      borderBoxHeight.value = borderBox.blockSize;
      contentBoxWidth.value = contentBox.inlineSize;
      contentBoxHeight.value = contentBox.blockSize;
      return;
    }
    const bBox = el.getBoundingClientRect();
    borderBoxWidth.value = bBox.width;
    borderBoxHeight.value = bBox.height;
    // 行内元素有内容时的clientWidth 也会是0，只能用边框代替
    contentBoxWidth.value = el.clientWidth || bBox.width;
    contentBoxHeight.value = el.clientHeight || bBox.height;
  };

  if (isWatchSource(el)) {
    watch(
      el,
      (el, _, onCleanup) => {
        if (!el) {
          return;
        }
        update(el);
        onCleanup(
          // avoid forced reflow.
          addResizeListener(el, (entry) => {
            requestAnimationFrame(() => {
              update(el, entry);
            });
          }),
        );
      },
      { immediate: true },
    );
  } else if (el) {
    update(el);
  }

  return {
    borderBoxHeight,
    borderBoxWidth,
    contentBoxHeight,
    contentBoxWidth,
    scrollHeight,
    scrollWidth,
    /** @deprecated  */
    width: contentBoxWidth,
    /** @deprecated  */
    height: contentBoxHeight,
  };
}
