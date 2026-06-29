<template>
  <input
    ref="inputEl"
    v-model.lazy="valueAsString"
    type="text"
    class="text-center bg-transparent border-0 focus:ring-0 focus:outline-none text-white font-mono [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
    @keydown="onKeydown"
    @wheel="onWheel"
    @blur="onBlur"
    @focus="onFocus"
  />
</template>

<script setup lang="ts">
import { computed, ref } from "vue";

const props = withDefaults(
  defineProps<{
    modelValue: number | undefined;
    min?: number;
    max?: number;
    step?: number;
  }>(),
  {
    min: undefined,
    max: undefined,
    step: 0.1,
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: number | undefined): void;
  (e: "blur"): void;
  (e: "focus", event: FocusEvent): void;
}>();

const inputEl = ref<HTMLInputElement | null>(null);

function clampValue(val: number): number {
  let v = val;
  if (props.min !== undefined && v < props.min) {
    v = props.min;
  }
  if (props.max !== undefined && v > props.max) {
    v = props.max;
  }
  // 保持合理的浮点数精度（避免如 0.1 + 0.2 = 0.30000000000000004 的偏差）
  return Math.round(v * 100) / 100;
}

const valueAsString = computed({
  get() {
    if (props.modelValue === undefined || isNaN(props.modelValue)) {
      return "";
    }
    return props.modelValue.toString();
  },
  set(val: string) {
    const parsed = parseFloat(val);
    if (isNaN(parsed)) {
      emit("update:modelValue", undefined);
    } else {
      emit("update:modelValue", clampValue(parsed));
    }
  },
});

function adjustValue(delta: number) {
  const current = props.modelValue ?? 0;
  const next = current + delta * props.step;
  emit("update:modelValue", clampValue(next));
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowUp") {
    e.preventDefault();
    adjustValue(1);
  } else if (e.key === "ArrowDown") {
    e.preventDefault();
    adjustValue(-1);
  } else if (e.key === "Enter" || e.key === "Escape") {
    e.preventDefault();
    inputEl.value?.blur();
  }
}

function onWheel(e: WheelEvent) {
  if (document.activeElement !== e.target) {
    return;
  }
  e.preventDefault();
  adjustValue(e.deltaY < 0 ? 1 : -1);
}

function onBlur() {
  emit("blur");
}

function onFocus(e: FocusEvent) {
  (e.target as HTMLInputElement).select();
  emit("focus", e);
}

defineExpose({
  focus: () => inputEl.value?.focus(),
  blur: () => inputEl.value?.blur(),
});
</script>
