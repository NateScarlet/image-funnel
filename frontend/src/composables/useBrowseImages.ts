import { computed, toValue, ref, type MaybeRefOrGetter, type Ref } from "vue";
import { useImageBrowse } from "./domain/useImage";
import type {
  BrowseImagesQueryVariables,
  ImageFragment,
} from "@/graphql/generated";

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

    images.value.forEach((img) => {
      const isMatched = matchesFilters(img);
      const isExcluded = excludedImageIds.value.has(img.id);

      const shouldHide = isExcluded && !isMatched;

      if (!shouldHide) {
        visibleImages.push(img);
        if (!isMatched) {
          mismatchedIds.add(img.id);
        }
      }

      if (!isMatched) {
        nextExcludedIds.add(img.id);
      }
    });

    return { visibleImages, mismatchedIds, nextExcludedIds };
  });

  function applyLocalFilter() {
    excludedImageIds.value = filterStates.value.nextExcludedIds;
  }

  return {
    images: computed(() => filterStates.value.visibleImages),
    outOfFilterImageIds: computed(() => filterStates.value.mismatchedIds),
    applyLocalFilter,
    hasNextPage,
    fetchMore,
  };
}
