import type { Ref } from "vue";
import { ref, watch } from "vue";
import createEventListeners from "@/utils/createEventListeners";

export default function useElementFullscreen(el: Ref<HTMLElement | undefined>) {
  const isFullscreen = ref(false);
  const updateFullscreen = () => {
    isFullscreen.value =
      el.value != null && document.fullscreenElement === el.value;
  };
  const requestFullscreen = async () => {
    await el.value?.requestFullscreen();
  };
  const exitFullscreen = async () => {
    if (!isFullscreen.value) {
      return;
    }
    await document.exitFullscreen();
  };
  const toggleFullscreen = async (force?: boolean) => {
    const wanted = force != null ? force : !isFullscreen.value;
    if (wanted) {
      await requestFullscreen();
    } else {
      await exitFullscreen();
    }
  };

  watch(
    el,
    (n, _, onCleanup) => {
      if (!n) {
        return;
      }
      const stack = new DisposableStack();
      onCleanup(() => stack.dispose());
      updateFullscreen();
      stack.use(
        createEventListeners(n, ({ on }) => {
          on("fullscreenchange", updateFullscreen);
        }),
      );
    },
    { immediate: true },
  );

  return {
    requestFullscreen,
    exitFullscreen,
    toggleFullscreen,
    isFullscreen,
  };
}
