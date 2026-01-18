import "core-js/actual/disposable-stack";

async function loadResizeObserver() {
  if (typeof ResizeObserver === "undefined") {
    const { ResizeObserver } = await import("@juggle/resize-observer");
    return ResizeObserver;
  }
  return ResizeObserver;
}

export default function addResizeListener(
  el: Element,
  fn: (entry: ResizeObserverEntry) => void,
): () => void {
  const stack = new DisposableStack();
  loadResizeObserver().then((Observer) => {
    if (stack.disposed) {
      return;
    }
    const ob = stack.adopt(
      new Observer((entries): void => {
        entries.forEach((i) => {
          fn(i);
        });
      }),
      (ob) => ob.disconnect(),
    );
    ob.observe(el);
  });
  return () => stack.dispose();
}
