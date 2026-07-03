// #region 单图操作
import { computed, ref, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useRelayConnection from "@/composables/useRelayConnection";
import useLiveConnection from "@/composables/useLiveConnection";
import mutate from "@/graphql/utils/mutate";
import client from "@/graphql/client";
import {
  UpdateImageMetadataDocument,
  HooksDocument,
  DispatchImageHookDocument,
  UpdateImagesMetadataDocument,
  BrowseImagesDocument,
  ImageSavedDocument,
  DirEntryDeletedDocument,
  ImageFragmentDoc,
  type ImageFragment,
  type ImageFiltersInput,
  type BrowseImagesQueryVariables,
} from "@/graphql/generated";
import { throttle } from "es-toolkit";

async function dispatch(hookId: string, filterBy: ImageFiltersInput) {
  await mutate(DispatchImageHookDocument, {
    variables: { input: { hookId, filterBy } },
  });
}

export default function useImage(image?: MaybeRefOrGetter<ImageFragment>) {
  const hooksLoadingCountRef = ref(0);
  const { data: hooksData } = useQuery(HooksDocument, {
    loadingCount: hooksLoadingCountRef,
  });

  const dispatchableHooks = computed(() => {
    return hooksData.value?.hooks.filter((h) => h.canDispatchByImage) || [];
  });

  async function setRating(rating: number) {
    if (!image) return;
    const img = toValue(image);
    await mutate(UpdateImageMetadataDocument, {
      variables: { input: { id: img.id, rating } },
    });
  }

  async function setLabel(label: string) {
    if (!image) return;
    const img = toValue(image);
    await mutate(UpdateImageMetadataDocument, {
      variables: { input: { id: img.id, label } },
    });
  }

  return {
    setRating,
    setLabel,
    dispatchableHooks,
    dispatch,
    hooksLoadingCountRef,
  };
}
// #endregion

// #region 图片浏览
export function useImageBrowse(
  variables: MaybeRefOrGetter<BrowseImagesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  const resolvedVariables = computed(() => toValue(variables));
  const directoryId = computed(() => resolvedVariables.value.id);

  const { data: imagesData, query: imagesQuery } = useQuery(BrowseImagesDocument, {
    variables: () => resolvedVariables.value,
    loadingCount: options?.loadingCount,
  });

  const imageConnection = useRelayConnection(
    () =>
      imagesData.value?.node?.__typename === "Directory" ? imagesData.value.node.images : undefined,
    () => imagesQuery,
  );

  const {
    nodes: images,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => imageConnection.nodes.value, {
    identity: (i: ImageFragment) => i.relPath,
    compare: (a: ImageFragment, b: ImageFragment) => {
      const aTime = new Date(a.modTime).getTime();
      const bTime = new Date(b.modTime).getTime();
      if (aTime !== bTime) return bTime - aTime;
      return b.id.localeCompare(a.id);
    },
    subscribe: (item, callback) => {
      const observable = client.watchFragment<ImageFragment>({
        fragment: ImageFragmentDoc,
        fragmentName: "Image",
        from: item,
      });
      const sub = observable.subscribe((result) => {
        if (result.complete && result.data) {
          callback(result.data);
        }
      });
      return () => sub.unsubscribe();
    },
  });

  useSubscription(ImageSavedDocument, {
    variables: () => ({ filterBy: { directoryId: [directoryId.value] } }),
    onNext: (result) => {
      const savedImage = result.data?.imageSaved;
      if (savedImage) {
        client.writeFragment({
          id: client.cache.identify(savedImage),
          fragment: ImageFragmentDoc,
          fragmentName: "Image",
          data: savedImage,
        });
        onSaved(savedImage);
      }
    },
  });

  const pendingRelPathDeletion = new Set<string>();
  function doFlushRelPathDeletion() {
    if (pendingRelPathDeletion.size === 0) return;
    for (const img of images.value) {
      if (pendingRelPathDeletion.has(img.relPath)) {
        onDeleted(img);
      }
    }
    pendingRelPathDeletion.clear();
  }
  const flushRelPathDeletion = throttle(doFlushRelPathDeletion, 1e3, {
    edges: ["leading", "trailing"],
  });

  useSubscription(DirEntryDeletedDocument, {
    variables: () => ({ directoryId: directoryId.value }),
    onNext: (result) => {
      const deletedEntries = result.data?.dirEntryDeleted;
      if (deletedEntries && deletedEntries.length > 0) {
        for (const entry of deletedEntries) {
          pendingRelPathDeletion.add(entry.relPath);
        }
        flushRelPathDeletion();
      }
    },
  });

  return {
    images,
    query: imagesQuery,
    data: imagesData,
    hasNextPage: computed(() => imageConnection.pageInfo.value.hasNextPage),
    fetchMore: imageConnection.fetchMore,
  };
}
// #endregion

// #region 批量操作
async function updateMetadata(options: {
  filterBy: ImageFiltersInput;
  rating?: number;
  label?: string;
}) {
  return mutate(UpdateImagesMetadataDocument, {
    variables: { input: options },
  });
}

export function useBulkImage() {
  return { updateMetadata };
}
// #endregion
