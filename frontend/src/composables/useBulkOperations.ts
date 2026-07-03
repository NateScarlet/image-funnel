import { ref, computed, toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment, ImageFiltersInput } from "@/graphql/generated";
import useNotification from "@/composables/useNotification";
import { useBulkImage } from "./domain/useImage";

export default function useBulkOperations(
  images: MaybeRefOrGetter<ImageFragment[]>,
  directoryId: MaybeRefOrGetter<string>,
  currentFilterBy: MaybeRefOrGetter<ImageFiltersInput>,
  hasNextPage: MaybeRefOrGetter<boolean>,
  onSuccess?: () => void,
) {
  const { updateMetadata } = useBulkImage();

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

  const selectedFilterBy = computed<ImageFiltersInput | undefined>(() => {
    if (isAllMatchingSelected.value) {
      const filter = { ...toValue(currentFilterBy) };
      filter.directoryId = [toValue(directoryId)];
      return filter;
    }
    if (selectedImageIds.value.length === 0) {
      return undefined;
    }
    return { id: selectedImageIds.value };
  });

  const selectedImages = computed(() => {
    const list = toValue(images);
    if (isAllMatchingSelected.value) return list;
    const selectedSet = new Set(selectedImageIds.value);
    return list.filter((img) => selectedSet.has(img.id));
  });

  function isSelected(id: string): boolean {
    if (isAllMatchingSelected.value) return true;
    return selectedImageIds.value.includes(id);
  }

  const selectedCountText = computed(() => {
    if (isAllMatchingSelected.value) {
      const currentLoadedCount = toValue(images).length;
      if (toValue(hasNextPage)) return `≥${currentLoadedCount}`;
      return String(currentLoadedCount);
    }
    return String(selectedImageIds.value.length);
  });

  const isUpdating = ref(false);
  const { show: showNotification } = useNotification();

  function toggleBulkMode() {
    isBulkMode.value = !isBulkMode.value;
  }

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

  function selectAll() {
    isAllMatchingSelected.value = true;
    selectedImageIds.value = toValue(images).map((img) => img.id);
  }

  function deselectAll() {
    isAllMatchingSelected.value = false;
    selectedImageIds.value = [];
  }

  function invertSelection() {
    if (isAllMatchingSelected.value) {
      deselectAll();
      return;
    }
    const currentListIds = toValue(images).map((img) => img.id);
    const selected = new Set(selectedImageIds.value);
    const nextSelected: string[] = [];
    for (const id of currentListIds) {
      if (!selected.has(id)) nextSelected.push(id);
    }
    selectedImageIds.value = nextSelected;
  }

  async function bulkSetRating(rating: number) {
    const filterBy = selectedFilterBy.value;
    if (!filterBy || isUpdating.value) return;

    isUpdating.value = true;
    try {
      const res = await updateMetadata({ filterBy, rating });
      const updatedCount = res.data?.updateImagesMetadata?.updatedCount ?? 0;

      showNotification(
        `已成功将 ${updatedCount} 张图片的星级设为 ${
          rating === 0 ? "无评分" : rating + "星"
        }`,
        "success",
      );
      onSuccess?.();
    } catch (err) {
      showNotification("批量设置星级失败，请重试", "error");
      console.error("Bulk update rating failed:", err);
    } finally {
      isUpdating.value = false;
    }
  }

  async function bulkSetLabel(label: string) {
    const filterBy = selectedFilterBy.value;
    if (!filterBy || isUpdating.value) return;

    isUpdating.value = true;
    try {
      const res = await updateMetadata({ filterBy, label });
      const updatedCount = res.data?.updateImagesMetadata?.updatedCount ?? 0;

      showNotification(
        label
          ? `已成功将 ${updatedCount} 张图片的颜色标签设为 ${label}`
          : `已成功清除 ${updatedCount} 张图片的颜色标签`,
        "success",
      );
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
