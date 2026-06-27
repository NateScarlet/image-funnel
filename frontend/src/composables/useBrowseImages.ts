import {
  computed,
  toValue,
  ref,
  type MaybeRefOrGetter,
  type Ref,
  nextTick,
} from "vue";
import { keyBy } from "es-toolkit";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import {
  BrowseImagesDocument,
  ImageSavedDocument,
  DirEntryDeletedDocument,
  ImageFragmentDoc,
  type BrowseImagesQueryVariables,
  type ImageFragment,
} from "@/graphql/generated";
import client from "@/graphql/client";

/**
 * useBrowseImages 提供目录图片的查询、实时订阅和分页加载功能
 * @param variables 查询变量，支持响应式对象或其 Getter
 * @param options 可选配置，支持传入全局的 loadingCount
 */
export default function useBrowseImages(
  variables: MaybeRefOrGetter<BrowseImagesQueryVariables>,
  options?: { loadingCount?: Ref<number> },
) {
  // 转换为计算属性，保证响应式更新
  const resolvedVariables = computed(() => toValue(variables));

  // 提取目录 ID，供实时订阅模块过滤使用
  const directoryId = computed(() => resolvedVariables.value.id);

  // 执行 GraphQL 查询获取图片
  const { data: imagesData, query: imagesQuery } = useQuery(
    BrowseImagesDocument,
    {
      variables: () => resolvedVariables.value,
      loadingCount: options?.loadingCount,
    },
  );

  // 利用 useRelayConnection 管理分页拼接和 fetchMore
  const imageConnection = useRelayConnection(
    () =>
      imagesData.value?.node?.__typename === "Directory"
        ? imagesData.value.node.images
        : undefined,
    () => imagesQuery,
  );

  // 通过 useLiveConnection 接入实时增量更新逻辑
  const {
    nodes: images,
    onSaved,
    onDeleted,
  } = useLiveConnection(() => imageConnection.nodes.value, {
    identity: (i) => i.relPath,
    compare: (a, b) => {
      const aTime = new Date(a.modTime).getTime();
      const bTime = new Date(b.modTime).getTime();
      if (aTime !== bTime) {
        return bTime - aTime;
      }
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

  // 订阅图片的新增与修改事件
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

  // 构建 relPath 到图片的索引，删除文件时 O(1) 查找
  const imageByRelPath = computed(() => keyBy(images.value, (i) => i.relPath));

  // 订阅文件/目录的删除事件
  useSubscription(DirEntryDeletedDocument, {
    variables: () => ({ directoryId: directoryId.value }),
    onNext: (result) => {
      nextTick(() => {
        const deletedEntry = result.data?.dirEntryDeleted;
        if (deletedEntry) {
          const match = imageByRelPath.value[deletedEntry.relPath];
          if (match) {
            onDeleted({ id: match.id });
          }
        }
      });
    },
  });

  // #region 本地筛选状态与声明式过滤逻辑
  // 记录已被手动筛选排除的图片 ID 集合
  const excludedImageIds = ref<Set<string>>(new Set());

  // 本地过滤判断函数
  const matchesFilters = (img: ImageFragment) => {
    const filterBy = resolvedVariables.value.filterBy;
    if (!filterBy) return true;

    // 1. 评分过滤
    if (filterBy.rating != null) {
      if (!filterBy.rating.includes(img.currentRating)) {
        return false;
      }
    }
    // 2. 标签过滤
    if (filterBy.label != null) {
      if (!img.label || !filterBy.label.includes(img.label)) {
        return false;
      }
    }
    // 3. 搜索过滤
    if (filterBy.query) {
      const q = filterBy.query.toLowerCase();
      if (!img.filename.toLowerCase().includes(q)) {
        return false;
      }
    }
    return true;
  };

  // 在一次原始数组迭代中同时计算：可见图片列表、已显示但不合规 ID、以及待排除不合规 ID
  const filterStates = computed(() => {
    const visibleImages: ImageFragment[] = [];
    const mismatchedIds = new Set<string>();
    const nextExcludedIds = new Set<string>();

    images.value.forEach((img) => {
      const isMatched = matchesFilters(img);
      const isExcluded = excludedImageIds.value.has(img.id);

      // 被隐藏的条件：已被排除，且目前依然不满足筛选
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

    return {
      visibleImages,
      mismatchedIds,
      nextExcludedIds,
    };
  });

  // 应用本地筛选，直接将最新计算的待排除 ID 集合赋值给排除列表
  function applyLocalFilter() {
    excludedImageIds.value = filterStates.value.nextExcludedIds;
  }
  // #endregion

  return {
    images: computed(() => filterStates.value.visibleImages),
    outOfFilterImageIds: computed(() => filterStates.value.mismatchedIds),
    applyLocalFilter,
    hasNextPage: computed(() => imageConnection.pageInfo.value.hasNextPage),
    fetchMore: imageConnection.fetchMore,
  };
}
