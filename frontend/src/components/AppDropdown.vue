<template>
  <div class="relative inline-block" ref="referenceEl">
    <!-- 触发器插槽，将控制状态与 toggle/close 方法暴露给调用者 -->
    <slot name="trigger" :isOpen="isOpen" :toggle="toggle" :close="close"></slot>

    <!-- 使用 Teleport 将下拉浮层挂载到 body 下，彻底规避父级容器 overflow 截断问题 -->
    <Teleport to="body">
      <Transition name="dropdown-fade">
        <div
          v-if="isOpen"
          ref="floatingEl"
          :style="floatingStyles"
          :class="[
            'z-60 rounded-xl border border-primary-700/60 bg-primary-900/95 p-2 shadow-xl backdrop-blur-md',
            contentClass,
          ]"
        >
          <!-- 下拉内容插槽，支持调用侧内部主动关闭（如点击选项后） -->
          <slot name="content" :close="close"></slot>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useFloating, offset as floatingOffset, flip, shift, autoUpdate } from "@floating-ui/vue";
import type { Placement } from "@floating-ui/vue";
import useEventListeners from "@/composables/useEventListeners";

// #region 属性定义
const props = withDefaults(
  defineProps<{
    /** 定位方向，例如 top-end, bottom-start 等 */
    placement?: Placement;
    /** 与触发器元素的主轴偏移像素 */
    offset?: number;
    /** 是否禁用下拉唤起 */
    disabled?: boolean;
    /** 浮层内容容器的额外样式类 */
    contentClass?: string;
  }>(),
  {
    placement: "bottom-start",
    offset: 8,
    disabled: false,
    contentClass: "",
  }
);
// #endregion

// #region 状态与定位控制
const isOpen = ref(false);
const referenceEl = ref<HTMLElement | null>(null);
const floatingEl = ref<HTMLElement | null>(null);

const { floatingStyles } = useFloating(referenceEl, floatingEl, {
  placement: computed(() => props.placement),
  strategy: "fixed", // 采用 fixed 定位确保悬浮菜单层在 fixed 布局的 BatchBar 上定位准确
  whileElementsMounted: autoUpdate,
  middleware: [
    floatingOffset(() => ({ mainAxis: props.offset })),
    flip(),
    shift(),
  ],
});
// #endregion

// #region 事件处理器与状态维护
function toggle() {
  if (props.disabled) return;
  isOpen.value = !isOpen.value;
}

function close() {
  isOpen.value = false;
}

// 侦听禁用状态：若在打开时变为禁用，应立刻关闭菜单防止残留
watch(
  () => props.disabled,
  (disabledVal) => {
    if (disabledVal) {
      close();
    }
  }
);

// 点击外部关闭：检测点击区域是否在触发器或浮层内容之外
function onClickOutside(event: MouseEvent) {
  if (!isOpen.value) return;
  const target = event.target as HTMLElement;
  if (
    referenceEl.value?.contains(target) ||
    floatingEl.value?.contains(target)
  ) {
    return;
  }
  close();
}

// 按下 Esc 键关闭下拉框
function onKeyDown(event: KeyboardEvent) {
  if (!isOpen.value) return;
  if (event.key === "Escape") {
    close();
  }
}

// 全局事件注册管理（组件销毁时会自动清理）
useEventListeners(document, ({ on }) => {
  on("click", onClickOutside);
  on("keydown", onKeyDown);
});
// #endregion
</script>

<style scoped>
/* 优雅的淡入与微移移动画 */
.dropdown-fade-enter-active,
.dropdown-fade-leave-active {
  transition: opacity 0.15s cubic-bezier(0.16, 1, 0.3, 1),
    transform 0.15s cubic-bezier(0.16, 1, 0.3, 1);
}
.dropdown-fade-enter-from,
.dropdown-fade-leave-to {
  opacity: 0;
  transform: translateY(4px) scale(0.95);
}
</style>
