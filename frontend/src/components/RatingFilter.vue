<template>
  <div
    class="flex items-center gap-2 bg-primary-800 border border-primary-700 px-3 py-1 rounded-lg select-none"
  >
    <span class="text-xs text-primary-400">评分:</span>

    <!-- 操作符切换按钮 -->
    <button
      class="w-7 h-6 bg-primary-750 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded flex items-center justify-center text-xs font-bold text-secondary-400 hover:text-secondary-300 transition-all cursor-pointer"
      :title="operatorTitles[currentOperator]"
      @click="cycleOperator"
    >
      {{ operatorSymbols[currentOperator] }}
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
          class="absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5 px-1.5 py-0.5 bg-black/80 backdrop-blur-md border border-white/10 text-[9px] text-white rounded opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity whitespace-nowrap z-50"
        >
          {{ getStarTooltip(star) }}
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import RatingIcon from "./RatingIcon.vue";

const props = withDefaults(
  defineProps<{
    modelValue?: number[];
  }>(),
  {
    modelValue: () => [],
  },
);

const emit = defineEmits<(e: "update:modelValue", value: number[]) => void>();

type Operator = "=" | ">=" | "<=" | "!=";

const currentOperator = ref<Operator>("=");
const selectedValue = ref<number | null>(null); // 用于 >=, <=, != 单选模式
const selectedValues = ref<number[]>([]); // 用于 = 模式

const operatorSymbols: Record<Operator, string> = {
  "=": "=",
  ">=": "≥",
  "<=": "≤",
  "!=": "≠",
};

const operatorTitles: Record<Operator, string> = {
  "=": "模式: 精确匹配 (支持多选)",
  ">=": "模式: 大于等于",
  "<=": "模式: 小于等于",
  "!=": "模式: 不等于",
};

// 判断特定的星是否处于激活态
function isStarActive(star: number): boolean {
  if (currentOperator.value === "=") {
    return selectedValues.value.includes(star);
  }
  if (selectedValue.value === null) {
    return false;
  }
  switch (currentOperator.value) {
    case ">=":
      return star >= selectedValue.value;
    case "<=":
      return star <= selectedValue.value;
    case "!=":
      return star !== selectedValue.value;
    default:
      return false;
  }
}

// 获取各个星星的悬浮提示文案
function getStarTooltip(star: number): string {
  if (currentOperator.value === "=") {
    return star === 0 ? "无评分 (0 星)" : `${star} 星`;
  }
  return star === 0
    ? `${operatorSymbols[currentOperator.value]} 无评分`
    : `${operatorSymbols[currentOperator.value]} ${star} 星`;
}

// 切换操作符
function cycleOperator() {
  const ops: Operator[] = ["=", ">=", "<=", "!="];
  const idx = ops.indexOf(currentOperator.value);
  currentOperator.value = ops[(idx + 1) % ops.length];

  // 切换操作符后，重新校准选值并发出更新
  syncAndEmit();
}

// 点击星星的处理逻辑
function handleStarClick(star: number) {
  if (currentOperator.value === "=") {
    // 多选切换
    const idx = selectedValues.value.indexOf(star);
    if (idx >= 0) {
      selectedValues.value.splice(idx, 1);
    } else {
      selectedValues.value.push(star);
    }
  } else {
    // 单选范围切换
    if (selectedValue.value === star) {
      selectedValue.value = null; // 再次点击代表取消选择
    } else {
      selectedValue.value = star;
    }
  }

  syncAndEmit();
}

let lastEmitted = "[]";

// 解析外部传入的 modelValue 从而同步组件的内部状态
function parsePropsValue(val: number[]) {
  if (!val || val.length === 0) {
    selectedValue.value = null;
    selectedValues.value = [];
    currentOperator.value = "=";
    return;
  }

  const sorted = [...val].sort((a, b) => a - b);

  // 单元素数组直接解析为精确匹配 '=' 的多选单选，保持界面简洁直观
  if (sorted.length === 1) {
    currentOperator.value = "=";
    selectedValues.value = [...sorted];
    selectedValue.value = null;
    return;
  }

  const min = sorted[0];
  const max = sorted[sorted.length - 1];
  const isContinuous =
    sorted.length === max - min + 1 && sorted.every((v, i) => v === min + i);

  if (isContinuous) {
    if (max === 5) {
      currentOperator.value = ">=";
      selectedValue.value = min;
      selectedValues.value = [];
      return;
    }
    if (min === 0) {
      currentOperator.value = "<=";
      selectedValue.value = max;
      selectedValues.value = [];
      return;
    }
  }

  if (sorted.length === 5) {
    const all = [0, 1, 2, 3, 4, 5];
    const missing = all.filter((x) => !sorted.includes(x));
    if (missing.length === 1) {
      currentOperator.value = "!=";
      selectedValue.value = missing[0];
      selectedValues.value = [];
      return;
    }
  }

  currentOperator.value = "=";
  selectedValues.value = [...sorted];
  selectedValue.value = null;
}

// 同步状态并向父组件触发 v-model 更新
function syncAndEmit() {
  let result: number[] = [];

  if (currentOperator.value === "=") {
    result = [...selectedValues.value];
  } else if (selectedValue.value !== null) {
    const val = selectedValue.value;
    if (currentOperator.value === ">=") {
      for (let i = val; i <= 5; i++) result.push(i);
    } else if (currentOperator.value === "<=") {
      for (let i = 0; i <= val; i++) result.push(i);
    } else if (currentOperator.value === "!=") {
      for (let i = 0; i <= 5; i++) {
        if (i !== val) result.push(i);
      }
    }
  }

  const resultStr = JSON.stringify(result);
  lastEmitted = resultStr;
  emit("update:modelValue", result);
}

// 监听外部清空或修改事件，同步组件内部状态
watch(
  () => props.modelValue,
  (newVal) => {
    const newValStr = JSON.stringify(newVal || []);
    if (newValStr === lastEmitted) {
      return;
    }

    parsePropsValue(newVal || []);
    lastEmitted = newValStr;
  },
  { deep: true, immediate: true },
);
</script>
