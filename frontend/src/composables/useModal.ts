import type {
  FunctionalComponent,
  InjectionKey,
  MaybeRefOrGetter,
  RendererElement,
  TeleportProps,
} from "vue";
import { Comment, Teleport, Transition, h, inject, provide, ref, toValue } from "vue";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";

// #region 渲染目标依赖注入配置
const rendererKey: InjectionKey<() => string | RendererElement> = Symbol("modalRenderer");

/**
 * 提供模态框渲染挂载的自定义容器
 * @param renderer 目标容器或其获取器
 */
export function provideModalRenderer(renderer: MaybeRefOrGetter<string | RendererElement>) {
  const parent = inject(rendererKey);
  provide(rendererKey, () => toValue(renderer) ?? parent?.());
}
// #endregion

// #region 核心 useModal Composable 实现
/**
 * 基础模态框挂载与动画状态控制 Composable
 */
export default function useModal() {
  const defaultRenderer = useFullscreenRendererElement();
  const skipRender = ref(true);
  const visible = ref(false);

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
          (props.teleport?.to === ":provide" ? inject(rendererKey)?.() : props.teleport?.to) ??
          defaultRenderer.value,
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

  function hide() {
    visible.value = false;
  }

  function show() {
    // 打开模态框时清除选中的文本，避免误操作导致 Ctrl+C 复制被拦截
    window.getSelection()?.removeAllRanges();
    visible.value = true;
    skipRender.value = false;
  }

  return {
    component,
    hide,
    show,
    visible,
  };
}
// #endregion
