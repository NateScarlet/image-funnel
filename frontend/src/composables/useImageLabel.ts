import { ref, computed, toValue, watch, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateLabelDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";

// 预设的图片标签颜色映射
export const PRESET_COLORS: Record<string, string> = {
  Red: "#ef4444",
  Yellow: "#eab308",
  Green: "#22c55e",
  Blue: "#3b82f6",
  Purple: "#a855f7",
  Orange: "#f97316",
  Grey: "#6b7280",
  Black: "#1e293b",
  White: "#f8fafc",
};

/**
 * useImageLabel 用于管理图片 XMP 标签的状态及修改逻辑
 * @param image 当前查看的图片对象或其 Getter
 * @param sessionId 当前筛选会话 ID 或其 Getter
 */
export default function useImageLabel(
  image: MaybeRefOrGetter<ImageFragment>,
  sessionId: MaybeRefOrGetter<string | undefined>,
) {
  // 控制玻璃磨砂下拉气泡菜单的显示隐藏
  const showPopover = ref(false);
  // 绑定自定义文本输入框的值
  const customLabelInput = ref("");

  // 获取当前图片的标签内容，无标签时默认为空字符串
  const currentLabel = computed(() => toValue(image).label || "");

  // 判断当前标签是否属于 9 种预设颜色之一
  const isPresetColor = computed(() => {
    return Object.keys(PRESET_COLORS).includes(currentLabel.value);
  });

  // 获取当前标签对应的颜色代码
  const currentLabelColor = computed(() => {
    return PRESET_COLORS[currentLabel.value];
  });

  // 监听图片 ID 改变，当切换图片时自动收起菜单并清空自定义输入框
  watch(
    () => toValue(image).id,
    () => {
      customLabelInput.value = "";
      showPopover.value = false;
    },
  );

  /**
   * 提交设置图片标签的 mutation 更改
   * @param label 目标标签字符串，传入空值代表清除标签
   */
  async function setLabel(label: string) {
    const sId = toValue(sessionId);
    const img = toValue(image);
    if (!sId) {
      return;
    }
    showPopover.value = false;
    try {
      await mutate(UpdateLabelDocument, {
        variables: {
          sessionId: sId,
          imageId: img.id,
          label: label,
        },
      });
    } catch (err) {
      console.error("Failed to update label:", err);
    }
  }

  /**
   * 保存输入框中的自定义标签
   */
  function saveCustomLabel() {
    const val = customLabelInput.value.trim();
    setLabel(val);
  }

  return {
    showPopover,
    customLabelInput,
    currentLabel,
    isPresetColor,
    currentLabelColor,
    setLabel,
    saveCustomLabel,
  };
}
