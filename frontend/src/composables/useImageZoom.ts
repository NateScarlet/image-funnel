import type { MaybeRefOrGetter, StyleValue } from "vue";
import { computed, ref, toValue } from "vue";
import roundDecimal from "@/utils/roundDecimal";
import useElementSize from "./useElementSize";
import usePollingV3 from "./usePolling";

export default function useImageZoom({
  size,
  container,
  levels = [0.5, 0.6, 0.7, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3],
  fallbackZoomInStep = () => 1,
  fallbackZoomOutStep = (v) => {
    return 10 ** Math.round(Math.log10(v) - 1);
  },
  renderer = container,
}: {
  /** 滚动容器 */
  container: MaybeRefOrGetter<HTMLElement | null | undefined>;
  /** 图像原始尺寸 */
  size: MaybeRefOrGetter<{ width: number; height: number } | null | undefined>;
  /** 预定义的缩放级别 */
  levels?: readonly number[];
  fallbackZoomInStep?: (current: number) => number;
  fallbackZoomOutStep?: (current: number) => number;
  /** 实际渲染图片的元素，应该图片本身一样大（可以滚动），默认为 container */
  renderer?: MaybeRefOrGetter<HTMLElement | null | undefined>;
}) {
  const { contentBoxWidth, contentBoxHeight } = useElementSize(container);
  const fitContainerBuffer = ref<boolean>();
  const zoomBuffer = ref<number>();

  // 缩放后保持滚动位置不变
  const scrollAnchor = ref<{
    viewport: { x: number; y: number };
    image?: { x: number; y: number };
  }>();

  usePollingV3({
    update: () => {
      if (!scrollAnchor.value) {
        return;
      }
      const el = toValue(container);
      if (!el) {
        return;
      }
      const { viewport, image = { x: 0.5, y: 0.5 } } = scrollAnchor.value;

      // 像素点位置
      const imageX = image.x * el.scrollWidth;
      const imageY = image.y * el.scrollHeight;

      // 相对于视口左上角的偏移（偏移越大，需要实际滚动就越小）
      const offsetX = viewport.x * el.clientWidth;
      const offsetY = viewport.y * el.clientHeight;

      const scrollLeft = imageX - offsetX;
      const scrollTop = imageY - offsetY;
      if (
        Math.abs(scrollLeft - el.scrollLeft) >= 1 ||
        Math.abs(scrollTop - el.scrollTop) >= 1
      ) {
        el.scrollLeft = imageX - offsetX;
        el.scrollTop = imageY - offsetY;
      }
    },
    paused: () => !scrollAnchor.value,
  });

  const fitContainerScale = computed(() => {
    const value = toValue(size);
    if (!value || value.width <= 0 || value.height <= 0) {
      return;
    }
    return Math.min(
      contentBoxWidth.value / value.width,
      contentBoxHeight.value / value.height,
    );
  });
  const zoomModel = computed({
    get() {
      if (zoomBuffer.value) {
        return zoomBuffer.value;
      }
      const value = toValue(size);
      if (!value) {
        return 1;
      }
      const scale = fitContainerScale.value;
      if (scale == null) {
        return 1;
      }
      if (fitContainerBuffer.value) {
        // fit container
        return scale;
      }
      // scale down
      return Math.min(1, scale);
    },
    set(v) {
      fitContainerBuffer.value = undefined;
      zoomBuffer.value = v;
    },
  });
  const fitContainerModel = computed({
    get() {
      return (
        fitContainerBuffer.value ?? zoomModel.value === fitContainerScale.value
      );
    },
    set(v) {
      fitContainerBuffer.value = v;
    },
  });

  function next(v: number, direction: 1 | -1) {
    const match =
      direction > 0 ? levels.find((i) => i > v) : levels.findLast((i) => i < v);
    if (match != null) {
      return match;
    }
    if (direction > 0) {
      return v + fallbackZoomInStep(v);
    }
    return v - fallbackZoomOutStep(v);
  }

  function viewportCenterScrollAnchor() {
    const el = toValue(container);
    if (!el) {
      return { viewport: { x: 0.5, y: 0.5 } };
    }
    return {
      viewport: {
        x:
          el.scrollWidth > el.clientWidth
            ? roundDecimal(
                (el.scrollLeft - 0.5 * (el.scrollWidth - el.clientWidth)) /
                  el.scrollWidth +
                  0.5,
                2,
              )
            : 0.5,
        y:
          el.scrollHeight > el.clientHeight
            ? roundDecimal(
                (el.scrollTop - 0.5 * (el.scrollHeight - el.clientHeight)) /
                  el.scrollHeight +
                  0.5,
                2,
              )
            : 0.5,
      },
    };
  }

  function anchorFromClientPosition(pos: { clientX: number; clientY: number }) {
    const el = toValue(renderer);
    if (!el) {
      return;
    }
    const { left, top } = el.getBoundingClientRect();
    const imageX = pos.clientX - left + el.scrollLeft;
    const imageY = pos.clientY - top + el.scrollTop;
    return {
      viewport: {
        x: (imageX - el.scrollLeft) / el.clientWidth,
        y: (imageY - el.scrollTop) / el.clientHeight,
      },
      image: {
        x: imageX / el.scrollWidth,
        y: imageY / el.scrollHeight,
      },
    };
  }

  function zoomIn() {
    if (!scrollAnchor.value) {
      scrollAnchor.value = viewportCenterScrollAnchor();
    }
    zoomModel.value = next(zoomModel.value, 1);
  }
  function zoomOut() {
    if (!scrollAnchor.value) {
      scrollAnchor.value = viewportCenterScrollAnchor();
    }
    zoomModel.value = next(zoomModel.value, -1);
  }

  const zoomAsPercentModel = computed({
    get() {
      return Math.round(zoomModel.value * 100);
    },
    set(v) {
      zoomModel.value = v / 100;
    },
  });

  const zoomAttrs = computed(() => {
    const v = toValue(size);

    return {
      onTransitionstart: () => {
        if (!scrollAnchor.value) {
          scrollAnchor.value = viewportCenterScrollAnchor();
        }
      },
      onTransitionend: () => {
        scrollAnchor.value = undefined;
      },
      style: {
        ...(v && v.width * v.height > 0
          ? {
              width: `${v.width * zoomModel.value}px`,
              height: `${v.height * zoomModel.value}px`,
            }
          : {
              width: `${zoomAsPercentModel.value}%`,
              height: `${zoomAsPercentModel.value}%`,
            }),
        transitionProperty: "width,height",
        transitionDuration: "0.3s",
        transitionTimingFunction: "ease-in-out",
      } as StyleValue,
    };
  });

  function toggleZoom(force?: boolean) {
    if (force === true || !fitContainerModel.value) {
      zoomBuffer.value = undefined;
      fitContainerModel.value = true;
    } else {
      zoomModel.value = 1;
    }
  }

  const containerAttrs = computed(() => {
    return {
      onWheel: (e: WheelEvent) => {
        if (!e.ctrlKey && !e.shiftKey && !e.altKey && e.deltaY) {
          e.preventDefault();
          if (!scrollAnchor.value) {
            scrollAnchor.value = anchorFromClientPosition(e);
          }
          zoomModel.value = next(zoomModel.value, e.deltaY < 0 ? 1 : -1);
        }
      },
      onDblclick(e: MouseEvent) {
        const el = toValue(renderer);
        if (el) {
          scrollAnchor.value = anchorFromClientPosition(e);
        }
        toggleZoom();
      },
    };
  });
  return {
    zoom: zoomModel,
    zoomAsPercent: zoomAsPercentModel,
    zoomIn,
    zoomOut,
    zoomAttrs,
    fitContainer: fitContainerModel,
    contentBoxHeight,
    contentBoxWidth,
    toggleZoom,
    containerAttrs,
  };
}
