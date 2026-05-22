<template>
  <div
    class="flex items-center gap-2 bg-primary-800 border border-primary-700 px-3 h-[34px] rounded-lg select-none"
  >
    <span class="text-xs text-primary-400">评分:</span>

    <!-- 操作符切换按钮 -->
    <button
      class="w-7 h-6 bg-primary-750 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded flex items-center justify-center text-xs font-bold text-secondary-400 hover:text-secondary-300 transition-all cursor-pointer"
      :title="configs[currentOperator].title"
      @click="cycleOperator"
    >
      {{ configs[currentOperator].symbol }}
    </button>

    <!-- 星级评分区域：0-5 星统一用 RatingIcon 渲染 -->
    <div class="flex items-center gap-0.5">
      <button
        v-for="star in [0, 1, 2, 3, 4, 5]"
        :key="star"
        class="transition-all hover:scale-125 p-0.5 rounded cursor-pointer group relative flex items-center justify-center"
        @click="handleStarClick(star)"
      >
        <RatingIcon
          :rating="star"
          :filled="isStarActive(star)"
          class="!w-5 !h-5"
        />

        <!-- 悬浮微型文字指示，特别是非等号模式时提示范围 -->
        <span
          class="absolute top-full left-1/2 -translate-x-1/2 mt-1.5 px-1.5 py-0.5 bg-black/80 backdrop-blur-md border border-white/10 text-[9px] text-white rounded opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity whitespace-nowrap z-50"
        >
          {{ getStarTooltip(star) }}
        </span>
      </button>
    </div>
  </div>
</template>

<script lang="ts">
import useStorage from "../composables/useStorage";

// 评分过滤器支持的操作符类型
export type Operator = "=" | ">=" | "<=";

// 使用 localStorage 记住用户上次选择的评分操作模式
const { model: currentOperator } = useStorage<Operator>(
  localStorage,
  "rating_filter_operator_d8615b",
  () => "=",
);
</script>

<script setup lang="ts">
import { watch } from "vue";
import RatingIcon from "./RatingIcon.vue";

// 双向绑定当前的评分过滤值
const modelValue = defineModel<number[]>({ default: () => [] });

interface OperatorConfig {
  symbol: string;
  title: string;
  // 处理在某颗星上点击，返回新的选值数组
  handleClick: (currentVal: number[], star: number) => number[];
  // 获取当前数组下该模式所对应的基准星级（用于切换模式时的数据迁移）
  getBaseStar: (currentVal: number[]) => number | undefined;
  // 给定一个基准星级，返回该模式下应当生成的数组值
  getValuesForStar: (star: number) => number[];
}

// #region 各项操作符的配置定义
const configs: Record<Operator, OperatorConfig> = {
  "=": {
    symbol: "=",
    title: "模式: 精确匹配 (支持多选)",
    handleClick(currentVal, star) {
      const idx = currentVal.indexOf(star);
      if (idx >= 0) {
        return currentVal.filter((v) => v !== star);
      } else {
        return [...currentVal, star].sort((a, b) => a - b);
      }
    },
    getBaseStar(currentVal) {
      return currentVal.length > 0 ? currentVal[0] : undefined;
    },
    getValuesForStar(star) {
      return [star];
    },
  },
  ">=": {
    symbol: "≥",
    title: "模式: 大于等于",
    handleClick(currentVal, star) {
      const expected = configs[">="].getValuesForStar(star);
      if (
        currentVal.length === expected.length &&
        currentVal.every((v, i) => v === expected[i])
      ) {
        return [];
      }
      return expected;
    },
    getBaseStar(currentVal) {
      if (currentVal.length > 0 && currentVal.includes(5)) {
        const min = Math.min(...currentVal);
        if (currentVal.length === 5 - min + 1) {
          return min;
        }
      }
      return undefined;
    },
    getValuesForStar(star) {
      const res: number[] = [];
      for (let i = star; i <= 5; i++) {
        res.push(i);
      }
      return res;
    },
  },
  "<=": {
    symbol: "≤",
    title: "模式: 小于等于",
    handleClick(currentVal, star) {
      const expected = configs["<="].getValuesForStar(star);
      if (
        currentVal.length === expected.length &&
        currentVal.every((v, i) => v === expected[i])
      ) {
        return [];
      }
      return expected;
    },
    getBaseStar(currentVal) {
      if (currentVal.length > 0 && currentVal.includes(0)) {
        const max = Math.max(...currentVal);
        if (currentVal.length === max + 1) {
          return max;
        }
      }
      return undefined;
    },
    getValuesForStar(star) {
      const res: number[] = [];
      for (let i = 0; i <= star; i++) {
        res.push(i);
      }
      return res;
    },
  },
};
// #endregion

// 判断特定的星是否处于激活态，直接检查是否包含在传入的评分数组中
function isStarActive(star: number): boolean {
  return modelValue.value.includes(star);
}

// 获取各个星星的悬浮提示文案
function getStarTooltip(star: number): string {
  if (currentOperator.value === "=") {
    return star === 0 ? "无评分 (0 星)" : `${star} 星`;
  }
  const symbol = configs[currentOperator.value].symbol;
  return star === 0 ? `${symbol} 无评分` : `${symbol} ${star} 星`;
}

// 切换操作符
function cycleOperator() {
  const ops: Operator[] = ["=", ">=", "<="];
  const idx = ops.indexOf(currentOperator.value);
  const nextOp = ops[(idx + 1) % ops.length];

  // 尝试从当前选中值中提取出基准星级，用于新模式的初始化
  const baseStar = configs[currentOperator.value].getBaseStar(modelValue.value);

  // 切换操作符
  currentOperator.value = nextOp;

  // 如果提取到了基准星级，则将选值转换为新模式下的对应范围，否则清空选值
  if (baseStar !== undefined) {
    modelValue.value = configs[nextOp].getValuesForStar(baseStar);
  } else {
    modelValue.value = [];
  }
}

// 点击星星的处理逻辑
function handleStarClick(star: number) {
  const config = configs[currentOperator.value];
  modelValue.value = config.handleClick(modelValue.value, star);
}

// #region 状态同步与事件触发

// 根据传入值自动反向推导最贴切的操作符
function inferOperator(val: number[]): Operator | undefined {
  if (val.length === 0) return undefined;

  const sortedVal = [...val].sort((a, b) => a - b);

  if (sortedVal.length === 1) {
    return "=";
  }

  const candidates: Operator[] = [">=", "<="];
  for (const op of candidates) {
    const baseStar = configs[op].getBaseStar(sortedVal);
    if (baseStar !== undefined) {
      const expected = configs[op].getValuesForStar(baseStar);
      if (
        sortedVal.length === expected.length &&
        sortedVal.every((v, i) => v === expected[i])
      ) {
        return op;
      }
    }
  }

  return "=";
}

// 监听外部清空或修改事件，同步组件内部状态
watch(
  modelValue,
  (newVal) => {
    const inferred = inferOperator(newVal);
    if (inferred !== undefined) {
      currentOperator.value = inferred;
    }
  },
  { deep: true, immediate: true },
);

// #endregion
</script>
