import { ref, computed, watchEffect, onMounted, nextTick, type Ref, type ComputedRef } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { useHotkeys } from "@/composables/useHotkeys";
import useModalFullscreen from "@/composables/useModalFullscreen";
import useLocationHash from "@/composables/useLocationHash";
import { openImageViewerByFilename } from "@/events";

interface UseImageViewerInput {
  images: ComputedRef<ImageFragment[]>;
  hasNextPage: ComputedRef<boolean | undefined>;
  loading: ComputedRef<boolean>;
  fetchMore: () => Promise<void>;
  clearFilters: () => void;
  searchQuery: Ref<string>;
}

export default function useImageViewer(input: UseImageViewerInput) {
  const { images, hasNextPage, loading, fetchMore, clearFilters, searchQuery } = input;

  const imageViewerDialog = useModalFullscreen();

  // #region 查看器状态
  const currentImageId = ref<string | undefined>(undefined);
  const currentImage = computed(() => {
    if (currentImageId.value === undefined) return undefined;
    return images.value.find((img) => img.id === currentImageId.value);
  });

  const currentImageIndex = computed(() => {
    if (currentImageId.value === undefined) return -1;
    return images.value.findIndex((img) => img.id === currentImageId.value);
  });

  // 构造交替预载图片列表（后1张、前1张、后2张、前2张……）以支持反向预载
  const preloadImages = computed(() => {
    const index = currentImageIndex.value;
    if (index === -1) return [];

    const list: ImageFragment[] = [];
    const len = images.value.length;
    const maxOffset = Math.max(index, len - 1 - index);

    for (let offset = 1; offset <= maxOffset; offset++) {
      const nextIdx = index + offset;
      const prevIdx = index - offset;

      if (nextIdx < len) {
        list.push(images.value[nextIdx]);
      }
      if (prevIdx >= 0) {
        list.push(images.value[prevIdx]);
      }
    }
    return list;
  });
  // #endregion

  // #region URL Hash 状态持久化
  const viewerHash = useLocationHash();
  // #endregion

  // #region 导航
  function prevImage() {
    const index = currentImageIndex.value;
    if (index > 0) {
      const img = images.value[index - 1];
      if (img) {
        currentImageId.value = img.id;
        viewerHash.value = img.filename;
      }
    }
  }

  // 预载下一页：若目标图片是当前列表的最后一张，且有后续页面未加载，则在后台静默发起加载请求
  function checkAndFetchMore(index: number) {
    if (index !== -1 && index === images.value.length - 1 && hasNextPage.value && !loading.value) {
      fetchMore();
    }
  }

  async function nextImage() {
    const index = currentImageIndex.value;
    if (index === -1) return;

    if (index < images.value.length - 1) {
      const nextIdx = index + 1;
      const img = images.value[nextIdx];
      if (img) {
        currentImageId.value = img.id;
        viewerHash.value = img.filename;
        checkAndFetchMore(nextIdx);
      }
    } else if (hasNextPage.value && !loading.value) {
      const prevLength = images.value.length;
      await fetchMore();
      await nextTick();
      if (images.value.length > prevLength) {
        const img = images.value[prevLength];
        if (img) {
          currentImageId.value = img.id;
          viewerHash.value = img.filename;
          checkAndFetchMore(prevLength);
        }
      }
    }
  }
  // #endregion

  // #region 打开/关闭
  function openViewer(image: ImageFragment) {
    currentImageId.value = image.id;
    viewerHash.value = image.filename;
    imageViewerDialog.open();

    const index = images.value.findIndex((img) => img.id === image.id);
    checkAndFetchMore(index);
  }

  function closeViewer() {
    imageViewerDialog.close();
  }

  function handleViewerAfterLeave() {
    currentImageId.value = undefined;
    viewerHash.value = "";
  }
  // #endregion

  // #region 通过文件名搜索并打开查看器
  function tryOpenViewerByFilename(filename: string): boolean {
    console.log("try open", filename);
    const image = images.value.find((img: ImageFragment) => img.filename === filename);
    if (image) {
      openViewer(image);
      return true;
    }
    return false;
  }

  async function waitLoading() {
    await nextTick();
    using stack = new DisposableStack();
    await new Promise<void>((resolve) => {
      stack.defer(
        watchEffect(() => {
          if (!loading.value) {
            resolve();
          }
        }),
      );
    });
  }

  async function searchAndOpenViewer(filename: string) {
    if (tryOpenViewerByFilename(filename)) {
      return;
    }
    clearFilters();
    searchQuery.value = filename;
    await waitLoading();
    tryOpenViewerByFilename(filename);
  }
  // #endregion

  // #region 查看器热键
  useHotkeys(
    {
      arrowleft: () => {
        prevImage();
      },
      arrowright: () => {
        nextImage();
      },
      home: () => {
        if (images.value.length > 0) {
          const img = images.value[0];
          currentImageId.value = img.id;
          viewerHash.value = img.filename;
        }
      },
      end: async () => {
        let pageCount = 0;
        while (hasNextPage.value) {
          if (pageCount > 0 && pageCount % 10 === 0) {
            const shouldContinue = confirm(`已经自动加载了 ${pageCount} 页图片，是否继续加载？`);
            if (!shouldContinue) {
              break;
            }
          }
          const prevLength = images.value.length;
          await fetchMore();
          await nextTick();
          if (images.value.length <= prevLength) {
            break;
          }
          pageCount++;
        }
        if (images.value.length > 0) {
          const img = images.value[images.value.length - 1];
          currentImageId.value = img.id;
          viewerHash.value = img.filename;
        }
      },
    },
    {
      allowInInputs: true,
      scope: imageViewerDialog.scopeId,
      category: "图片浏览",
    },
  );
  // #endregion

  // #region 初始化与事件监听
  onMounted(async () => {
    if (viewerHash.value) {
      await waitLoading();
      searchAndOpenViewer(viewerHash.value);
    }
  });

  // 响应 NoteList 打开图片查看器的事件
  watchEffect((onCleanup) => {
    const unsubscribe = openImageViewerByFilename.subscribe((event) => {
      searchAndOpenViewer(event.detail.filename);
    });
    onCleanup(unsubscribe);
  });
  // #endregion

  return {
    imageViewerDialog,
    currentImageId,
    currentImage,
    currentImageIndex,
    preloadImages,
    openViewer,
    closeViewer,
    handleViewerAfterLeave,
    viewerHash,
    prevImage,
    nextImage,
  };
}
