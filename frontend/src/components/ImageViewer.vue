<template>
  <div
    ref="rootEl"
    class="flex flex-col bg-primary-800 rounded-lg overflow-hidden isolate contain-layout"
  >
    <div
      ref="containerRef"
      class="flex-1 w-full flex items-center [scrollbar-gutter:stable] overflow-auto"
      :class="{ 'pointer-events-none': locked }"
      v-bind="!locked ? containerAttrs : {}"
    >
      <!-- zoom -->
      <div v-bind="zoomAttrs" class="contain-layout m-auto flex-none">
        <img
          ref="imgEl"
          :src="src"
          :alt="image.filename"
          :data-image-id="image.id"
          class="object-contain w-full h-full"
          @loadstart="onLoadStart"
          @load="updateLoaded"
          @error="updateLoaded"
        />
      </div>
      <!-- 加载提示 -->
      <Transition
        enter-active-class="transition duration-100 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <template v-if="isSlowLoading">
          <div
            class="absolute inset-0 flex items-center justify-center bg-primary-900/25 backdrop-blur-sm"
          >
            <svg
              class="w-12 h-12 animate-spin text-secondary-400"
              viewBox="0 0 24 24"
              fill="none"
            >
              <path
                :d="mdiLoading"
                fill="none"
                stroke="currentColor"
                stroke-width="3"
                stroke-linecap="round"
              />
            </svg>
          </div>
        </template>
      </Transition>
    </div>

    <!-- 图片尺寸和缩放操作 -->
    <div
      data-no-gesture
      class="flex-none flex items-center justify-center flex-wrap gap-2 bg-black/70 text-white text-xs px-2 py-1"
    >
      <button
        class="hover:bg-white/20 w-6 h-6 items-center justify-center rounded transition-colors hidden md:flex"
        title="缩小"
        @click="zoomOut"
      >
        <svg
          class="w-4 h-4"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M20 12H4"
          />
        </svg>
      </button>
      <span
        class="min-w-12 text-center cursor-pointer"
        :class="isFullscreen ? 'hidden md:block' : ''"
        @click="zoomAsPercent = 100"
        >{{ zoomAsPercent }}%</span
      >
      <button
        class="hover:bg-white/20 w-6 h-6 items-center justify-center rounded transition-colors hidden md:flex"
        title="放大"
        @click="zoomIn"
      >
        <svg
          class="w-4 h-4"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M12 4v16m8-8H4"
          />
        </svg>
      </button>
      <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
      <span class="min-w-16">{{ image.width }} × {{ image.height }}</span>
      <div class="w-px h-4 bg-white/30 mx-1 hidden md:block"></div>
      <slot name="info" :is-fullscreen />
      <div
        class="w-px h-4 bg-white/30 mx-1"
        :class="isFullscreen ? 'hidden md:block' : ''"
      ></div>
      <button
        class="hover:bg-white/20 w-6 h-6 flex items-center justify-center rounded transition-colors"
        :class="isFullscreen ? 'hidden md:block' : ''"
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
import { ref, computed, useTemplateRef, shallowRef, watch } from "vue";
import useImageZoom from "../composables/useImageZoom";
import useGrabScroll from "../composables/useGrabScroll";
import useEventListeners from "../composables/useEventListeners";
import useElementFullscreen from "../composables/useElementFullscreen";
import { mdiFullscreen, mdiFullscreenExit, mdiLoading } from "@mdi/js";
import type { ImageFragment } from "@/graphql/generated";
import { getImageUrlByZoom } from "@/utils/image";
import useCurrentTime from "@/composables/useCurrentTime";
import Time from "@/utils/Time";
import useAsyncTask from "@/composables/useAsyncTask";

const {
  image,
  nextImages = [],
  locked = false,
  allowPan = () => true,
} = defineProps<{
  image: ImageFragment;
  nextImages?: ImageFragment[];
  locked?: boolean;
  allowPan?: (e: PointerEvent) => boolean;
}>();

const containerRef = ref<HTMLElement>();
const rootEl = ref<HTMLElement>();

const { toggleFullscreen, isFullscreen } = useElementFullscreen(rootEl);

function handleToggleFullscreen() {
  toggleFullscreen();
}

