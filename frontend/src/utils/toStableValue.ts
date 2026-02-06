import { isEqual } from "es-toolkit";
import type { MaybeRefOrGetter } from "vue";
import { toValue } from "vue";

export default function toStableValue<T>(
  getter: MaybeRefOrGetter<T>,
  oldValue: NoInfer<T | undefined>,
): T {
  const newValue = toValue(getter);
  if (isEqual(newValue, oldValue)) {
    return oldValue as T;
  }
  return newValue;
}
