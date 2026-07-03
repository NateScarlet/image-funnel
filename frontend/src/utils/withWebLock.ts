import withStorageLock from "./withStorageLock";

export type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export default async function withWebLock<T>(
  key: string,
  cb: () => Promise<T>,
  fallbackOptions: {
    ttlMs?: number;
    checkIntervalMs?: number;
  } = {},
): Promise<Awaited<T>> {
  if (typeof navigator.locks === "undefined") {
    return withStorageLock(localStorage, `web-lock-fallback-${key}`, cb, fallbackOptions);
  }
  return await navigator.locks.request(key, () => {
    return cb();
  });
}
