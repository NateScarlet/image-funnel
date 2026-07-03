import "core-js/actual/disposable-stack";

import { getCurrentInstance, onUnmounted, watch, type MaybeRefOrGetter } from "vue";
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
  options?: {
    beforeStart?: (e: PointerEvent) => boolean;
  },
): Disposable {
  function setup(stack: DisposableStack, targetEl: HTMLElement) {
    const oldCursor = targetEl.style.cursor;
    const oldUserSelect = targetEl.style.userSelect;
    stack.defer(() => {
      targetEl.style.cursor = oldCursor;
      targetEl.style.userSelect = oldUserSelect;
    });
    let lastPos = { x: 0, y: 0 };
    let isGrabbing = false;
    function render() {
      targetEl.style.userSelect = "none";
      if (isGrabbing) {
        targetEl.style.cursor = "grabbing";
      } else {
        targetEl.style.cursor = "grab";
      }
    }
    render();
    stack.use(
      createEventListeners(targetEl, ({ on }) => {
        on("pointerdown", (e) => {
          if (!e.isPrimary) return;
          if (options?.beforeStart && !options.beforeStart(e)) return;
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
          targetEl.scrollTop -= dy;
          targetEl.scrollLeft -= dx;
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
      watch(el, (targetEl, _, onCleanup) => {
        if (!targetEl) {
          return;
        }
        const innerStack = new DisposableStack();
        onCleanup(() => innerStack.dispose());
        setup(innerStack, targetEl);
      }),
    );
  } else if (el) {
    setup(stack, el);
  }
  return stack;
}
