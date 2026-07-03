<template>
  <div
    class="group relative bg-primary-800/40 hover:bg-primary-800/90 border rounded-xl overflow-hidden aspect-square cursor-pointer transition-all hover:scale-105 hover:shadow-lg hover:shadow-black/40 flex flex-col justify-between"
    :class="[
      isBulkMode && isSelected
        ? 'border-secondary-500 ring-2 ring-secondary-500/50 bg-primary-800/90 scale-105'
        : 'border-primary-800 hover:border-primary-600/80',
      isOutOfFilter ? 'border-yellow-600 border-2 border-dashed' : '',
    ]"
    @click="$emit('click', img, $event)"
  >
    <!-- 选中态的整体外框 overlay，防止被 overflow-hidden 裁剪或子元素遮挡 -->
    <div
      v-if="isBulkMode && isSelected"
      class="absolute inset-0 border-2 border-secondary-500 rounded-xl pointer-events-none z-10"
    ></div>
    <!-- 缩略图加载 -->
    <div
      class="w-full h-full relative overflow-hidden bg-black/10 flex items-center justify-center"
    >
      <!-- 左上角勾选徽章 -->
      <div
        v-if="isBulkMode"
        class="absolute top-2 left-2 z-10 w-6 h-6 rounded-full flex items-center justify-center transition-all duration-200 border cursor-pointer"
        :class="[
          isSelected
            ? 'bg-secondary-500 border-secondary-400 text-white shadow-[0_2px_8px_rgba(235,94,85,0.4)] scale-110'
            : 'bg-black/40 border-white/20 text-white/50 opacity-0 group-hover:opacity-100 hover:scale-105',
        ]"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24">
          <path :d="mdiCheck" fill="currentColor" stroke="currentColor" stroke-width="2" />
        </svg>
      </div>

      <img
        :src="img.url256 || img.url"
        :alt="img.filename"
        loading="lazy"
        class="object-cover w-full h-full select-none"
      />

      <!-- 评星与标签的悬浮徽章 -->
      <div
        class="absolute bottom-2 left-2 right-2 flex items-center justify-between pointer-events-none opacity-90 group-hover:opacity-100 transition-opacity"
      >
        <!-- 评分图标 -->
        <RatingIcon v-if="img.currentRating" :rating="img.currentRating" filled />

        <!-- 颜色标签：使用白色边框 + 黑色描边 ring 增强对比度，以防与图片背景颜色融为一体 -->
        <span
          v-if="img.label"
          class="w-3 h-3 rounded-full shadow-md border border-white ml-auto ring-1 ring-black/30"
          :style="{
            backgroundColor: PRESET_COLORS[img.label] || '#94a3b8',
          }"
          :title="img.label"
        ></span>
      </div>
    </div>

    <!-- 卡片底部的文件名遮罩 -->
    <div
      class="absolute inset-x-0 top-0 bg-linear-to-b from-black/80 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
    >
      <p class="text-xs text-white font-medium truncate" :title="img.filename">
        {{ img.filename }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { mdiCheck } from "@mdi/js";
import { PRESET_COLORS } from "@/composables/useImageLabel";
import RatingIcon from "./RatingIcon.vue";
import type { ImageFragment } from "@/graphql/generated";

defineProps<{
  img: ImageFragment;
  isBulkMode: boolean;
  isSelected: boolean;
  isOutOfFilter: boolean;
}>();

defineEmits<{
  click: [img: ImageFragment, event: MouseEvent];
}>();
</script>
