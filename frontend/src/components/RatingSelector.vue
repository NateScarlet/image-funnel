<template>
  <div class="flex items-center gap-2" role="input">
    <button
      v-if="allowNull"
      type="button"
      :disabled="readonly"
      class="px-2 py-1 text-xs rounded border transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50 animate-pulse-subtle"
      :class="
        model === null
          ? 'bg-red-500/20 border-red-500 text-red-300 font-bold shadow-[0_0_8px_rgba(239,68,68,0.2)]'
          : 'border-primary-600/50 text-primary-300 hover:border-primary-400 hover:text-white'
      "
      @click="model = null"
    >
      不操作
    </button>
    <div class="flex items-center gap-1">
      <label
        v-for="item in items"
        :key="item.key"
        class="w-8 h-8 flex items-center justify-center rounded transition-all hover:scale-110 cursor-pointer"
      >
        <RatingIcon v-bind="item.iconAttrs" />
        <input class="hidden" v-bind="item.inputAttrs" />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, InputHTMLAttributes } from "vue";
import { STAR_CONFIGS } from "../utils/starConfig";
import RatingIcon from "./RatingIcon.vue";

const { readonly = false, allowNull = false } = defineProps<{
  readonly?: boolean;
  allowNull?: boolean;
}>();

const model = defineModel<number | null | readonly number[]>();

const arrayModel = computed({
  get() {
    if (model.value === null || model.value === undefined) {
      return [];
    }
    return Array.isArray(model.value) ? model.value : [model.value];
  },
  set(value) {
    if (Array.isArray(model.value)) {
      model.value = value;
    } else {
      model.value = value[0] ?? (allowNull ? null : 0);
    }
  },
});

function isSelected(value: number): boolean {
  return arrayModel.value.includes(value);
}

function toggleStar(value: number, force?: boolean) {
  if (readonly) return;

  const current = isSelected(value);
  const want = force ?? !current;
  if (current === want) {
    return;
  }
  const arr = [...arrayModel.value];
  if (want) {
    arr.unshift(value);
  } else {
    arr.splice(arr.indexOf(value), 1);
  }
  arrayModel.value = arr;
}

const items = computed(() => {
  return STAR_CONFIGS.map((star) => {
    const selected = isSelected(star.value);

    return {
      key: star.value,
      selected,
      iconAttrs: {
        rating: star.value,
        filled: selected,
      } satisfies InstanceType<typeof RatingIcon>["$props"],
      inputAttrs: {
        type: "checkbox",
        disabled: readonly,
        checked: selected,
        onChange: (e) => {
          if (e.target instanceof HTMLInputElement) {
            // 完全基于 UI 状态，保证符合用户预期
            toggleStar(star.value, e.target.checked);
          }
        },
      } satisfies InputHTMLAttributes,
    };
  });
});
</script>
