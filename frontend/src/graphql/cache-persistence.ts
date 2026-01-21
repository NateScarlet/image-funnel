import { InMemoryCache, type NormalizedCacheObject } from "@apollo/client/core";

/**
 * 带持久化功能的 InMemoryCache
 * 自动将缓存数据保存到 localStorage
 */
export class PersistentCache extends InMemoryCache {
  private saveTimeout: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private storageKey: string,
    private maxSize: number,
    private debounceMs: number,
  ) {
    super();
    // 恢复缓存
    this.restoreFromStorage();
  }

  // #region 持久化相关方法

  private restoreFromStorage(): void {
    try {
      const cachedData = localStorage.getItem(this.storageKey);
      if (!cachedData) {
        return;
      }

      if (cachedData.length > this.maxSize) {
        if (import.meta.env.DEV) {
          console.log("[PersistentCache] 缓存超过最大限制，已清除");
        }
        localStorage.removeItem(this.storageKey);
        return;
      }

      const parsed = JSON.parse(cachedData);
      super.restore(parsed);
      if (import.meta.env.DEV) {
        console.log("[PersistentCache] 已从 localStorage 恢复缓存");
      }
    } catch (error) {
      console.error("恢复缓存失败:", error);
      localStorage.removeItem(this.storageKey);
    }
  }

  private saveToStorage(): void {
    if (this.saveTimeout) {
      clearTimeout(this.saveTimeout);
    }

    this.saveTimeout = setTimeout(() => {
      try {
        const data = super.extract();
        const serialized = JSON.stringify(data);

        if (serialized.length > this.maxSize) {
          if (import.meta.env.DEV) {
            console.log("[PersistentCache] 缓存超过最大限制，跳过保存");
          }
          return;
        }

        localStorage.setItem(this.storageKey, serialized);
        if (import.meta.env.DEV) {
          console.log("[PersistentCache] 已保存缓存到 localStorage");
        }
      } catch (error) {
        console.error("保存缓存失败:", error);
      }
    }, this.debounceMs);
  }

  // #endregion

  // #region 重写会修改缓存的方法，触发持久化

  override write(options: unknown) {
    const result = super.write(options as never);
    this.saveToStorage();
    return result;
  }

  override evict(options: unknown): boolean {
    const result = super.evict(options as never);
    if (result) {
      this.saveToStorage();
    }
    return result;
  }

  override restore(data: NormalizedCacheObject): this {
    super.restore(data);
    this.saveToStorage();
    return this;
  }

  override reset(options?: unknown): Promise<void> {
    const result = super.reset(options as never);
    this.saveToStorage();
    return result;
  }

  override removeOptimistic(id: string): void {
    super.removeOptimistic(id);
    this.saveToStorage();
  }

  override performTransaction(
    transaction: unknown,
    optimisticId?: unknown,
  ): void {
    super.performTransaction(transaction as never, optimisticId as never);
    this.saveToStorage();
  }

  override recordOptimisticTransaction(
    transaction: unknown,
    optimisticId: string,
  ): void {
    super.recordOptimisticTransaction(transaction as never, optimisticId);
    this.saveToStorage();
  }

  override gc(): string[] {
    const result = super.gc();
    this.saveToStorage();
    return result;
  }

  override modify(options: unknown): boolean {
    const result = super.modify(options as never);
    if (result) {
      this.saveToStorage();
    }
    return result;
  }

  // #endregion
}
