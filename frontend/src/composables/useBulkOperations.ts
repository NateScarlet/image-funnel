import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateImageMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";

/**
 * useBulkOperations 提供批量管理图片的操作逻辑与状态
 * @param images 当前展现的图片列表或其 Getter
 * @param directoryId 当前目录 ID 或其 Getter
 * @param onSuccess 操作成功后的回调，通常用于清理选中状态或退出批量模式
 */
export default function useBulkOperations(
  images: MaybeRefOrGetter<ImageFragment[]>,
  directoryId: MaybeRefOrGetter<string>,
  onSuccess?: () => void,
) {
  // 记录是否开启批量管理模式，并使用目录 ID 进行缓冲隔离以实现声明式重置
  const bulkModeBuffer = ref({ directoryId: "", enabled: false });
  const isBulkMode = computed({
    get: () => {
      const dirId = toValue(directoryId);
      return bulkModeBuffer.value.directoryId === dirId
        ? bulkModeBuffer.value.enabled
        : false;
    },
    set: (val) => {
      bulkModeBuffer.value = {
        directoryId: toValue(directoryId),
        enabled: val,
      };
    },
  });

  // 存储当前选中的图片 ID 列表，并使用目录 ID 进行缓冲隔离以实现声明式重置
  const selectedBuffer = ref({ directoryId: "", ids: [] as string[] });
  const selectedImageIds = computed<string[]>({
    get: () => {
      const dirId = toValue(directoryId);
      return selectedBuffer.value.directoryId === dirId
        ? selectedBuffer.value.ids
        : [];
    },
    set: (val) => {
      selectedBuffer.value = {
        directoryId: toValue(directoryId),
        ids: val,
      };
    },
  });

  // 标记批量操作是否正在提交中
  const isUpdating = ref(false);

  const { show: showNotification } = useNotification();

  // 切换批量模式
  function toggleBulkMode() {
    isBulkMode.value = !isBulkMode.value;
  }

  // 切换单张图片的选中状态
  function toggleSelectImage(id: string) {
    const current = [...selectedImageIds.value];
    const index = current.indexOf(id);
    if (index >= 0) {
      current.splice(index, 1);
    } else {
      current.push(id);
    }
    selectedImageIds.value = current;
  }

  // 选中当前列表的所有图片
  function selectAll() {
    selectedImageIds.value = toValue(images).map((img) => img.id);
  }

  // 取消选中所有图片
  function deselectAll() {
    selectedImageIds.value = [];
  }

  // 反选当前列表的图片
  function invertSelection() {
    const currentListIds = toValue(images).map((img) => img.id);
    const selected = new Set(selectedImageIds.value);
    const nextSelected: string[] = [];
    for (const id of currentListIds) {
      if (!selected.has(id)) {
        nextSelected.push(id);
      }
    }
    selectedImageIds.value = nextSelected;
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
    invertSelection,
    bulkSetRating,
    bulkSetLabel,
  };
}
