import {
  onScopeDispose,
  shallowRef,
  toValue,
  type Ref,
  type MaybeRefOrGetter,
  computed,
  watch,
} from "vue";
import defineCustomEvent from "@/utils/defineCustomEvent";
import isWatchSource from "@/utils/isWatchSource";
import useEventListeners from "./useEventListeners";

export type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export type UseStorageV3ReturnType<T> = Disposable & {
  model: Ref<T>;
  flush: () => void;
  clear: () => void;
  reload: () => T;
};

const { dispatch, subscribe } = defineCustomEvent<{
  storage: StorageLike;
  key: string;
  senderId: number;
}>();
let nextId = 0;

function useStorage<T>(
  storage: StorageLike,
  key: MaybeRefOrGetter<string>,
  nullValue: MaybeRefOrGetter<T>,
): UseStorageV3ReturnType<T>;
function useStorage<T>(
  storage: StorageLike,
  key: MaybeRefOrGetter<string>,
): UseStorageV3ReturnType<T | undefined>;
function useStorage<T>(
  storage: StorageLike,
  key: MaybeRefOrGetter<string>,
  nullValue?: MaybeRefOrGetter<T>,
): UseStorageV3ReturnType<T | undefined> {
  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose(), true);
  import.meta.hot?.dispose(() => stack.dispose());

  const id = nextId;
  nextId += 1;

  // 存储原始字符串，这是唯一的响应式来源
  const rawEntry = shallowRef<[string, string | null]>();
  const raw = computed(() => rawEntry.value?.[1]);

  function write(v: T | undefined) {
    const k = toValue(key);
    // 直接原地修改返回的对象然后通知写入是合法操作，所以每次写入都应该重新序列化
    const newValue = v == null ? null : JSON.stringify(v);
    if (k === rawEntry.value?.[0] && newValue === rawEntry.value[1]) {
      return;
    }
    if (newValue == null) {
      storage.removeItem(k);
    } else {
      storage.setItem(k, newValue);
    }
    rawEntry.value = [k, newValue];
    dispatch({ detail: { storage, key: k, senderId: id } });
  }

  const model = computed<T | undefined>({
    get() {
      const str = raw.value;
      if (str != null) {
        try {
          return JSON.parse(str) as T;
        } catch (err) {
          if (import.meta.env.DEV) {
            console.error({
              msg: "invalid value",
              storage,
              key: rawEntry.value?.[0],
              value: str,
              err,
            });
          }
        }
      }
      return toValue(nullValue);
    },
    set(v) {
      write(v);
    },
  });

  function reload(): T | undefined {
    const k = toValue(key);
    rawEntry.value = [k, storage.getItem(k)];
    return model.value;
  }

  function flush() {
    write(model.value);
  }

  function clear() {
    write(undefined);
  }

  // 初始加载
  reload();

  // 监听 key 变化，重新加载数据
  if (isWatchSource(key)) {
    stack.defer(
      watch(key, () => {
        reload();
      }),
    );
  }

  // 监听 storage 事件和自定义同标签页事件
  stack.use(
    useEventListeners(window, ({ on }) => {
      on("storage", (e) => {
        if (e.storageArea === storage && e.key === toValue(key)) {
          reload();
        }
      });
    }),
  );
  stack.defer(
    subscribe((e) => {
      if (
        e.detail.senderId !== id &&
        e.detail.storage === storage &&
        e.detail.key === toValue(key)
      ) {
        reload();
      }
    }),
  );

  return {
    model,
    flush,
    reload,
    clear,

    [Symbol.dispose]: stack.dispose.bind(stack),
  };
}

export default useStorage;
