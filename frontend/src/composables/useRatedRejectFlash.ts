import { computed, ref, toValue, type MaybeRefOrGetter } from "vue";
import { ImageAction } from "@/graphql/generated";

export interface RatedRejectSignal {
  seq: number;
  rating: number;
}

// 信号按会话作用域隔离：仅当触发动画的会话仍是当前会话时才对外可见。
// 会话提交后自动切换到下一目录会话时 SessionView 组件实例被复用（路由参数变化不重建），
// 若不隔离会话，残留信号会在新会话首图加载时重放扫过动画。
export default function useRatedRejectFlash(sessionId: MaybeRefOrGetter<string>) {
  const buffer = ref<{ sessionId: string; signal: RatedRejectSignal } | undefined>(undefined);
  const signal = computed(() => {
    const b = buffer.value;
    return b !== undefined && b.sessionId === toValue(sessionId) ? b.signal : undefined;
  });
  let seq = 0;

  // 标记入口调用：仅当 REJECT 且已评分时生成信号。
  // 调用者负责在标记前从当前图片读取评分传入。
  function flash(action: ImageAction, rating: number) {
    if (action !== ImageAction.REJECT || rating <= 0) {
      return;
    }
    seq += 1;
    buffer.value = { sessionId: toValue(sessionId), signal: { seq, rating } };
  }

  // 动画结束或中断时清除信号
  function clear() {
    buffer.value = undefined;
  }

  return { signal, flash, clear };
}
