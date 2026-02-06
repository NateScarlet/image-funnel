import toStableValue from "@/utils/toStableValue";
import {
  computed,
  type ComputedGetter,
  type WritableComputedOptions,
} from "vue";

export default function stableComputed<T>(
  getter: ComputedGetter<T>,
): ReturnType<typeof computed<T>>;
export default function stableComputed<T, U = T>(
  options: WritableComputedOptions<T, U>,
): ReturnType<typeof computed<T, U>>;
export default function stableComputed<T, U = T>(
  input: ComputedGetter<T> | WritableComputedOptions<T, U>,
) {
  if (typeof input === "function") {
    return computed<T>((oldValue) => {
      return toStableValue(input(oldValue), oldValue);
    });
  }
  return computed<T, U>({
    get(oldValue) {
      return toStableValue(input.get(oldValue), oldValue);
    },
    set(v) {
      input.set(v);
    },
  });
}
