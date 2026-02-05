import { computed, shallowRef, type Ref } from "vue";
import toStableValue from "@/utils/toStableValue";
import useEventListeners from "./useEventListeners";

export type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export type UseStorageReturnType<T> = Disposable & {
  model: Ref<T>;
  flush: () => void;
  clear: () => void;
  reload: () => T;
};

function useStorage<T>(
  storage: StorageLike,
  key: string,
  defaultValue: () => T,
): UseStorageReturnType<T>;
function useStorage<T>(
  storage: StorageLike,
  key: string,
): UseStorageReturnType<T | undefined>;
function useStorage<T>(
  storage: StorageLike,
  key: string,
  defaultValue?: () => T,
): UseStorageReturnType<T | undefined> {
  const buffer = shallowRef<T | undefined>();
  function reload() {
    const rawValue = storage.getItem(key);
    try {
      const value = rawValue == null ? undefined : JSON.parse(rawValue);
      buffer.value = value;
      return value;
    } catch {
      buffer.value = undefined;
    }
  }
  function flush() {
    const v = buffer.value;
    if (v == null) {
      storage.removeItem(key);
    } else {
      storage.setItem(key, JSON.stringify(v));
    }
    reload();
  }
  function clear() {
    buffer.value = undefined;
    flush();
  }
  const { [Symbol.dispose]: dispose } = useEventListeners(window, ({ on }) => {
    on("storage", (e) => {
      if (e.storageArea !== storage || e.key !== key) {
        return;
      }
      reload();
    });
  });
  reload();
  let lastValue: T | undefined;
  const model = computed({
    get() {
      lastValue = toStableValue(buffer.value ?? defaultValue?.(), lastValue);
      return lastValue;
    },
    set(v) {
      buffer.value = v;
      flush();
    },
  });

  return {
    model,
    flush,
    reload,
    clear,

    [Symbol.dispose]: dispose,
  };
}

export default useStorage;
