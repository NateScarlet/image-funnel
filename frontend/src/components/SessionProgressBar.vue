<template>
  <div
    class="h-1 w-full bg-black/20 pointer-events-none relative overflow-hidden"
  >
    <!-- 背景层：展示目标达成状态 -->
    <div
      class="absolute inset-0 transition-all duration-700"
      :class="isTargetMet ? 'bg-success-500/30' : 'bg-white/10'"
    ></div>

    <!-- 操作历史层 (渐变部分)：处理大量历史记录的底层，不包含正在动画的部分 -->
    <Transition
      enter-active-class="transition-opacity duration-300"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-300"
      leave-to-class="opacity-0"
    >
      <div
        :key="historyKey"
        class="absolute inset-0"
        :style="{ backgroundImage: historyGradient }"
      ></div>
    </Transition>

    <!-- 操作历史层 (活跃部分)：通过 JS 钩子实现精准的进入/退出动画控制 -->
    <TransitionGroup
      tag="div"
      class="absolute inset-0"
      :css="false"
      @enter="onEnter"
      @leave="onLeave"
    >
      <div
        v-for="item in animatedActions"
        :key="item.index"
        :data-index="item.index"
        class="absolute top-0 bottom-0 origin-left"
        :style="{
          left: (item.index / currentSize) * 100 + '%',
          width: (1 / currentSize) * 100 + '%',
          backgroundColor: getActionColor(item.action),
        }"
      ></div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ImageAction, type SessionFragment } from "@/graphql/generated";

const props = defineProps<{
  session: SessionFragment;
}>();

const queueActions = computed(() => props.session.queueActions);
const currentSize = computed(() => props.session.currentSize);
const isTargetMet = computed(
  () => props.session.stats.kept >= props.session.targetKeep,
);

// 识别是否为批量修改
const isBatchChange = ref(false);
watch(
  () => queueActions.value.length,
  (newLen, oldLen) => {
    isBatchChange.value = Math.abs(newLen - (oldLen ?? 0)) > 1;
  },
);

/**
 * 将操作历史分为“稳定部分”和“活跃部分”
 * 虽然图片的操作类型（Action）可能会在非尾部发生变化，但其在队列中的位置（Index）是固定不变的，
 * 因此 index 是稳定的 key。
 */
const ANIMATED_COUNT = 5;

interface SplitState {
  totalLength: number;
  gradientActions: ImageAction[];
  animatedActions: {
    index: number;
    action: ImageAction;
    isTrulyNew: boolean;
  }[];
}

const splitState = computed((oldValue?: SplitState): SplitState => {
  const actions = queueActions.value;
  const total = actions.length;
  const prevTotal = oldValue?.totalLength ?? 0;

  // 批量修改时，全部进入渐变层
  if (isBatchChange.value) {
    return {
      totalLength: total,
      gradientActions: actions,
      animatedActions: [],
    };
  }

  const startIdx = Math.max(0, total - ANIMATED_COUNT);
  return {
    totalLength: total,
    gradientActions: actions.slice(0, startIdx),
    animatedActions: actions.slice(startIdx).map((action, i) => {
      const index = startIdx + i;
      return {
        index,
        action,
        // 如果索引大于等于上一次的总长度，才是真正的新增
        isTrulyNew: index >= prevTotal,
      };
    }),
  };
});

const animatedActions = computed(() => splitState.value.animatedActions);

/**
 * 用于触发渐变层整体动画的 Key
 */
const historyKey = computed(() => {
  const roundPrefix = `r${props.session.currentRound}`;
  // 批量修改时，通过改变 key 触发整体淡入动画
  if (isBatchChange.value)
    return `${roundPrefix}-batch-${queueActions.value.length}`;
  // 单步修改时保持 key 不变，确保底层稳定
  return `${roundPrefix}-stable`;
});

/**
 * 活跃部分的进入动画逻辑 (命令式控制)
 */
function onEnter(el: Element, done: () => void) {
  const htmlEl = el as HTMLElement;
  const index = Number(htmlEl.dataset.index);
  const item = animatedActions.value.find((a) => a.index === index);

  // 只有在真正的队列末尾新增时才播放缩放动画
  // 如果是从渐变层“拉回”的元素，或者是批量更新，则不播放动画以避免跳动
  if (isBatchChange.value || !item?.isTrulyNew) {
    done();
    return;
  }

  // 使用 .finished.finally 确保无论动画完成还是被取消，都能调用 done() 释放 DOM
  htmlEl
    .animate(
      [
        { transform: "scaleX(0)", opacity: 0 },
        { transform: "scaleX(1)", opacity: 1 },
      ],
      { duration: 300, easing: "ease-in-out" },
    )
    .finished.finally(done);
}

/**
 * 活跃部分的离开动画逻辑
 */
function onLeave(el: Element, done: () => void) {
  const htmlEl = el as HTMLElement;
  const index = Number(htmlEl.dataset.index);

  // 批量修改时，执行淡出动画
  if (isBatchChange.value) {
    htmlEl
      .animate([{ opacity: 1 }, { opacity: 0 }], {
        duration: 300,
        easing: "ease-in-out",
      })
      .finished.finally(done);
    return;
  }

  // 如果索引仍然在当前队列范围内，说明它是被移入了渐变层，不需要动画
  if (index < queueActions.value.length) {
    done();
    return;
  }

  htmlEl
    .animate(
      [
        { transform: "scaleX(1)", opacity: 1 },
        { transform: "scaleX(0)", opacity: 0 },
      ],
      { duration: 300, easing: "ease-in-out" },
    )
    .finished.finally(done);
}

/**
 * 根据操作类型获取对应的 CSS 变量颜色
 */
function getActionColor(action: ImageAction): string {
  switch (action) {
    case ImageAction.KEEP:
      return "var(--color-success-500)";
    case ImageAction.SHELVE:
      return "var(--color-secondary-500)";
    case ImageAction.REJECT:
      return "var(--color-error-500)";
    default:
      return "var(--color-primary-500)";
  }
}

/**
 * 生成历史记录的渐变背景
 */
const historyGradient = computed(() => {
  const actions = splitState.value.gradientActions;
  if (actions.length === 0) return "none";

  const total = currentSize.value;
  const stops: string[] = [];

  let currentAction = actions[0];
  let startIdx = 0;

  for (let i = 1; i <= actions.length; i++) {
    const action = actions[i];
    if (i === actions.length || action !== currentAction) {
      const startPos = (startIdx / total) * 100;
      const endPos = (i / total) * 100;
      const color = getActionColor(currentAction);
      stops.push(`${color} ${startPos.toFixed(4)}% ${endPos.toFixed(4)}%`);

      if (i < actions.length) {
        currentAction = action;
        startIdx = i;
      }
    }
  }

  // 补充透明部分
  const lastPos = (actions.length / total) * 100;
  stops.push(`transparent ${lastPos.toFixed(4)}%`);

  return `linear-gradient(to right, ${stops.join(", ")})`;
});
</script>
