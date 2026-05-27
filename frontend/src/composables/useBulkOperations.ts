import { ref, toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateImageMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";

/**
 * useBulkOperations 提供批量管理图片的操作逻辑与状态
 * @param images 当前展现的图片列表或其 Getter
 * @param onSuccess 操作成功后的回调，通常用于清理选中状态或退出批量模式
 */
export default function useBulkOperations(
  images: MaybeRefOrGetter<ImageFragment[]>,
  onSuccess?: () => void,
) {
  // 记录是否开启批量管理模式
  const isBulkMode = ref(false);
  // 存储当前选中的图片 ID 列表
  const selectedImageIds = ref<string[]>([]);
  // 标记批量操作是否正在提交中
  const isUpdating = ref(false);

  const { show: showNotification } = useNotification();

  // 切换批量模式
  function toggleBulkMode() {
    isBulkMode.value = !isBulkMode.value;
    if (!isBulkMode.value) {
      // 退出批量模式时，自动清空已选中的图片
      selectedImageIds.value = [];
    }
  }

  // 切换单张图片的选中状态
  function toggleSelectImage(id: string) {
    const index = selectedImageIds.value.indexOf(id);
    if (index >= 0) {
      selectedImageIds.value.splice(index, 1);
    } else {
      selectedImageIds.value.push(id);
    }
  }

  // 选中当前列表的所有图片
  function selectAll() {
    selectedImageIds.value = toValue(images).map((img) => img.id);
  }

  // 取消选中所有图片
  function deselectAll() {
    selectedImageIds.value = [];
  }

  /**
   * 批量更新选中图片的星级评分
   * @param rating 星级分数（0-5），0 代表无评分
   */
  async function bulkSetRating(rating: number) {
    const totalSelected = selectedImageIds.value.length;
    if (totalSelected === 0 || isUpdating.value) return;

    isUpdating.value = true;
    try {
      // 并发更新每个选中的图片元数据
      await Promise.all(
        selectedImageIds.value.map((id) =>
          mutate(UpdateImageMetadataDocument, {
            variables: {
              input: {
                id,
                rating,
              },
            },
          }),
        ),
      );

      showNotification(
        `已成功将 ${totalSelected} 张图片的星级设为 ${rating === 0 ? "无评分" : rating + "星"}`,
        "success",
      );
      // 触发成功回调
      onSuccess?.();
    } catch (err) {
      showNotification("批量设置星级失败，请重试", "error");
      console.error("Bulk update rating failed:", err);
    } finally {
      isUpdating.value = false;
    }
  }

  /**
   * 批量更新选中图片的颜色标签
   * @param label 颜色标签名称，为空字符串 "" 表示清除标签
   */
  async function bulkSetLabel(label: string) {
    const totalSelected = selectedImageIds.value.length;
    if (totalSelected === 0 || isUpdating.value) return;

    isUpdating.value = true;
    try {
      // 并发更新每个选中的图片元数据
      await Promise.all(
        selectedImageIds.value.map((id) =>
          mutate(UpdateImageMetadataDocument, {
            variables: {
              input: {
                id,
                label,
              },
            },
          }),
        ),
      );

      showNotification(
        label
          ? `已成功将 ${totalSelected} 张图片的颜色标签设为 ${label}`
          : `已成功清除 ${totalSelected} 张图片的颜色标签`,
        "success",
      );
      // 触发成功回调
      onSuccess?.();
    } catch (err) {
      showNotification("批量设置颜色标签失败，请重试", "error");
      console.error("Bulk update label failed:", err);
    } finally {
      isUpdating.value = false;
    }
  }

  return {
    isBulkMode,
    selectedImageIds,
    isUpdating,
    toggleBulkMode,
    toggleSelectImage,
    selectAll,
    deselectAll,
    bulkSetRating,
    bulkSetLabel,
  };
}
