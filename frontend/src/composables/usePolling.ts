import isAbortError from "@/utils/isAbortError";
import { watch, type WatchSource } from "vue";

export class PollingContext {
  /** stack 直到下次执行或中止轮询才会被清理 */
  public readonly stack = new DisposableStack();

  private rawSignal?: AbortSignal;

  get signal(): AbortSignal {
    if (this.rawSignal == null) {
      // 按需提供 signal
      const ctr = this.stack.adopt(new AbortController(), (i) => i.abort());
      this.rawSignal = ctr.signal;
    }
    return this.rawSignal;
  }

  public readonly stopPolling = () => {
    this.stack.dispose();
  };
}

export default function usePolling({
  update,
  scheduleNext = (update) => {
    const stack = new DisposableStack();
    stack.adopt(requestAnimationFrame(update), cancelAnimationFrame);
    return stack;
  },
  paused = () => false,
  onError = (err) => {
    if (import.meta.env.DEV) {
      console.error("error during polling", err, update);
    }
  },
}: {
  update: (ctx: PollingContext) => Promise<void> | void;
  scheduleNext?: (update: () => void) => Disposable;
  paused?: WatchSource<boolean>;
  onError?: (err: unknown) => void;
}) {
  const dispose = watch(
    paused,
    (v, _, onCleanup) => {
      if (v) {
        return;
      }
      let activeCtx: PollingContext | undefined;
      let didCancel = false; // 确保还未异步创建 ctx 时也能够中止
      onCleanup(() => {
        didCancel = true;
        activeCtx?.stack.dispose();
      });
      async function run() {
        if (didCancel) {
          // 停止轮询
          return;
        }
        activeCtx?.stack.dispose(); // 每次执行前取消前一次执行，确保同时只有一次执行。
        const ctx = new PollingContext();
        activeCtx = ctx;
        // 执行
        try {
          await update(ctx);
        } catch (err) {
          if (!isAbortError(err)) {
            onError?.(err);
          }
        }
        if (ctx.stack.disposed) {
          // 执行中取消了，不再调度下一次
          return;
        }
        // 调度
        try {
          ctx.stack.use(scheduleNext(run));
        } catch (err) {
          ctx.stack.dispose(); // 无法继续调度，只能停止
          onError?.(err);
        }
      }
      run();
    },
    { immediate: true },
  );
  import.meta.hot?.dispose(dispose);
  return {
    dispose,
    [Symbol.dispose]: dispose,
  };
}
