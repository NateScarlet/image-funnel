import { shallowReactive, shallowRef, computed, watch } from "vue";
import { uniqBy } from "es-toolkit";
import replaceArrayItemBy from "@/utils/replaceArrayItemBy";
import equalArray from "@/utils/equalArray";

export default function useLiveArray<T extends { id: string }>(
  arr: () => readonly T[],
  {
    compare,
    filter = () => true,
    identity = (i) => i.id,
  }: {
    compare?: (a: T, b: T) => number;
    filter?: (i: T) => boolean;
    identity?: (i: T) => string;
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
  const items = computed(() => {
    const v = arr();
    return uniqBy([...liveItems.value, ...v], (i) => identity(i))
      .filter((i) => !liveDeletedID.has(i.id))
      .filter(filter)
      .sort((a, b) => {
        const aIndex = v.findIndex((i) => identity(i) === identity(a));
        const bIndex = v.findIndex((i) => identity(i) === identity(b));
        if (aIndex >= 0 && bIndex >= 0) {
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
