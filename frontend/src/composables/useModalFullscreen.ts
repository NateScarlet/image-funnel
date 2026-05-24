import type { FunctionalComponent, TeleportProps } from "vue";
import { h, ref } from "vue";
import ModalFullscreen from "@/components/ModalFullscreen.vue";
import useModal from "./useModal";

type PromiseInput<T> = T | Promise<T> | PromiseLike<T>;

/**
 * 封装全屏弹窗组件及其控制逻辑的 Composable
 */
export default function useModalFullscreen({
  onDidOpen,
  onDidClose,
  onWillOpen,
  onWillClose,
}: {
  onDidOpen?: () => void;
  onDidClose?: () => void;
  onWillOpen?: (e: Event) => PromiseInput<void>;
  onWillClose?: (e: Event) => PromiseInput<void>;
} = {}) {
  const modal = useModal(() => close());
  const visible = ref(false);

  // 包装后的全屏对话框组件，内部渲染 ModalFullscreen 并传递事件与属性
  const component: FunctionalComponent<
    {
      teleport?: TeleportProps;
    },
    {
      afterLeave(): void;
    }
  > = function ModalFullscreenWrapper(props, ctx) {
    return h(
      modal.component,
      {
        teleport: props.teleport,
      },
      () =>
        h(
          ModalFullscreen,
          {
            ...ctx.attrs,
            visible: visible.value,
            onAfterLeave: () => {
              modal.close();
              onDidClose?.();
              ctx.emit("afterLeave");
            },
            "onUpdate:visible": async (v: boolean) => {
              if (!v) {
                await close();
              } else {
                visible.value = true;
              }
            },
          },
          ctx.slots.default,
        ),
    );
  };

  component.inheritAttrs = false;
  component.props = ["teleport"];
  component.emits = ["afterLeave"];

  async function open() {
    visible.value = true;
    const e = new Event("open", { cancelable: true });
    await onWillOpen?.(e);
    if (e.defaultPrevented) {
      return;
    }
    modal.open();
    onDidOpen?.();
  }

  async function close() {
    const e = new Event("close", { cancelable: true });
    await onWillClose?.(e);
    if (e.defaultPrevented) {
      return false;
    }
    visible.value = false;
    return true;
  }

  return {
    component,
    open,
    close,
    visible,
  };
}
