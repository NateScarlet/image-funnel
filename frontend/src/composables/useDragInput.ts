import { ref, watch, type Ref } from "vue";
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
  cursorStyle?: () => string;
}) {
  const stack = new DisposableStack();
  const dragging = ref(false);

  if (cursorStyle) {
    stack.defer(
      watch(
        [cursorStyle, dragging, el],
        ([cursorStyle, v, el], _, onCleanup) => {
          if (!el) {
            return;
          }
          const oldBodyCursor = document.body.style.cursor;
          const oldElCursor = el.style.cursor;
          onCleanup(() => {
            document.body.style.cursor = oldBodyCursor;
            el.style.cursor = oldElCursor;
          });
          el.style.cursor = cursorStyle;
          if (v) {
            document.body.style.cursor = cursorStyle;
          }
        },
        { immediate: true }
      )
    );
  }
  stack.defer(
    watch(el, (el, _, onCleanup) => {
      if (!el) {
        return;
      }
      let startX: number | undefined;
      let startY: number | undefined;
      let originX = 0;
      let originY = 0;
      const stack = new DisposableStack();
      onCleanup(() => stack.dispose());
      stack.use(
        createEventListeners(el, ({ on }) => {
          on("pointerdown", (e) => {
            if (e.target === el) {
              e.preventDefault();
            }
            dragging.value = true;
            originX = e.clientX;
            originY = e.clientY;
            startX = x?.value;
            startY = y?.value;
          });
        })
      );
      stack.use(
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
        })
      );
    })
  );

  return {
    [Symbol.dispose]: () => stack.dispose(),
    dragging,
  };
}
