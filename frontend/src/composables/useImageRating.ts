import { toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateImageMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";

/**
 * useImageRating 用于管理图片的评分（Rating）
 * @param image 当前查看的图片对象或其 Getter
 */
export default function useImageRating(image: MaybeRefOrGetter<ImageFragment>) {
  async function setRating(rating: number) {
    const img = toValue(image);
    try {
      await mutate(UpdateImageMetadataDocument, {
        variables: {
          input: {
            id: img.id,
            rating,
          },
        },
      });
    } catch (err) {
      console.error("Failed to update rating:", err);
    }
  }

  return {
    setRating,
  };
}
