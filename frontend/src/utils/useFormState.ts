import "core-js/actual/iterator";
import "core-js/actual/disposable-stack";
import "core-js/actual/symbol";
import {
  shallowReactive,
  computed,
  shallowRef,
  type ShallowRef,
  type InjectionKey,
  inject,
  type ShallowReactive,
  provide,
  type ComputedRef,
  onScopeDispose,
} from "vue";

export type FormState = Omit<ReturnType<typeof useFormState>, typeof Symbol.dispose | "dispose">;

const key: InjectionKey<{
  buffers: ShallowReactive<Set<ShallowRef<unknown>>>;
  isDirty: ComputedRef<boolean>;
}> = Symbol("FormState");

export default function useFormState() {
  const { buffers, isDirty } = inject(
    key,
    () => {
      const b = shallowReactive(new Set<ShallowRef<unknown>>());
      const dirty = computed(() => {
        return Iterator.from(b).some((i) => i.value !== undefined);
      });
      return {
        buffers: b,
        isDirty: dirty,
      };
    },
    true,
  );
  provide(key, { buffers, isDirty });

  const stack = new DisposableStack();
  onScopeDispose(() => stack.dispose(), true);

  function createBuffer<T>() {
    const b = shallowRef<T>();
    buffers.add(b);
    stack.defer(() => buffers.delete(b));
    return b;
  }

  function reset() {
    buffers.forEach((i) => {
      i.value = undefined;
    });
  }

  return {
    createBuffer,
    reset,
    isDirty,
    [Symbol.dispose]: stack[Symbol.dispose],
    dispose: () => stack.dispose(),
  };
}
