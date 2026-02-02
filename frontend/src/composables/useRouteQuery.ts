import type { Ref } from "vue";
import { computed } from "vue";
import type { Router } from "vue-router";
import { useRoute, useRouter } from "vue-router";
import { debounce } from "es-toolkit";
import toArray from "@/utils/toArray";
import equalArray from "@/utils/equalArray";
import isNonNull from "@/utils/isNonNull";

let buffer: Record<string, string[]> = {};
let pushHistory = false;
const flush = debounce((router: Router) => {
  const cr = router.currentRoute.value;
  (pushHistory ? router.push : router.replace)({
    ...cr,
    query: {
      ...cr.query,
      ...buffer,
    },
  });
  buffer = {};
  pushHistory = false;
}, 1);

function setRouteQuery(
  router: Router,
  name: string,
  values: string[],
  pushHistoryArg: boolean,
) {
  buffer[name] = values;
  pushHistory ||= pushHistoryArg;
  flush(router);
}

export default function useRouteQuery(
  name: string,
  {
    pushHistory = false,
    defaultValue = [],
  }: { pushHistory?: boolean; defaultValue?: string[] } = {},
): Ref<string[]> {
  if (import.meta.env.DEV) {
    if (!/^[a-z][a-z_]*$/.test(name)) {
      console.warn(`useRouteQuery: name should be snake_case: '${name}'`);
    }
  }
  const route = useRoute();

  const router = useRouter();
  const values = computed({
    get() {
      const ret = toArray(route.query[name]).filter(isNonNull);
      if (ret.length === 0) {
        return defaultValue;
      }
      return ret;
    },
    set(v: string[]) {
      if (equalArray(values.value, v)) {
        return;
      }
      setRouteQuery(router, name, v, pushHistory);
    },
  });

  return values;
}
