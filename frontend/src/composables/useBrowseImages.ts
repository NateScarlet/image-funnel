import { computed, toValue, type MaybeRefOrGetter, type Ref } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import useSubscription from "@/graphql/utils/useSubscription";
import useRelayConnection from "./useRelayConnection";
import useLiveConnection from "./useLiveConnection";
import {
  BrowseImagesDocument,
  ImageSavedDocument,
  ImageDeletedDocument,
  type ImageFragment,
  type BrowseImagesQueryVariables,
} from "@/graphql/generated";

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
  } = useLiveConnection(() => imageConnection.nodes.value);

  // 订阅图片的新增与修改事件
  useSubscription(ImageSavedDocument, {
    variables: () => {
      const filterBy = directoryId.value
        ? { directoryId: [directoryId.value] }
        : undefined; // 避免返回 null，使用 undefined 代替
      return { filterBy };
    },
    onNext: (result) => {
      const savedImage = result.data?.imageSaved;
      if (savedImage) {
        onSaved(savedImage);
      }
    },
  });

  // 订阅图片的删除事件
  useSubscription(ImageDeletedDocument, {
    variables: () => {
      const filterBy = directoryId.value
        ? { directoryId: [directoryId.value] }
        : undefined; // 避免返回 null，使用 undefined 代替
      return { filterBy };
    },
    onNext: (result) => {
      const deletedImage = result.data?.imageDeleted;
      if (deletedImage) {
        onDeleted({ id: deletedImage.id } as ImageFragment);
      }
    },
  });

  return {
    images,
    hasNextPage: computed(() => imageConnection.pageInfo.value.hasNextPage),
    fetchMore: imageConnection.fetchMore,
  };
}
