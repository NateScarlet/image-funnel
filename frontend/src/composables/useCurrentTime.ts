import "core-js/actual/disposable-stack";

import {
  customRef,
  getCurrentInstance,
  onUnmounted,
  shallowReactive,
  watch,
  type MaybeRefOrGetter,
} from "vue";
import Time, { type TimeInput, type TimeSource } from "@/utils/Time";
import isWatchSource from "@/utils/isWatchSource";
import useDocumentVisibility from "./useDocumentVisibility";

const MAX_TIMEOUT_DELAY = 0x7fff_ffff;

export default function useCurrentTime() {
  const stack = new DisposableStack();
  if (getCurrentInstance()) {
    onUnmounted(() => stack.dispose());
  }
  import.meta.hot?.dispose(() => stack.dispose());

  const { lastBecameVisibleAt } = useDocumentVisibility();

  let ctr: {
    track: () => void;
    trigger: () => void;
  };
  const currentTime = customRef((track, trigger) => {
    ctr = { track, trigger };
    let lastValue: Time | undefined;
    return {
      get() {
        track();
        void lastBecameVisibleAt.value;
        const v = Time.now();
        if (lastValue != null && v.equal(lastValue)) {
          // 避免 === 对象相等性比较得到不同结果
          return lastValue;
        }
        lastValue = v;
        return v;
      },
      set() {
        trigger();
      },
    };
  });

  const scheduledTimes = shallowReactive(new Set<Time>());
  function refresh() {
    scheduledTimes.forEach((i) => {
      if (i <= currentTime.value) {
        scheduledTimes.delete(i);
      }
    });
    ctr.trigger();
  }

  stack.defer(
    watch(
      () => Time.min(scheduledTimes.values()),
      (t, _, onCleanup) => {
        if (t == null) {
          return;
        }
        const delayMs = Math.min(MAX_TIMEOUT_DELAY, t.sub(Time.now()));
        const id = setTimeout(refresh, delayMs);
        return onCleanup(() => clearTimeout(id));
      },
      { immediate: true },
    ),
  );

  function schedule(input: TimeInput | null | undefined) {
    const t = Time.from(input);
    const now = Time.now();
    if (t == null || t <= now) {
      return () => undefined;
    }
    scheduledTimes.add(t);
    return () => scheduledTimes.delete(t);
  }

  function refreshOn(t: MaybeRefOrGetter<TimeSource>) {
    if (isWatchSource(t)) {
      stack.defer(
        watch(
          t,
          (t, _, onCleanup) => {
            for (const i of Time.collect(t)) {
              onCleanup(schedule(i));
            }
          },
          { immediate: true },
        ),
      );
    } else {
      for (const i of Time.collect(t)) {
        stack.defer(schedule(i));
      }
    }
  }

  function isPast(v: TimeInput | null | undefined): boolean {
    if (v == null) {
      return false;
    }
    const t = Time.from(v);
    if (t == null) {
      return false;
    }
    return t < currentTime.value;
  }

  function isFuture(v: TimeInput | null | undefined): boolean {
    if (v == null) {
      return false;
    }
    const t = Time.from(v);
    if (t == null) {
      return false;
    }
    return t > currentTime.value;
  }

  return {
    [Symbol.dispose]: () => stack.dispose(),
    refresh,
    refreshOn,
    currentTime,
    isPast,
    isFuture,
  };
}

if (import.meta.env.DEV) {
  // 简易测试

  const { currentTime, refreshOn } = useCurrentTime();
  refreshOn(() => [
    currentTime.value.add(-1),
    currentTime.value,
    currentTime.value.add(1e3),
    currentTime.value.add(1e3),
    currentTime.value.add(2e3),
  ]);
}
