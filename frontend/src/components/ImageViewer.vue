<template>
  <div
    ref="containerRef"
    class="relative w-full h-full flex flex-col bg-slate-800 rounded-lg overflow-hidden"
    v-bind="containerAttrs"
  >
    <div
      ref="rendererRef"
      class="flex-1 flex items-center justify-center overflow-auto"
    >
      <img
        ref="imageRef"
        :src="src"
        :alt="alt"
        class="block max-w-none"
        v-bind="zoomAttrs"
      />
    </div>

    <div
      v-if="imageSize"
      class="flex items-center justify-center gap-2 bg-black/70 text-white text-xs px-2 py-1"
    >
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        title="缩小"
        @click="zoomOut"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
        </svg>
      </button>
      <span class="min-w-12 text-center">{{ zoomAsPercent }}%</span>
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        title="放大"
        @click="zoomIn"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
      </button>
      <div class="w-px h-4 bg-white/30 mx-1"></div>
      <span class="min-w-16">{{ imageSize.width }} × {{ imageSize.height }}</span>
      <div class="w-px h-4 bg-white/30 mx-1"></div>
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        :title="isFullscreen ? '退出全屏' : '全屏'"
        @click="handleToggleFullscreen"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path :d="isFullscreen ? mdiFullscreenExit : mdiFullscreen" />
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import useImageZoom from "../composables/useImageZoom";
import useGrabScroll from "../composables/useGrabScroll";
import useEventListeners from "../composables/useEventListeners";
import useAsyncTask from "../composables/useAsyncTask";
import useElementFullscreen from "../composables/useElementFullscreen";
import { mdiFullscreen, mdiFullscreenExit } from "@mdi/js";

interface Props {
  src: string;
  alt?: string;
  naturalWidth?: number;
  naturalHeight?: number;
}

const props = withDefaults(defineProps<Props>(), {
  alt: "",
  naturalWidth: undefined,
  naturalHeight: undefined,
});

const containerRef = ref<HTMLElement>();
const rendererRef = ref<HTMLElement>();
const imageRef = ref<HTMLImageElement>();

const { toggleFullscreen, isFullscreen } = useElementFullscreen(containerRef);

function handleToggleFullscreen() {
  toggleFullscreen();
}

const { result: size, restart: updateSize } = useAsyncTask({
  args: () => {
    const img = imageRef.value;
    if (img) {
      return [props.src, img];
    }
  },
  task: async (_, img) => {
    if (img.naturalWidth === 0 && img.naturalHeight === 0) {
      await img.decode();
    }
    return {
      width: img.naturalWidth,
      height: img.naturalHeight,
    };
  },
});

useEventListeners(imageRef, (ctx) => {
  ctx.on("load", () => {
    updateSize();
  });
});

const imageSize = computed(() => {
  if (props.naturalWidth && props.naturalHeight) {
    return { width: props.naturalWidth, height: props.naturalHeight };
  }
  return size.value;
});

const zoom = useImageZoom({
  container: containerRef,
  renderer: rendererRef,
  size: imageSize,
});

const { containerAttrs, zoomAsPercent, toggleZoom, zoomIn, zoomOut, zoomAttrs } = zoom;

useGrabScroll(() => {
  if (!zoom.fitContainer.value) {
    return rendererRef.value;
  }
});

let initialPinchDistance = 0;
let initialZoom = 1;

function getTouchDistance(touches: TouchList): number {
  if (touches.length < 2) return 0;
  const dx = touches[0].clientX - touches[1].clientX;
  const dy = touches[0].clientY - touches[1].clientY;
  return Math.sqrt(dx * dx + dy * dy);
}

useEventListeners(containerRef, ({ on }) => {
  on(
    "touchstart",
    (e: Event) => {
      const touchEvent = e as TouchEvent;
      if (touchEvent.touches.length === 2) {
        e.preventDefault();
        initialPinchDistance = getTouchDistance(touchEvent.touches);
        initialZoom = zoom.zoom.value;
      }
    },
    { passive: false }
  );

  on(
    "touchmove",
    (e: Event) => {
      const touchEvent = e as TouchEvent;
      if (touchEvent.touches.length === 2) {
        e.preventDefault();
        const currentDistance = getTouchDistance(touchEvent.touches);
        if (initialPinchDistance > 0) {
          const scale = currentDistance / initialPinchDistance;
          zoom.zoom.value = Math.max(0.1, Math.min(10, initialZoom * scale));
        }
      }
    },
    { passive: false }
  );

  on("touchend", (e: Event) => {
    const touchEvent = e as TouchEvent;
    if (touchEvent.touches.length < 2) {
      initialPinchDistance = 0;
    }
  });
});

defineExpose({
  zoomIn,
  zoomOut,
  toggleZoom,
  zoom: zoom.zoom,
  fitContainer: zoom.fitContainer,
  zoomAsPercent,
});
</script>
