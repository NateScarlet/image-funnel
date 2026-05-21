import { toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateImageMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useHotkey from "./useHotkey";

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

  // 注册数字键 0-5 快捷键用于直接修改评分
  for (let r = 0; r <= 5; r++) {
    useHotkey(String(r), () => setRating(r), {
      description: `设置评分为 ${r} 星`,
    });
  }

  return {
    setRating,
  };
}
