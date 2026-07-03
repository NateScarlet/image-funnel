import { ref, toValue, watch, type MaybeRefOrGetter, type Ref } from "vue";
import createEventListeners from "@/utils/createEventListeners";

export default function useDragInput({
  el,
  x,
  y,
  cursorStyle,
}: {
  el: () => HTMLElement | null | undefined;
  x?: Ref<number>;
  y?: Ref<number>;
  cursorStyle?: MaybeRefOrGetter<string>;
}) {
  const stack = new DisposableStack();
  const dragging = ref(false);

  if (cursorStyle) {
    stack.defer(
      watch(
        [() => toValue(cursorStyle), dragging, el],
        ([cs, v, targetEl], _, onCleanup) => {
          if (!targetEl) {
            return;
          }
          const oldBodyCursor = document.body.style.cursor;
          const oldElCursor = targetEl.style.cursor;
          onCleanup(() => {
            document.body.style.cursor = oldBodyCursor;
            targetEl.style.cursor = oldElCursor;
          });
          targetEl.style.cursor = cs;
          if (v) {
            document.body.style.cursor = cs;
          }
        },
        { immediate: true },
      ),
    );
  }
  stack.defer(
    watch(el, (targetEl, _, onCleanup) => {
      if (!targetEl) {
        return;
      }
      let startX: number | undefined;
      let startY: number | undefined;
      let originX = 0;
      let originY = 0;
      const innerStack = new DisposableStack();
      onCleanup(() => innerStack.dispose());
      innerStack.use(
        createEventListeners(targetEl, ({ on }) => {
          on("pointerdown", (e) => {
            if (e.target === targetEl) {
              e.preventDefault();
            }
            dragging.value = true;
            originX = e.clientX;
            originY = e.clientY;
            startX = x?.value;
            startY = y?.value;
          });
        }),
      );
      innerStack.use(
        createEventListeners(window, ({ on }) => {
          on("pointerup", () => {
            dragging.value = false;
          });
          on("pointermove", (e) => {
            if (!dragging.value) {
              return;
            }
            if (x && startX != null) {
              const deltaX = e.clientX - originX;
              x.value = startX + deltaX;
            }
            if (y && startY != null) {
              const deltaY = e.clientY - originY;
              y.value = startY + deltaY;
            }
          });
        }),
      );
    }),
  );

  return {
    [Symbol.dispose]: () => stack.dispose(),
    dragging,
  };
}