const zoom = useImageZoom({
  container: containerRef,
  size: () => image,
  allowTransition: () => loaded.value,
});
const {
  containerAttrs,
  zoomAsPercent,
  toggleZoom,
  zoomIn,
  zoomOut,
  zoomAttrs,
} = zoom;

const src = computed(() => getImageUrlByZoom(image, zoom.zoom.value));

const activeContainer = computed(() => (locked ? null : containerRef.value));

// 主动按顺序预加载后续图片
useAsyncTask({
  args() {
    return [
      [
        getImageUrlByZoom(image, zoom.zoom.value),
        ...nextImages.map((img) => getImageUrlByZoom(img, zoom.zoom.value)),
      ],
    ];
  },
  async task(urls, ctx) {
    for (const url of urls) {
      if (ctx.signal().aborted) {
        return;
      }
      const img = new window.Image();
      img.src = url;
      try {
        await img.decode();
      } catch {
        // ignore
      }
    }
  },
});

useGrabScroll(
  () => {
    if (!zoom.fitContainer.value) {
      return activeContainer.value;
    }
  },
  {
    beforeStart: allowPan,
  },
);

const imgEl = useTemplateRef("imgEl");
const loadedId = ref("");
const loaded = computed(() => loadedId.value === image.id);
const lastLoading = shallowRef({ image, startAt: Time.now() });
const { currentTime, refreshOn } = useCurrentTime();
const slowLoadingTimeoutMs = 100;
const isSlowLoading = computed(
  () =>
    !loaded.value &&
    lastLoading.value.image.id === image.id &&
    currentTime.value.sub(lastLoading.value.startAt) > slowLoadingTimeoutMs,
);

watch(
  () => image.id,
  () => {
    lastLoading.value = { image, startAt: Time.now() };
  },
);

function onLoadStart() {
  if (lastLoading.value.image.id !== image.id) {
    lastLoading.value = { image, startAt: Time.now() };
  }
}
refreshOn(() => lastLoading.value.startAt.add(slowLoadingTimeoutMs + 1));
function updateLoaded() {
  const el = imgEl.value;
  if (el?.complete) {
    loadedId.value = el.dataset.imageId || "";
  }
}

let initialPinchDistance = 0;
let initialZoom = 1;

function getTouchDistance(touches: TouchList): number {
  if (touches.length < 2) return 0;
  const dx = touches[0].clientX - touches[1].clientX;
  const dy = touches[0].clientY - touches[1].clientY;
  return Math.sqrt(dx * dx + dy * dy);
}

function getTouchCenter(touches: TouchList) {
  return {
    clientX: (touches[0].clientX + touches[1].clientX) / 2,
    clientY: (touches[0].clientY + touches[1].clientY) / 2,
  };
}

let initialAnchorImage: { x: number; y: number } | undefined;

useEventListeners(containerRef, ({ on }) => {
  on(
    "touchstart",
    (e) => {
      if (locked) return;
      if (e.touches.length === 2) {
        e.preventDefault();
        e.stopPropagation();
        initialPinchDistance = getTouchDistance(e.touches);
        initialZoom = zoom.zoom.value;

        // Set anchor based on initial finger position
        const center = getTouchCenter(e.touches);
        const anchor = zoom.anchorFromClientPosition(center);
        if (anchor) {
          zoom.scrollAnchor.value = anchor;
          initialAnchorImage = anchor.image;
        }
      }
    },
    { passive: false },
  );

  on(
    "touchmove",
    (e) => {
      if (e.touches.length === 2) {
        e.preventDefault();
        e.stopPropagation();
        const currentDistance = getTouchDistance(e.touches);
        if (initialPinchDistance > 0) {
          const scale = currentDistance / initialPinchDistance;
          zoom.zoom.value = Math.max(0.1, Math.min(10, initialZoom * scale));

          // Update anchor to track finger movement (panning while zooming)
          const center = getTouchCenter(e.touches);
          const currentAnchor = zoom.anchorFromClientPosition(center);
          if (currentAnchor && initialAnchorImage) {
            zoom.scrollAnchor.value = {
              viewport: currentAnchor.viewport,
              image: initialAnchorImage,
            };
          }
        }
      }
    },
    { passive: false },
  );

  on("touchend", (e) => {
    if (e.touches.length < 2) {
      initialPinchDistance = 0;
      zoom.scrollAnchor.value = undefined;
      initialAnchorImage = undefined;
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
