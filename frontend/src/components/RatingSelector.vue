<template>
  <div class="flex items-center gap-2" role="input">
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

const { readonly = false, clearable = false } = defineProps<{
  readonly?: boolean;
  clearable?: boolean;
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
      model.value = value[0] ?? (clearable ? null : 0);
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
            // 启用 clearable 时，取消选中已选中的星会将值设为 null
            if (clearable && selected && !e.target.checked) {
              model.value = null;
              return;
            }
            toggleStar(star.value, e.target.checked);
          }
        },
      } satisfies InputHTMLAttributes,
    };
  });
});
</script>
