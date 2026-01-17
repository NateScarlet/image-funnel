import "core-js/actual/disposable-stack";

export default function createEventListeners<
  T extends {
    addEventListener: (...args: Parameters<T["addEventListener"]>) => void;
    removeEventListener: (...args: Parameters<T["addEventListener"]>) => void;
  },
>(target: T, init: (ctx: { on: T["addEventListener"] }) => void): Disposable {
  const stack = new DisposableStack();
  init({
    on(...args: Parameters<T["addEventListener"]>): void {
      target.addEventListener(...args);
      stack.defer(() => target.removeEventListener(...args));
    },
  });
  return stack;
}
