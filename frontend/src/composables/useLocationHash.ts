import type { Ref } from "vue";
import { triggerRef, computed } from "vue";
import useLocation from "@/composables/useLocation";

export default function useLocationHash({
  pushHistory = false,
}: { pushHistory?: boolean } = {}): Ref<string> {
  const location = useLocation();
  const value = computed({
    get() {
      const v = location.value.hash;
      if (v) {
        const raw = v.slice(1); // 去掉开头的 '#'
        try {
          return decodeURIComponent(raw);
        } catch {
          // 解码失败时返回原始字符串
          return raw;
        }
      }
      return "";
    },
    set(v: string) {
      if (value.value === v) {
        return;
      }
      const newURL = new URL(location.value.href);
      // 对输入进行 URL 编码
      const encoded = v ? encodeURIComponent(v) : "";
      newURL.hash = encoded ? `#${encoded}` : "";
      if (pushHistory) {
        window.history.pushState(null, "", newURL);
      } else {
        window.history.replaceState(null, "", newURL);
      }
      triggerRef(location);
    },
  });
  return value;
}
