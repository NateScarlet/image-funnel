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
  loadResizeObserver()
    .then((Observer) => {
      if (stack.disposed) {
        return;
      }
      const observer = stack.adopt(
        new Observer((entries): void => {
          entries.forEach((i) => {
            fn(i);
          });
        }),
        (o) => o.disconnect(),
      );
      observer.observe(el);
    })
    .catch((err: unknown) => {
      console.error("ResizeObserver polyfill 加载失败:", err);
    });
  return () => stack.dispose();
}
