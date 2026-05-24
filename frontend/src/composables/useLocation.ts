import { once } from "es-toolkit";
import type { Ref } from "vue";
import { customRef } from "vue";

function rawUseLocation(): Readonly<Ref<Location>> {
  return customRef((track, trigger) => {
    window.addEventListener("popstate", trigger);
    window.addEventListener("hashchange", trigger);
    return {
      get() {
        track();
        return window.location;
      },
      set() {
        if (import.meta.env.DEV) {
          console.error("location is read only");
        }
      },
    };
  });
}

const useLocation = once(rawUseLocation);
export default useLocation;
