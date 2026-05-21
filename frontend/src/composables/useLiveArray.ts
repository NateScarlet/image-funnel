import { shallowReactive, shallowRef, computed, watch } from "vue";
import { uniqBy } from "es-toolkit";
import replaceArrayItemBy from "@/utils/replaceArrayItemBy";
import equalArray from "@/utils/equalArray";

export default function useLiveArray<T extends { id: string }>(
  arr: () => readonly T[],
  {
    compare,
    filter = () => true,
  }: {
    compare?: (a: T, b: T) => number;
    filter?: (i: T) => boolean;
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
      (i) => i.id === v.id,
      v,
      { whenNoMatch: "prepend" },
    );
  };
  const items = computed(() => {
    const v = arr();
    return uniqBy([...liveItems.value, ...v], (i) => i.id)
      .filter((i) => !liveDeletedID.has(i.id))
      .filter(filter)
      .sort((a, b) => {
        const aIndex = v.findIndex((i) => i.id === a.id);
        const bIndex = v.findIndex((i) => i.id === b.id);
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
    if (!equalArray(newValue, oldValue, { equal: (a, b) => a.id === b.id })) {
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
