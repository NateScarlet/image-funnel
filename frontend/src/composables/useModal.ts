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

  // 恢复到"从未渲染过"的初始状态，保证遮罩不会脱离关闭流程残留在页面上
  function reset() {
    skipRender.value = true;
    visible.value = false;
  }

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
        // 渲染状态复位在退场动画的 afterLeave 上，但调用端按文档把数据就绪判定的 v-if 绑在包装组件上，
        // 数据可能在退场动画播完前就消失（切换目录、重新拉取等），此时 afterLeave 不会再触发。
        // 卸载时在此复位，避免残留的"可见"状态在下一次挂载时渲染出一个吞掉整页点击的空遮罩。
        onVnodeUnmounted: reset,
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
            return undefined;
          }
          return h(
            "div",
            {
              ...ctx.attrs,
              class: ctx.attrs.class ?? "fixed inset-0 isolate z-50",
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
