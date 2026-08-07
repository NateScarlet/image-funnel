import { ref } from "vue";
import { ImageAction } from "@/graphql/generated";

export interface RatedRejectSignal {
  seq: number;
  rating: number;
}

export default function useRatedRejectFlash() {
  const signal = ref<RatedRejectSignal | undefined>(undefined);
  let seq = 0;

  // 标记入口调用：仅当 REJECT 且已评分时生成信号。
  // 调用者负责在标记前从当前图片读取评分传入。
  function flash(action: ImageAction, rating: number) {
    if (action !== ImageAction.REJECT || rating <= 0) {
      return;
    }
    seq += 1;
    signal.value = { seq, rating };
  }

  return { signal, flash };
}
