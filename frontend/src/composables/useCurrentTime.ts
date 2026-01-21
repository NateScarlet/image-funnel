import "core-js/actual/disposable-stack";

import {
  computed,
  customRef,
  getCurrentInstance,
  onUnmounted,
  shallowReactive,
  watch,
  type MaybeRefOrGetter,
} from "vue";
import useDocumentVisibility from "./useDocumentVisibility";
import Time, { type TimeInput } from "@/utils/Time";
import isWatchSource from "@/utils/isWatchSource";

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
          // 避免 === 比较出错
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
    ctr.trigger();
    scheduledTimes.forEach((i) => {
      if (i <= currentTime.value) {
        scheduledTimes.delete(i);
      }
    });
  }

  const nextScheduledAt = computed(() => {
    let ret: Time | undefined;
    const now = currentTime.value;
    for (const t of scheduledTimes.values()) {
      if (t > now && (ret == null || t < ret)) {
        ret = t;
      }
    }
    return ret;
  });

  watch(
    nextScheduledAt,
    (t, _, onCleanup) => {
      if (t == null) {
        return;
      }
      const delayMs = Math.min(MAX_TIMEOUT_DELAY, t.sub(Time.now()));
      const id = setTimeout(refresh, delayMs);
      return onCleanup(() => clearTimeout(id));
    },
    { immediate: true },
  );

  function schedule(input: TimeInput) {
    const t = Time.from(input);
    const now = Time.now();
    if (t == null || t <= now) {
      refresh(); // 立即刷新
      return () => undefined;
    }
    scheduledTimes.add(t);
    return () => scheduledTimes.delete(t);
  }

  function refreshOn(t: MaybeRefOrGetter<TimeInput | undefined>) {
    if (isWatchSource(t)) {
      watch(
        t,
        (t, _, onCleanup) => {
          if (t != null) {
            onCleanup(schedule(t));
          }
        },
        { immediate: true },
      );
    } else if (t != null) {
      stack.defer(schedule(t));
    }
  }

  return {
    refresh,
    refreshOn,
    currentTime,
  };
}
