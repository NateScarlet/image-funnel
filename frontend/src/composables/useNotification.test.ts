import { describe, test, expect, vi, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import useNotification from "./useNotification";
import NotificationList from "@/components/NotificationList.vue";

describe("useNotification", () => {
  beforeEach(() => {
    const { clear } = useNotification();
    clear();
  });

  // #region Composable 单元测试
  describe("show 及辅助函数", () => {
    test("show 函数能够包含 message 和可选的 body 字段", () => {
      const { notifications, show } = useNotification();

      show("标题", "info", 3000, "正文内容");

      expect(notifications.value).toHaveLength(1);
      expect(notifications.value[0]).toEqual(
        expect.objectContaining({
          message: "标题",
          type: "info",
          body: "正文内容",
        }),
      );
    });

    test("showError / showSuccess / showInfo / showWarning 支持传入 body", () => {
      const { notifications, showError, showSuccess, showInfo, showWarning } = useNotification();

      showError("错误标题", 5000, "错误正文");
      showSuccess("成功标题", 3000, "成功正文");
      showInfo("提示标题", 3000, "提示正文");
      showWarning("警告标题", 3000, "警告正文");

      expect(notifications.value).toHaveLength(4);
      expect(notifications.value[0].body).toBe("错误正文");
      expect(notifications.value[1].body).toBe("成功正文");
      expect(notifications.value[2].body).toBe("提示正文");
      expect(notifications.value[3].body).toBe("警告正文");
    });

    test("remove 和 clear 能够正确移除通知", () => {
      const { notifications, show, remove, clear } = useNotification();

      const id1 = show("通知1");
      show("通知2");

      expect(notifications.value).toHaveLength(2);

      remove(id1);
      expect(notifications.value).toHaveLength(1);
      expect(notifications.value[0].message).toBe("通知2");

      clear();
      expect(notifications.value).toHaveLength(0);
    });
  });
  // #endregion

  // #region NotificationList.vue 渲染测试
  describe("NotificationList.vue 渲染断言", () => {
    test("收到带 body 的通知时，正确渲染标题与正文", () => {
      const { show } = useNotification();
      show("测试标题", "info", 0, "测试正文内容");

      const wrapper = mount(NotificationList);

      expect(wrapper.text()).toContain("测试标题");
      expect(wrapper.text()).toContain("测试正文内容");
    });

    test("收到不带 body 的通知时，仅渲染标题，不渲染正文", () => {
      const { show } = useNotification();
      show("仅标题通知", "info", 0);

      const wrapper = mount(NotificationList);

      expect(wrapper.text()).toContain("仅标题通知");
      expect(wrapper.find(".whitespace-pre-wrap").exists()).toBe(false);
    });

    test("点击右上角关闭按钮时触发 controller.dismiss 并移除通知", async () => {
      const { show } = useNotification();
      const dismiss = vi.fn();
      show("可关闭通知", "info", 0, undefined, undefined, { dismiss });

      const wrapper = mount(NotificationList);
      await wrapper.find("button").trigger("click");

      expect(dismiss).toHaveBeenCalled();
      expect(wrapper.text()).not.toContain("可关闭通知");
    });

    test("不带 controller 的通知点击关闭按钮仅移除，不调用 dismiss", async () => {
      const { show } = useNotification();
      show("普通通知", "info", 0);

      const wrapper = mount(NotificationList);
      await wrapper.find("button").trigger("click");

      expect(wrapper.text()).not.toContain("普通通知");
    });

    test("带 openDetails 控制器的通知渲染查看详情按钮且点击触发", async () => {
      const { show } = useNotification();
      const openDetails = vi.fn();
      show("带详情通知", "info", 0, undefined, undefined, { openDetails });

      const wrapper = mount(NotificationList);

      const btn = wrapper.findAll("button").find((b) => b.text() === "查看详情");
      expect(btn).toBeTruthy();
      await btn?.trigger("click");

      expect(openDetails).toHaveBeenCalled();
      expect(wrapper.text()).not.toContain("带详情通知");
    });
  });
  // #endregion
});
