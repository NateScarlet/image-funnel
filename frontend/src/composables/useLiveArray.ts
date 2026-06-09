import {
  shallowReactive,
  shallowRef,
  computed,
  watch,
  onScopeDispose,
} from "vue";
import replaceArrayItemBy from "@/utils/replaceArrayItemBy";
import equalArray from "@/utils/equalArray";
import { isEqual, uniqBy } from "es-toolkit";

export default function useLiveArray<T extends { id: string }>(
  arr: () => readonly T[],
  {
    compare,
    filter = () => true,
    identity = (i) => i.id,
    subscribe,
  }: {
    compare?: (a: T, b: T) => number;
    filter?: (i: T) => boolean;
    identity?: (i: T) => string;
    subscribe?: (item: T, callback: (v: T) => void) => () => void;
  } = {},
) {
  const liveDeletedID = shallowReactive(new Set<string>());
  const deleteItem = (id: string) => {
    liveDeletedID.add(id);
  };
  const liveItems = shallowRef<T[]>([]);
  const addItem = (v: T) => {
    liveDeletedID.delete(v.id);
    liveItems.value = replaceArrayItemBy(
      liveItems.value,
      (i) => identity(i) === identity(v),
      v,
      { whenNoMatch: "prepend" },
    );
  };

  if (subscribe) {
    const subs = new Map<string, () => void>();
    onScopeDispose(() => {
      subs.values().forEach((i) => i());
      subs.clear();
    }, true);
    watch(
      liveItems,
      (items, oldItems) => {
        const incoming = new Map(items.map((i) => [identity(i), i]));

        for (const [k, v] of incoming) {
          if (!subs.has(k)) {
            let skipOnce = true;
            subs.set(
              k,
              subscribe(v, (newValue) => {
                if (skipOnce && isEqual(newValue, v)) {
                  // 订阅可能立即返回当前值，没必要插入
                  skipOnce = false;
                  return;
                }
                addItem(newValue);
              }),
            );
            skipOnce = false;
          }
        }
        if (oldItems) {
          for (const i of oldItems) {
            const k = identity(i);
            if (!incoming.has(k)) {
              subs.get(k)?.();
              subs.delete(k);
            }
          }
        }
      },
      { immediate: true },
    );
  }
  const items = computed(() => {
    const v = arr();

    // 建立单次遍历缓存 Map，将频繁的 O(N) 查找优化为 O(1)
    const itemByKey = new Map<string, T>();
    const indexByKey = new Map<string, number>();
    v.forEach((item, index) => {
      const key = identity(item);
      itemByKey.set(key, item);
      indexByKey.set(key, index);
    });

    const merged = uniqBy([...liveItems.value, ...v], (i) => identity(i));
    const mapped = merged.map((item) => {
      const activeItem = itemByKey.get(identity(item));
      // 仅当原始查询里的项没有被标记逻辑删除时，才使用其更完整的原始数据进行合并覆盖。
      // 否则说明原始数据已被逻辑删除（属于失效历史数据），应直接保留代表最新还原或创建的当前活跃项，避免被屏蔽过滤。
      if (activeItem && !liveDeletedID.has(activeItem.id)) {
        return activeItem;
      }
      return item;
    });

    return mapped
      .filter((i) => !liveDeletedID.has(i.id))
      .filter(filter)
      .sort((a, b) => {
        const aKey = identity(a);
        const bKey = identity(b);
        const aIndex = indexByKey.get(aKey);
        const bIndex = indexByKey.get(bKey);
        if (aIndex !== undefined && bIndex !== undefined) {
          // preserve original order
          return aIndex - bIndex;
        }
        if (compare) {
          return compare(a, b);
        }
        return 0;
      });
  });
  const itemIDs = () => items.value.map((i) => i.id);
  function reset() {
    liveDeletedID.clear();
    liveItems.value = [];
  }
  watch(arr, (newValue, oldValue) => {
    if (
      !equalArray(newValue, oldValue, {
        equal: (a, b) => identity(a) === identity(b),
      })
    ) {
      reset();
    }
  });

  return {
    items,
    addItem,
    deleteItem,
    reset,
    itemIDs,
  };
}
