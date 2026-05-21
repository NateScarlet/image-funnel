import containsDeepChildNode from "@/utils/containsDeepChildNode";
import { toValue, type MaybeRefOrGetter } from "vue";
import useEventListeners from "./useEventListeners";

export default function useClickOutside(
  el: MaybeRefOrGetter<Element | null | undefined>,
  cb: (e: Event) => void,
) {
  function onEvent(e: Event) {
    const elV = toValue(el);
    if (!elV || e.target === elV || !(e.target instanceof Node)) {
      return;
    }
    if (!containsDeepChildNode(elV, e.target)) {
      cb(e);
    }
  }
  useEventListeners(
    () => document,
    (ctx) => {
      ctx.on("click", onEvent, { capture: true });
      ctx.on("focus", onEvent, { capture: true });
    },
  );
}
