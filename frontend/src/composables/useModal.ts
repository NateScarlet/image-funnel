import type {
  FunctionalComponent,
  InjectionKey,
  MaybeRefOrGetter,
  RendererElement,
  TeleportProps,
} from "vue";
import {
  Comment,
  Teleport,
  Transition,
  h,
  inject,
  provide,
  ref,
  toValue,
  onUnmounted,
} from "vue";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import useEventListeners from "@/composables/useEventListeners";
import randomUUID from "@/utils/randomUUID";

// #region 渲染目标依赖注入配置
const rendererKey: InjectionKey<() => string | RendererElement> =
  Symbol("modalRenderer");

/**
 * 提供模态框渲染挂载的自定义容器
 * @param renderer 目标容器或其获取器
 */
export function provideModalRenderer(
  renderer: MaybeRefOrGetter<string | RendererElement>,
) {
  const parent = inject(rendererKey);
  provide(rendererKey, () => toValue(renderer) ?? parent?.());
}
// #endregion

// #region 核心 useModal Composable 实现
/**
 * 基础模态框挂载与动画状态控制 Composable
 */
export default function useModal(
  onPopState?: () => Promise<boolean> | boolean | undefined,
) {
  const defaultRenderer = useFullscreenRendererElement();
  const skipRender = ref(true);
  const visible = ref(false);

  // 内部维护物理返回键拦截所需的状态
  const stateKey = `modal-${randomUUID()}`;
  let hasPushState = false;

  // 使用统一的事件监听 Composable 挂载并自动清理 popstate 监听器
  useEventListeners(window, ({ on }) => {
    on("popstate", async (event: PopStateEvent) => {
      // 若 Modal 处于开启状态，且历史状态中不再存在当前 Modal 的 stateKey，说明发生了返回回退
      if (visible.value && (!event.state || !event.state[stateKey])) {
        hasPushState = false;
        if (onPopState) {
          const result = await onPopState();
          // 如果外层关闭操作被拦截（如 onWillClose 返回 false），则重新 pushState 恢复拦截状态
          if (result === false) {
            window.history.pushState(
              { ...window.history.state, [stateKey]: true },
              "",
            );
            hasPushState = true;
          }
        } else {
          close();
        }
      }
    });
  });

  // 包装模态框的函数式组件
  const component: FunctionalComponent<
    {
      enterActiveClass?: string;
      enterFromClass?: string;
      leaveActiveClass?: string;
      leaveToClass?: string;
      teleport?: TeleportProps;
    },
    {
      afterLeave(el: Element): void;
      afterEnter(el: Element): void;
    }
  > = function ModalComponent(props, ctx) {
    if (skipRender.value) {
      return h(Comment, "ModelComponent: skip");
    }
    return h(
      Teleport,
      {
        ...props.teleport,
        to:
          (props.teleport?.to === ":provide"
            ? inject(rendererKey)?.()
            : props.teleport?.to) ?? defaultRenderer.value,
      },
      h(
        Transition,
        {
          ...props,
          teleport: undefined,
          appear: true,
          onAfterEnter(el) {
            ctx.emit("afterEnter", el);
          },
          onAfterLeave(el) {
            skipRender.value = true;
            ctx.emit("afterLeave", el);
          },
        },
        () => {
          if (!visible.value) {
            return;
          }
          return h(
            "div",
            {
              ...ctx.attrs,
              class: ctx.attrs.class ?? "fixed inset-0 isolate",
              role: "dialog",
            },
            ctx.slots.default?.(),
          );
        },
      ),
    );
  };

  component.inheritAttrs = false;
  component.props = [
    "enterActiveClass",
    "enterFromClass",
    "leaveActiveClass",
    "leaveToClass",
    "teleport",
  ];
  component.emits = ["afterLeave", "afterEnter"];

  function close() {
    visible.value = false;
    // 如果存在历史记录标记，需要主动调用 history.back() 抹掉它
    if (hasPushState) {
      window.history.back();
      hasPushState = false;
    }
  }

  function open() {
    visible.value = true;
    skipRender.value = false;

    // 当需要拦截返回键时，向 history 栈中推送 dummy state
    if (!hasPushState) {
      window.history.pushState(
        { ...window.history.state, [stateKey]: true },
        "",
      );
      hasPushState = true;
    }
  }

  onUnmounted(() => {
    hasPushState = false;
  });

  return {
    component,
    close,
    open,
    visible,
  };
}
// #endregion
