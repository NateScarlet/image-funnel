import "core-js/actual/disposable-stack";
import randomUUID from "./randomUUID";
import sleep from "./sleep.ts";
import requireNever from "./requireNever.ts";

export type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

interface Lock {
  owner: string;
  expiresAt: number;
}

export default async function withStorageLock<T>(
  storage: StorageLike,
  key: string,
  f: () => Promise<T>,
  {
    ttlMs = 60e3,
    checkIntervalMs = 1e3,
    signal,
  }: {
    ttlMs?: number;
    checkIntervalMs?: number;
    signal?: AbortSignal;
  } = {},
): Promise<Awaited<T>> {
  const id = randomUUID();
  while (!signal?.aborted) {
    const existing = storage.getItem(key);
    if (existing) {
      // 有非法值说明不是由当前函数创建的，直接报错
      const { expiresAt } = JSON.parse(existing) as Lock;
      if (expiresAt > Date.now()) {
        await sleep(checkIntervalMs, { signal });
        continue;
      }
    }
    const lock: Lock = {
      owner: id,
      expiresAt: Date.now() + ttlMs,
    };
    let value = JSON.stringify(lock);
    using stack = new DisposableStack();
    stack.adopt(storage.setItem(key, value), () => {
      if (storage.getItem(key) === value) {
        storage.removeItem(key);
      } else {
        throw new Error(`lock ${key} stolen during execution`);
      }
    });
    // 等待可能的并发设置结束
    await sleep(checkIntervalMs, { signal });
    // 检查是否成功获取
    if (storage.getItem(key) !== value) {
      await sleep(checkIntervalMs, { signal });
      stack.dispose();
      continue;
    }
    stack.adopt(
      setInterval(() => {
        if (storage.getItem(key) === value) {
          // 持续续期
          lock.expiresAt = Date.now() + ttlMs;
          value = JSON.stringify(lock);
          storage.setItem(key, value);
        }
      }, checkIntervalMs),
      clearInterval,
    );
    return await f();
  }
  signal.throwIfAborted();
  requireNever();
}
