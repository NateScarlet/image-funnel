import "core-js/actual/disposable-stack";

import {
  getCurrentInstance,
  onUnmounted,
  watch,
  type MaybeRefOrGetter,
} from "vue";
import createEventListeners from "@/utils/createEventListeners";
import isWatchSource from "@/utils/isWatchSource";

function posOf(e: PointerEvent) {
  return {
    x: e.clientX,
    y: e.clientY,
  };
}

export default function useGrabScroll(
  el: MaybeRefOrGetter<HTMLElement | null | undefined>,
): Disposable {
  function setup(stack: DisposableStack, el: HTMLElement) {
    const oldCursor = el.style.cursor;
    const oldUserSelect = el.style.userSelect;
    stack.defer(() => {
      el.style.cursor = oldCursor;
      el.style.userSelect = oldUserSelect;
    });
    let lastPos = { x: 0, y: 0 };
    let isGrabbing = false;
    function render() {
      el.style.userSelect = "none";
      if (isGrabbing) {
        el.style.cursor = "grabbing";
      } else {
        el.style.cursor = "grab";
      }
    }
    render();
    stack.use(
      createEventListeners(el, ({ on }) => {
        on("pointerdown", (e) => {
          if (!e.isPrimary) return;
          e.preventDefault();
          isGrabbing = true;
          render();
          lastPos = posOf(e);
        });
        on("pointermove", (e) => {
          if (!isGrabbing) {
            return;
          }
          const pos = posOf(e);
          const dy = pos.y - lastPos.y;
          const dx = pos.x - lastPos.x;
          el.scrollTop -= dy;
          el.scrollLeft -= dx;
          lastPos = pos;
        });
        on("pointerup", () => {
          isGrabbing = false;
          render();
        });
        on("pointerleave", () => {
          isGrabbing = false;
          render();
        });
      }),
    );
  }

  const stack = new DisposableStack();
  import.meta.hot?.dispose(() => stack.dispose());
  if (getCurrentInstance()) {
    onUnmounted(() => stack.dispose());
  }

  if (isWatchSource(el)) {
    stack.defer(
      watch(el, (el, _, onCleanup) => {
        if (!el) {
          return;
        }
        const stack = new DisposableStack();
        onCleanup(() => stack.dispose());
        setup(stack, el);
      }),
    );
  } else if (el) {
    setup(stack, el);
  }
  return stack;
}
