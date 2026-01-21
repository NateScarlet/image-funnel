import createEventListeners from "@/utils/createEventListeners";
import Time from "@/utils/Time";
import { ref, shallowRef } from "vue";

const state = ref(document.visibilityState);
const lastChangeAt = shallowRef(Time.now());
const lastBecameVisibleAt = shallowRef(lastChangeAt.value);
function update() {
  if (document.visibilityState === state.value) {
    return;
  }
  state.value = document.visibilityState;
  const now = Time.now();
  lastChangeAt.value = now;
  if (document.visibilityState === "visible") {
    lastBecameVisibleAt.value = now;
  }
}
let initOnce = false;

export default function useDocumentVisibility() {
  if (!initOnce) {
    const disposable = createEventListeners(document, ({ on }) => {
      on("visibilitychange", update, { passive: true });
    });
    import.meta.hot?.dispose(() => disposable[Symbol.dispose]());

    initOnce = true;
  }
  return {
    state,
    lastChangeAt,
    lastBecameVisibleAt,
  };
}
