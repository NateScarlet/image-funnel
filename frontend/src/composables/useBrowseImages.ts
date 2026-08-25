import { computed, toValue, ref, type MaybeRefOrGetter, type Ref } from "vue";
import { useImageBrowse } from "./domain/useImage";
import type { BrowseImagesQueryVariables, ImageFragment } from "@/graphql/generated";

export default function useBrowseImages(
  variables: MaybeRefOrGetter<BrowseImagesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  const resolvedVariables = computed(() => toValue(variables));

  const { images, hasNextPage, fetchMore } = useImageBrowse(variables, options);

  const excludedImageIds = ref<Set<string>>(new Set());

  const matchesFilters = (img: ImageFragment) => {
    const filterBy = resolvedVariables.value.filterBy;
    if (!filterBy) return true;

    if (filterBy.rating != null) {
      if (!filterBy.rating.includes(img.currentRating)) {
        return false;
      }
    }
    if (filterBy.label != null) {
      if (!img.label || !filterBy.label.includes(img.label)) {
        return false;
      }
    }
    if (filterBy.query) {
      const q = filterBy.query.toLowerCase();
      if (!img.filename.toLowerCase().includes(q)) {
        return false;
      }
    }
    return true;
  };

  const filterStates = computed(() => {
    const visibleImages: ImageFragment[] = [];
    const mismatchedIds = new Set<string>();
    const nextExcludedIds = new Set<string>();
    let hiddenCount = 0;

    images.value.forEach((img) => {
      const isMatched = matchesFilters(img);
      const isExcluded = excludedImageIds.value.has(img.id);

      // 已被本地筛选排除且当前仍不匹配的图片不再显示
      const shouldHide = isExcluded && !isMatched;

      if (!shouldHide) {
        visibleImages.push(img);
        if (!isMatched) {
          mismatchedIds.add(img.id);
        }
      } else {
        hiddenCount += 1;
      }

      if (!isMatched) {
        nextExcludedIds.add(img.id);
      }
    });

    return { visibleImages, mismatchedIds, nextExcludedIds, hiddenCount };
  });

  function applyLocalFilter() {
    excludedImageIds.value = filterStates.value.nextExcludedIds;
  }

  // 清空本地排除集，恢复显示所有已加载图片
  function clearLocalFilter() {
    excludedImageIds.value = new Set();
  }

  return {
    images: computed(() => filterStates.value.visibleImages),
    outOfFilterImageIds: computed(() => filterStates.value.mismatchedIds),
    hiddenImageCount: computed(() => filterStates.value.hiddenCount),
    applyLocalFilter,
    clearLocalFilter,
    hasNextPage,
    fetchMore,
  };
}
