import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment, ImageFiltersInput } from "@/graphql/generated";
import { UpdateImagesMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useNotification from "@/composables/useNotification";

/**
 * useBulkOperations 提供批量管理图片的操作逻辑与状态
 * @param images 当前展现的图片列表或其 Getter
 * @param directoryId 当前目录 ID 或其 Getter
 * @param currentFilterBy 当前的图片过滤条件或其 Getter
 * @param onSuccess 操作成功后的回调，通常用于清理选中状态或退出批量模式
 */
export default function useBulkOperations(
  images: MaybeRefOrGetter<ImageFragment[]>,
  directoryId: MaybeRefOrGetter<string>,
  currentFilterBy: MaybeRefOrGetter<ImageFiltersInput>,
  hasNextPage: MaybeRefOrGetter<boolean>,
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

  // 是否选中了匹配过滤条件的所有图片（采用与 directoryId 对应的缓冲声明式重置）
  const matchingSelectedBuffer = ref({
    directoryId: "",
    enabled: false,
  });

  const isAllMatchingSelected = computed({
    get: () => {
      const dirId = toValue(directoryId);
      if (matchingSelectedBuffer.value.directoryId === dirId) {
        return matchingSelectedBuffer.value.enabled;
      }
      return false;
    },
    set: (val) => {
      matchingSelectedBuffer.value = {
        directoryId: toValue(directoryId),
        enabled: val,
      };
    },
  });

  // 选中图片对应的筛选条件计算属性，高内聚处理全选匹配与局部勾选。未选中任何图片时返回 undefined
  const selectedFilterBy = computed<ImageFiltersInput | undefined>(() => {
    if (isAllMatchingSelected.value) {
      const filter = { ...toValue(currentFilterBy) };
      filter.directoryId = [toValue(directoryId)];
      return filter;
    }
    if (selectedImageIds.value.length === 0) {
      return undefined;
    }
    return {
      id: selectedImageIds.value,
    };
  });

  // 选中图片的已加载实体集合
  const selectedImages = computed(() => {
    const list = toValue(images);
    if (isAllMatchingSelected.value) {
      return list;
    }
    const selectedSet = new Set(selectedImageIds.value);
    return list.filter((img) => selectedSet.has(img.id));
  });

  // 判断指定 ID 图片是否在当前选择范围内
  function isSelected(id: string): boolean {
    if (isAllMatchingSelected.value) {
      return true;
    }
    return selectedImageIds.value.includes(id);
  }

  // 计算底栏选中的图片数量范围与估算文案
  const selectedCountText = computed(() => {
    if (isAllMatchingSelected.value) {
      const currentLoadedCount = toValue(images).length;
      if (toValue(hasNextPage)) {
        return `≥${currentLoadedCount}`;
      }
      return String(currentLoadedCount);
    }
    return String(selectedImageIds.value.length);
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
    if (isAllMatchingSelected.value) {
      isAllMatchingSelected.value = false;
      const allCurrentIds = toValue(images).map((img) => img.id);
      selectedImageIds.value = allCurrentIds.filter(
        (currentId) => currentId !== id,
      );
    } else {
      const current = [...selectedImageIds.value];
      const index = current.indexOf(id);
      if (index >= 0) {
        current.splice(index, 1);
      } else {
        current.push(id);
      }
      selectedImageIds.value = current;
    }
  }

  // 选中当前列表的所有图片（直接进入全选匹配模式）
  function selectAll() {
    isAllMatchingSelected.value = true;
    selectedImageIds.value = toValue(images).map((img) => img.id);
  }

  // 取消选中所有图片
  function deselectAll() {
    isAllMatchingSelected.value = false;
    selectedImageIds.value = [];
  }

  // 反选当前列表的图片
  function invertSelection() {
    if (isAllMatchingSelected.value) {
      deselectAll();
      return;
    }
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
    const filterBy = selectedFilterBy.value;
    if (!filterBy || isUpdating.value) return;

    isUpdating.value = true;
    try {
      const res = await mutate(UpdateImagesMetadataDocument, {
        variables: {
          input: {
            filterBy,
            rating,
          },
        },
      });

      const updatedCount = res.data?.updateImagesMetadata?.updatedCount ?? 0;

      showNotification(
        `已成功将 ${updatedCount} 张图片的星级设为 ${
          rating === 0 ? "无评分" : rating + "星"
        }`,
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
    const filterBy = selectedFilterBy.value;
    if (!filterBy || isUpdating.value) return;

    isUpdating.value = true;
    try {
      const res = await mutate(UpdateImagesMetadataDocument, {
        variables: {
          input: {
            filterBy,
            label,
          },
        },
      });

      const updatedCount = res.data?.updateImagesMetadata?.updatedCount ?? 0;

      showNotification(
        label
          ? `已成功将 ${updatedCount} 张图片的颜色标签设为 ${label}`
          : `已成功清除 ${updatedCount} 张图片的颜色标签`,
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
    selectedFilterBy,
    selectedImages,
    isSelected,
    selectedCountText,
    isUpdating,
    isAllMatchingSelected,
    toggleBulkMode,
    toggleSelectImage,
    selectAll,
    deselectAll,
    invertSelection,
    bulkSetRating,
    bulkSetLabel,
  };
}
