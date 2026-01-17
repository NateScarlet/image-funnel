import { isRef, type MaybeRefOrGetter, type WatchSource } from 'vue';

export default function isWatchSource<T>(
  v: MaybeRefOrGetter<T>
): v is WatchSource<T> {
  return isRef(v) || typeof v === 'function';
}
