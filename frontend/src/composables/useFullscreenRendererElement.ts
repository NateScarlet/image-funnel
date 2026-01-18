import type { RendererElement } from "vue";
import { ref } from "vue";
import useEventListeners from "./useEventListeners";

const rendererEl = ref<RendererElement>(document.body);

useEventListeners(document, (ctx) => {
  ctx.on(
    "fullscreenchange",
    () => {
      if (document.fullscreenElement) {
        rendererEl.value = document.fullscreenElement;
        return;
      }
      rendererEl.value = document.body;
    },
    { passive: true },
  );
});

export default function useFullscreenRendererElement() {
  return rendererEl;
}
