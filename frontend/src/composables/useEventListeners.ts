import "core-js/actual/disposable-stack";
import {
  getCurrentInstance,
  onUnmounted,
  watch,
  type MaybeRefOrGetter,
} from "vue";
import createEventListeners from "@/utils/createEventListeners";
import isWatchSource from "@/utils/isWatchSource";

export default function useEventListeners<
  T extends {
    addEventListener: (...args: Parameters<T["addEventListener"]>) => void;
    removeEventListener: (
      ...args: Parameters<T["removeEventListener"]>
    ) => void;
  },
>(
  target: MaybeRefOrGetter<T | null | undefined>,
  init: (ctx: {
    target: T;
    on: T["addEventListener"];
    stack: DisposableStack;
  }) => void,
): Disposable {
  function setup(stack: DisposableStack, v: T) {
    stack.use(
      createEventListeners(v, (ctx) => {
        return init({ ...ctx, target: v, stack });
      }),
    );
  }

  const stack = new DisposableStack();
  if (getCurrentInstance()) {
    onUnmounted(() => {
      stack.dispose();
    });
  }
  import.meta.hot?.dispose(() => stack.dispose());

  if (isWatchSource(target)) {
    stack.defer(
      watch(
        target,
        (v, _, onCleanup) => {
          if (!v) {
            return;
          }
          const stack = new DisposableStack();
          onCleanup(() => stack.dispose());
          setup(stack, v);
        },
        { immediate: true },
      ),
    );
  } else if (target) {
    setup(stack, target);
  }
  return stack;
}
