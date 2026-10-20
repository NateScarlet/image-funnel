/** 旧版浏览顺序记录键的精确前缀，仅用于清理历史数据 */
const LEGACY_DIRECTORY_ORDER_PREFIX = "directoryOrder@122b3b541739-";

/**
 * 清理已被游标导航取代的旧版目录顺序记录。
 * 幂等：没有旧键时不做任何操作。
 */
export function migrate(): void {
  const legacyKeys: string[] = [];

  for (let i = 0; i < localStorage.length; i += 1) {
    const key = localStorage.key(i);
    if (key?.startsWith(LEGACY_DIRECTORY_ORDER_PREFIX)) {
      legacyKeys.push(key);
    }
  }

  for (const key of legacyKeys) {
    localStorage.removeItem(key);
  }
}
