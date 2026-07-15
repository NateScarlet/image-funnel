import {
  InMemoryCache,
  type NormalizedCacheObject,
  type Cache,
  type Transaction,
  type Reference,
  type OperationVariables,
} from "@apollo/client/core";
import { get, set } from "idb-keyval";

// #region 持久化筛选

/** 从缓存 key 中提取 __typename，格式为 "Typename:id" */
export function getTypename(key: string): string | undefined {
  const colonIndex = key.indexOf(":");
  return colonIndex === -1 ? undefined : key.substring(0, colonIndex);
}

/** 从 Directory 实体中提取 stats.latestImage.__ref */
function latestImageRef(entity: Record<string, unknown>): string | undefined {
  const stats = entity.stats as Record<string, unknown> | undefined;
  const latestImage = stats?.latestImage as { __ref?: string } | null | undefined;
  return latestImage?.__ref;
}

/** 安全解析时间戳，无效值返回 0 */
function parseTime(value: unknown): number {
  if (typeof value !== "string" && typeof value !== "number") return 0;
  const ms = new Date(value).getTime();
  return Number.isFinite(ms) ? ms : 0;
}

/**
 * 筛选缓存数据，仅保留需要持久化的内容：
 * - 非实体 key（ROOT_QUERY 等）始终保留
 * - Directory 实体全部保留，上限 200 条，按 latestImage.modTime 降序
 * - Image 实体仅保留被保留的 Directory.stats.latestImage 引用的
 * - 其他实体类型全部排除
 */
export function filterForPersistence(
  data: NormalizedCacheObject,
  maxDirectories = 200,
): NormalizedCacheObject {
  const directories: Array<{ key: string; entity: Record<string, unknown> }> = [];
  const images = new Map<string, unknown>();

  for (const [key, value] of Object.entries(data)) {
    if (!value) continue;
    const typename = getTypename(key);
    if (typename === "Directory") {
      directories.push({ key, entity: value as Record<string, unknown> });
    } else if (typename === "Image") {
      images.set(key, value);
    }
  }

  directories.sort((a, b) => {
    const aRef = latestImageRef(a.entity);
    const bRef = latestImageRef(b.entity);
    if (!aRef && !bRef) return 0;
    if (!aRef) return 1;
    if (!bRef) return -1;

    const aImage = images.get(aRef) as Record<string, unknown> | undefined;
    const bImage = images.get(bRef) as Record<string, unknown> | undefined;
    return parseTime(bImage?.modTime) - parseTime(aImage?.modTime);
  });

  const keptDirectories = directories.slice(0, maxDirectories);

  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(data)) {
    if (!value) continue;
    const typename = getTypename(key);
    if (!typename) {
      result[key] = value;
    }
  }

  for (const dir of keptDirectories) {
    result[dir.key] = dir.entity;
    const ref = latestImageRef(dir.entity);
    if (ref) {
      const img = images.get(ref);
      if (img) {
        result[ref] = img;
      }
    }
  }

  return result as NormalizedCacheObject;
}

// #endregion

// #region 加载统计

export interface LoadStats {
  elapsedMs: number;
  entityCounts: Record<string, number>;
}

export type LoadCompleteCallback = (stats: LoadStats) => void;

// #endregion

/**
 * 带持久化功能的 InMemoryCache
 * 使用 IndexedDB (idb-keyval) 进行异步存储，支持结构化克隆算法，无需 JSON 序列化
 */
export class PersistentCache extends InMemoryCache {
  private saveTimeout: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private storageKey: string,
    private debounceMs: number,
    private onLoadComplete?: LoadCompleteCallback,
  ) {
    super();
  }

  // #region 持久化相关方法

  async load(): Promise<void> {
    const start = performance.now();

    try {
      const data = await get<NormalizedCacheObject>(this.storageKey);
      if (data) {
        super.restore(data);
      }
    } catch (error) {
      console.error("恢复缓存失败:", error);
      throw error;
    }

    const elapsedMs = performance.now() - start;

    if (elapsedMs > 1000 && this.onLoadComplete) {
      const currentData = super.extract();
      const entityCounts: Record<string, number> = {};
      for (const key of Object.keys(currentData)) {
        const typename = getTypename(key);
        if (typename) {
          entityCounts[typename] = (entityCounts[typename] || 0) + 1;
        }
      }
      if (Object.keys(currentData).length === 0) {
        entityCounts["(empty)"] = 1;
      }
      this.onLoadComplete({ elapsedMs, entityCounts });
    }
  }

  private save(): void {
    if (this.saveTimeout) {
      clearTimeout(this.saveTimeout);
    }

    this.saveTimeout = setTimeout(() => {
      try {
        const data = super.extract();
        const filtered = filterForPersistence(data);
        set(this.storageKey, filtered).catch((error) => {
          console.error("保存缓存失败:", error);
        });
      } catch (error) {
        console.error("提取缓存数据失败:", error);
      }
    }, this.debounceMs);
  }

  // #endregion

  // #region 重写会修改缓存的方法，触发持久化

  override write<TData = unknown, TVariables extends OperationVariables = OperationVariables>(
    options: Cache.WriteOptions<TData, TVariables>,
  ): Reference | undefined {
    const result = super.write(options);
    this.save();
    return result;
  }

  override evict(options: Cache.EvictOptions): boolean {
    const result = super.evict(options);
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

  override reset(options?: Cache.ResetOptions): Promise<void> {
    const result = super.reset(options);
    this.save();
    return result;
  }

  override removeOptimistic(id: string): void {
    super.removeOptimistic(id);
    this.save();
  }

  override performTransaction(
    transaction: (cache: InMemoryCache) => unknown,
    optimisticId?: string | null,
  ): unknown {
    const result = super.performTransaction(transaction, optimisticId);
    this.save();
    return result;
  }

  override recordOptimisticTransaction(transaction: Transaction, optimisticId: string): void {
    super.recordOptimisticTransaction(transaction, optimisticId);
    this.save();
  }

  override gc(options?: { resetResultCache?: boolean }): string[] {
    const result = super.gc(options);
    this.save();
    return result;
  }

  override modify<Entity extends Record<string, unknown> = Record<string, unknown>>(
    options: Cache.ModifyOptions<Entity>,
  ): boolean {
    const result = super.modify(options);
    if (result) {
      this.save();
    }
    return result;
  }

  // #endregion
}
