import { InMemoryCache, type NormalizedCacheObject } from "@apollo/client/core";
import { get, set } from "idb-keyval";

/**
 * 带持久化功能的 InMemoryCache
 * 使用 IndexedDB (idb-keyval) 进行异步存储，支持结构化克隆算法，无需 JSON 序列化
 */
export class PersistentCache extends InMemoryCache {
  private saveTimeout: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private storageKey: string,
    private debounceMs: number,
  ) {
    super();
  }

  // #region 持久化相关方法

  /**
   * 异步恢复缓存数据
   * 应在应用启动时调用
   */
  async load(): Promise<void> {
    try {
      const data = await get<NormalizedCacheObject>(this.storageKey);
      if (data) {
        super.restore(data);
      }
    } catch (error) {
      console.error("恢复缓存失败:", error);
    }
  }

  private save(): void {
    if (this.saveTimeout) {
      clearTimeout(this.saveTimeout);
    }

    this.saveTimeout = setTimeout(() => {
      try {
        // extract() 获取的是普通 JS 对象，IndexedDB 可以直接存储
        const data = super.extract();
        // 这是一个异步操作，不需要等待它完成
        set(this.storageKey, data).catch((error) => {
          console.error("保存缓存失败:", error);
        });
      } catch (error) {
        console.error("提取缓存数据失败:", error);
      }
    }, this.debounceMs);
  }

  // #endregion

  // #region 重写会修改缓存的方法，触发持久化

  override write(options: unknown) {
    const result = super.write(options as never);
    this.save();
    return result;
  }

  override evict(options: unknown): boolean {
    const result = super.evict(options as never);
    if (result) {
      this.save();
    }
    return result;
  }

  override restore(data: NormalizedCacheObject): this {
    super.restore(data);
    this.save();
    return this;
  }

  override reset(options?: unknown): Promise<void> {
    const result = super.reset(options as never);
    this.save();
    return result;
  }

  override removeOptimistic(id: string): void {
    super.removeOptimistic(id);
    this.save();
  }

  override performTransaction(
    transaction: unknown,
    optimisticId?: unknown,
  ): void {
    super.performTransaction(transaction as never, optimisticId as never);
    this.save();
  }

  override recordOptimisticTransaction(
    transaction: unknown,
    optimisticId: string,
  ): void {
    super.recordOptimisticTransaction(transaction as never, optimisticId);
    this.save();
  }

  override gc(): string[] {
    const result = super.gc();
    this.save();
    return result;
  }

  override modify(options: unknown): boolean {
    const result = super.modify(options as never);
    if (result) {
      this.save();
    }
    return result;
  }

  // #endregion
}
