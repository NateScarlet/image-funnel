<template>
  <svg 
    class="w-6 h-6" 
    viewBox="0 0 24 24" 
    fill="currentColor"
    :class="getStarColorClass(rating)"
  >
    <path :d="filled ? starConfig?.filledIcon : starConfig?.outlineIcon" />
    <text 
      v-if="filled"
      x="12" 
      y="16" 
      text-anchor="middle" 
      font-size="9" 
      font-weight="bold" 
      fill="white"
    >{{ rating }}</text>
  </svg>
</template>

<script setup lang="ts">
import { STAR_CONFIGS } from '../utils/starConfig'
import { getStarColorClass } from '../utils/starConfig'

interface Props {
  rating: number
  filled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  filled: false
})

const starConfig = STAR_CONFIGS.find(s => s.value === props.rating)
</script>
