import type { FunctionalComponent, TeleportProps } from "vue";
import { h, ref, useId } from "vue";
import ModalDialog from "@/components/ModalDialog.vue";
import useModal from "./useModal";

type PromiseInput<T> = T | Promise<T> | PromiseLike<T>;

// #region useModalDialog Composable 实现
/**
 * 封装对话框组件及其控制逻辑的 Composable
 */
export default function useModalDialog({
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
  const modal = useModal();
  const visible = ref(false);

  const scopeId = useId();

  // 包装后的对话框组件，内部渲染 ModalDialog 并传递事件与属性
  const component: FunctionalComponent<
    {
      containerClass?: string;
      teleport?: TeleportProps;
    },
    {
      afterLeave(): void;
    }
  > = function ModalDialogWrapper(props, ctx) {
    return h(
      modal.component,
      {
        teleport: props.teleport,
      },
      () =>
        h(
          ModalDialog,
          {
            ...ctx.attrs,
            visible: visible.value,
            scopeId,
            containerClass: props.containerClass,
            onAfterLeave: () => {
              modal.hide();
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
  component.props = ["containerClass", "teleport"];
  component.emits = ["afterLeave"];

  async function open() {
    visible.value = true;
    const e = new Event("open", { cancelable: true });
    await onWillOpen?.(e);
    if (e.defaultPrevented) {
      return;
    }
    modal.show();
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
    scopeId,
  };
}
// #endregion
