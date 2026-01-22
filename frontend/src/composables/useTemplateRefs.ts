import { type VNodeRef, computed, onBeforeUpdate, shallowReactive } from "vue";

/**
 * 用于获取响应式引用数组
 * vue 原生的 useTemplateRef 对于 v-for 是非响应式的
 */
export default function useTemplateRefs<T>() {
  const refs = shallowReactive<T[]>([]);
  onBeforeUpdate(() => {
    refs.length = 0;
  });
  const ref: VNodeRef = (node) => {
    if (node) {
      refs.push(node as T);
    }
  };
  return computed(() => Object.assign(refs, { ref }));
}
