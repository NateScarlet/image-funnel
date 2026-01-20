<template>
  <div class="star-selector">
    <div class="flex items-center gap-1">
      <button
        v-for="star in stars"
        :key="star.value"
        type="button"
        class="w-8 h-8 flex items-center justify-center rounded transition-all hover:scale-110 active:scale-95"
        :disabled="disabled"
        @click="toggleStar(star.value)"
        @mouseenter="hoveredStar = star.value"
        @mouseleave="hoveredStar = null"
      >
        <RatingIcon
          :rating="star.value"
          :filled="isSelected(star.value) || hoveredStar === star.value"
        />
      </button>
    </div>
    <div v-if="label" class="text-sm text-slate-400 mt-1">{{ label }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { STAR_CONFIGS, type StarConfig } from "../utils/starConfig";
import RatingIcon from "./RatingIcon.vue";

interface Props {
  mode?: "single" | "multi";
  disabled?: boolean;
  label?: string;
}

const props = withDefaults(defineProps<Props>(), {
  mode: "single",
  disabled: false,
  label: "",
});

const model = defineModel<number | readonly number[]>();

const stars: StarConfig[] = STAR_CONFIGS;
const hoveredStar = ref<number | null>(null);

function isSelected(value: number): boolean {
  if (props.mode === "single") {
    return model.value === value;
  } else {
    return Array.isArray(model.value) && model.value.includes(value);
  }
}

function toggleStar(value: number) {
  if (props.disabled) return;

  if (props.mode === "single") {
    model.value = value;
  } else {
    const current = Array.isArray(model.value) ? [...model.value] : [];
    const index = current.indexOf(value);

    if (index === -1) {
      current.push(value);
    } else {
      current.splice(index, 1);
    }

    model.value = current;
  }
}
</script>
