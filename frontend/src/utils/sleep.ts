import "core-js/actual/disposable-stack";
import createEventListeners from "./createEventListeners";

export default async function sleep(
  durationMs: number,
  {
    signal,
  }: {
    signal?: AbortSignal;
  } = {},
): Promise<void> {
  if (!signal) {
    // 简单版本
    return new Promise((resolve) => {
      setTimeout(resolve, durationMs);
    });
  }
  using stack = new DisposableStack();
  await new Promise((resolve, reject) => {
    stack.use(
      createEventListeners(signal, ({ on }) => [
        on("abort", reject, { once: true }),
      ]),
    );
    stack.adopt(setTimeout(resolve, durationMs), clearTimeout);
  });
}
