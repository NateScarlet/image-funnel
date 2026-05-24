/**
 * defineAwaitableEvent 定义一个类型安全的自定义异步事件
 * 通常应该直接共享返回的对象，仅需要和外部代码共享事件时有必要传入参数
 */
export default function defineAwaitableEvent<
  T extends (...args: never[]) => Promise<unknown> = () => Promise<void>,
>(type = "", target: EventTarget = new EventTarget()) {
  type Args = Parameters<T>;
  type Return = Promise<Awaited<ReturnType<T>>>;
  interface Detail {
    args: Args;
    cb: (v: Return) => void;
  }
  function dispatch(...args: Args): readonly Return[] {
    const ret: Return[] = [];
    const e = new CustomEvent(type, {
      detail: {
        args,
        cb: (v) => {
          ret.push(v);
        },
      } satisfies Detail,
    });
    target.dispatchEvent(e);
    return ret;
  }
  function subscribe(handle: T, options?: AddEventListenerOptions): () => void {
    function listener(e: CustomEvent<Detail>) {
      e.detail.cb(
        (async () => {
          return await handle(...e.detail.args);
        })() as Return,
      );
    }
    target.addEventListener(type, listener as EventListener, options);
    return () => {
      target.removeEventListener(type, listener as EventListener, options);
    };
  }
  return {
    subscribe,
    dispatch,
  };
}
