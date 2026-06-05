import type { Ref } from "vue";
import { triggerRef, computed } from "vue";
import useLocation from "@/composables/useLocation";

export default function useLocationHash({
  pushHistory = false,
}: { pushHistory?: boolean } = {}): Ref<string> {
  const location = useLocation();
  const value = computed({
    get() {
      const h = location.value.hash.slice(1);
      try {
        return decodeURIComponent(h); // 去掉开头的 '#'
      } catch {
        return h;
      }
    },
    set(v: string) {
      if (value.value === v) {
        return;
      }
      const newURL = new URL(location.value.href);
      newURL.hash = v ? `#${v}` : "";
      if (pushHistory) {
        window.history.pushState(window.history.state, "", newURL);
      } else {
        window.history.replaceState(window.history.state, "", newURL);
      }
      triggerRef(location);
    },
  });
  return value;
}
