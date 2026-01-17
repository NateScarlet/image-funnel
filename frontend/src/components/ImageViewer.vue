<template>
  <div
    ref="containerRef"
    class="relative w-full h-full overflow-hidden bg-slate-800 rounded-lg"
    v-bind="zoom.containerAttrs"
  >
    <div
      ref="rendererRef"
      class="absolute inset-0 flex items-center justify-center overflow-auto"
      v-bind="zoomAttrs"
    >
      <img
        v-if="imageSize"
        ref="imageRef"
        :src="src"
        :alt="alt"
        class="block object-contain"
        v-bind="zoom.zoomAttrs"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from "vue";
import useImageZoom from "../composables/useImageZoom";
import useDragInput from "../composables/useDragInput";
import useEventListeners from "../composables/useEventListeners";

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

const imageSize = computed(() => {
  if (props.naturalWidth && props.naturalHeight) {
    return { width: props.naturalWidth, height: props.naturalHeight };
  }
  if (imageRef.value) {
    return {
      width: imageRef.value.naturalWidth,
      height: imageRef.value.naturalHeight,
    };
  }
  return null;
});

const zoom = useImageZoom({
  container: containerRef,
  renderer: rendererRef,
  size: imageSize,
});

const scrollX = ref(0);
const scrollY = ref(0);

watch(
  () => props.src,
  () => {
    if (rendererRef.value) {
      rendererRef.value.scrollLeft = scrollX.value;
      rendererRef.value.scrollTop = scrollY.value;
    }
  }
);

watch(rendererRef, (el) => {
  if (el) {
    el.scrollLeft = scrollX.value;
    el.scrollTop = scrollY.value;
  }
});

const { dragging } = useDragInput({
  el: () => rendererRef.value,
  x: scrollX,
  y: scrollY,
  cursorStyle: () => (dragging.value ? "grabbing" : "grab"),
});

const zoomAttrs = computed(() => ({
  style: {
    cursor: dragging.value ? "grabbing" : "grab",
    userSelect: "none" as const,
  },
}));

let initialPinchDistance = 0;
let initialZoom = 1;

function getTouchDistance(touches: TouchList): number {
  if (touches.length < 2) return 0;
  const dx = touches[0].clientX - touches[1].clientX;
  const dy = touches[0].clientY - touches[1].clientY;
  return Math.sqrt(dx * dx + dy * dy);
}

useEventListeners(containerRef, ({ on }) => {
  on("touchstart", (e: Event) => {
    const touchEvent = e as TouchEvent;
    if (touchEvent.touches.length === 2) {
      e.preventDefault();
      initialPinchDistance = getTouchDistance(touchEvent.touches);
      initialZoom = zoom.zoom.value;
    }
  }, { passive: false });

  on("touchmove", (e: Event) => {
    const touchEvent = e as TouchEvent;
    if (touchEvent.touches.length === 2) {
      e.preventDefault();
      const currentDistance = getTouchDistance(touchEvent.touches);
      if (initialPinchDistance > 0) {
        const scale = currentDistance / initialPinchDistance;
        zoom.zoom.value = Math.max(0.1, Math.min(10, initialZoom * scale));
      }
    }
  }, { passive: false });

  on("touchend", (e: Event) => {
    const touchEvent = e as TouchEvent;
    if (touchEvent.touches.length < 2) {
      initialPinchDistance = 0;
    }
  });
});

onMounted(() => {
  if (rendererRef.value) {
    scrollX.value = rendererRef.value.scrollLeft;
    scrollY.value = rendererRef.value.scrollTop;
  }
});

onUnmounted(() => {
  if (rendererRef.value) {
    scrollX.value = rendererRef.value.scrollLeft;
    scrollY.value = rendererRef.value.scrollTop;
  }
});

defineExpose({
  zoomIn: zoom.zoomIn,
  zoomOut: zoom.zoomOut,
  toggleZoom: zoom.toggleZoom,
  zoom: zoom.zoom,
  fitContainer: zoom.fitContainer,
});
</script>
