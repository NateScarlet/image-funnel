import stableComputed from "./stableComputed";
import useLiveArrayV3 from "./useLiveArray";

export default function useLiveConnection<
  T extends { id: string; isRedacted?: boolean },
>(
  nodes: () => T[],
  {
    compare,
    filter,
    onNodeDidLeave,
    identity,
    subscribe,
  }: {
    compare?: (a: T, b: T) => number;
    filter?: (i: T) => boolean;
    onNodeDidLeave?: (i: T) => void;
    identity?: (i: T) => string;
    subscribe?: (
      item: T,
      callback: (v: T) => void,
    ) => Disposable | (() => void);
  } = {},
) {
  const resolvedIdentity = identity ?? ((i: T) => i.id);
  const { items, addItem, deleteItem, reset } = useLiveArrayV3(nodes, {
    compare,
    filter,
    identity: resolvedIdentity,
    subscribe,
  });
  const onSaved = (i: T | null | undefined) => {
    if (i == null) {
      return;
    }
    if (onNodeDidLeave && filter && !filter(i)) {
      onNodeDidLeave(i);
      return;
    }
    addItem(i);
  };
  const onDeleted = (i: { id: string } | null | undefined) => {
    if (i == null) {
      return;
    }
    deleteItem(i.id);
  };
  const onDidSaveUnfiltered = (i: T | null | undefined) => {
    if (i == null) {
      return;
    }
    if (
      items.value.some((j) => resolvedIdentity(j) === resolvedIdentity(i)) ||
      (filter && filter(i))
    ) {
      return onSaved(i);
    }
  };
  const filteredNodes = stableComputed<T[]>(() =>
    items.value.filter((i) => !i.isRedacted),
  );
  return {
    nodes: filteredNodes,
    reset,
    onSaved,
    onDeleted,
    onDidSaveUnfiltered,
  };
}
