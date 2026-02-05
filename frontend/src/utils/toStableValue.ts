import { isEqual } from "es-toolkit";
import { MaybeRefOrGetter, toValue } from "vue";

export default function toStableValue<T>(
  getter: MaybeRefOrGetter<T>,
  oldValue: T | undefined,
): T {
  const newValue = toValue(getter);
  if (isEqual(newValue, oldValue)) {
    return oldValue as T;
  }
  return newValue;
}
