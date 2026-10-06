// #region 导入与测试宿主
import { describe, test, expect, afterEach, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import useModalDialog from "./useModalDialog";
import sleep from "@/utils/sleep";

// 遮罩是 useModal 渲染的全屏点击拦截层，出现即代表弹窗"仍被认为处于打开状态"。
const OVERLAY_SELECTOR = 'div.fixed.inset-0.isolate.z-50[role="dialog"]';
// ModalDialog 退场动画时长，超过它才能确认动画真的播完
const LEAVE_DURATION_MS = 200;

function overlayCount(): number {
  return document.querySelectorAll(OVERLAY_SELECTOR).length;
}

// 宿主复刻调用端的实际用法：按文档把数据就绪判定的 v-if 绑在控制器包装组件上。
const Host = defineComponent({
  setup() {
    const ready = ref(true);
    const dialog = useModalDialog();
    return { ready, dialog };
  },
  render() {
    return this.ready ? h(this.dialog.component, null, { default: () => h("div", "内容") }) : null;
  },
});

type DialogController = {
  open(): Promise<void>;
  close(): Promise<boolean>;
};

const mounted: VueWrapper[] = [];

function mountHost() {
  // 关闭 transition stub：退场动画与 afterLeave 的时序正是本用例要验证的行为
  const wrapper = mount(Host, {
    attachTo: document.body,
    global: { stubs: { transition: false } },
  });
  mounted.push(wrapper);
  return { wrapper, dialog: wrapper.vm.dialog as unknown as DialogController };
}

afterEach(() => {
  while (mounted.length > 0) {
    mounted.pop()?.unmount();
  }
  document.body.innerHTML = "";
});
// #endregion

// #region 组件卸载后不得残留遮罩
describe("useModal 遮罩渲染状态", () => {
  test("包装组件在退场动画播放期间被卸载后重新挂载，不得残留遮罩", async () => {
    const { wrapper, dialog } = mountHost();

    await dialog.open();
    expect(overlayCount()).toBe(1);

    // 关闭后立刻让数据消失：调用端的数据判定 v-if 会把包装组件连同退场动画一起卸载，
    // 此时动画的 afterLeave 不会再触发，useModal 的渲染状态必须由卸载本身复位。
    await dialog.close();
    wrapper.vm.ready = false;
    await flushPromises();
    expect(overlayCount()).toBe(0);

    // 数据重新就绪，包装组件再次挂载
    wrapper.vm.ready = true;
    await flushPromises();
    await sleep(LEAVE_DURATION_MS * 2);
    expect(overlayCount()).toBe(0);

    // 复用未受污染的控制器：仍能正常打开
    await dialog.open();
    expect(overlayCount()).toBe(1);
  });

  test("包装组件完整走完开关流程后不得残留遮罩", async () => {
    const { dialog } = mountHost();

    await dialog.open();
    expect(overlayCount()).toBe(1);

    await dialog.close();
    await vi.waitFor(() => expect(overlayCount()).toBe(0));

    await dialog.open();
    expect(overlayCount()).toBe(1);
  });
});
// #endregion
