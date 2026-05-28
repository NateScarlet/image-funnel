import type { ShallowRef } from "vue";
import { nextTick } from "vue";
import isEventTargetScrollToEnd from "@/utils/isEventTargetScrollToEnd";
import { throttle } from "es-toolkit/compat";
import useEventListeners from "./useEventListeners";

export default function useInfiniteScroll(
  container: Readonly<ShallowRef<HTMLElement | null | undefined>>,
  fetchMore: () => Promise<void> | void,
  {
    shouldFetchMore = isEventTargetScrollToEnd,
    anchor = () => undefined,
  }: {
    shouldFetchMore?: (e: Event) => boolean;
    anchor?: () => HTMLElement | null | undefined;
  } = {},
): void {
  useEventListeners(container, (ctx) => {
    ctx.on(
      "scroll",
      throttle(async (e: Event) => {
        if (shouldFetchMore(e)) {
          const anchorEl = anchor();
          await fetchMore();
          if (anchorEl) {
            void nextTick(() => {
              const el = anchorEl.offsetParent;
              if (el) {
                el.scrollTop = anchorEl.offsetTop;
                el.scrollLeft = anchorEl.offsetLeft;
              }
            });
          }
        }
      }, 1e3),
    );
  });
}
