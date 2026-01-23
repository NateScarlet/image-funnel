<template>
  <div class="flex items-center gap-1">
    <label
      v-for="item in items"
      :key="item.key"
      v-bind="item.labelAttrs"
      class="w-8 h-8 flex items-center justify-center rounded transition-all hover:scale-110 cursor-pointer"
    >
      <RatingIcon v-bind="item.iconAttrs" />
      <input class="hidden" v-bind="item.inputAttrs" />
    </label>
  </div>
</template>

<script setup lang="ts">
import { computed, HTMLAttributes, InputHTMLAttributes, ref } from "vue";
import { STAR_CONFIGS } from "../utils/starConfig";
import RatingIcon from "./RatingIcon.vue";

const { readonly = false } = defineProps<{
  readonly?: boolean;
}>();

const model = defineModel<number | readonly number[]>();

const hoveredStar = ref<number | null>(null);

const arrayModel = computed({
  get() {
    return Array.isArray(model.value) ? model.value : [];
  },
  set(value) {
    if (Array.isArray(model.value)) {
      model.value = value;
    } else {
      model.value = value[0] ?? 0;
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
    arr.push(value);
  } else {
    arr.splice(arr.indexOf(value), 1);
  }
  arrayModel.value = arr;
}

const items = computed(() => {
  return STAR_CONFIGS.map((star) => {
    const selected = isSelected(star.value);
    const hovered = hoveredStar.value === star.value;

    return {
      key: star.value,
      selected,
      labelAttrs: {
        onMouseenter: () => (hoveredStar.value = star.value),
        onMouseleave: () => (hoveredStar.value = null),
      } satisfies HTMLAttributes,
      iconAttrs: {
        rating: star.value,
        filled: selected || (!readonly && hovered),
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
