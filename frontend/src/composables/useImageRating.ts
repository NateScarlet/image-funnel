import { toValue, type MaybeRefOrGetter } from "vue";
import type { ImageFragment } from "@/graphql/generated";
import { UpdateImageMetadataDocument } from "@/graphql/generated";
import mutate from "@/graphql/utils/mutate";
import useHotkey from "./useHotkey";

/**
 * useImageRating 用于管理非会话模式下图片的评分（Rating）
 * @param image 当前查看的图片对象或其 Getter
 * @param sessionId 当前筛选会话 ID 或其 Getter
 */
export default function useImageRating(
  image: MaybeRefOrGetter<ImageFragment>,
  sessionId: MaybeRefOrGetter<string | undefined>,
) {
  async function setRating(rating: number) {
    const sId = toValue(sessionId);
    const img = toValue(image);
    if (sId) {
      // 会话模式下禁用直接修改 Rating 的操作
      return;
    }

    try {
      await mutate(UpdateImageMetadataDocument, {
        variables: {
          input: {
            id: img.id,
            rating: rating === 0 ? null : rating, // 0 星代表清除评分，传入 null
          },
        },
      });
    } catch (err) {
      console.error("Failed to update rating:", err);
    }
  }

  // 注册数字键 0-5 快捷键用于直接修改评分
  for (let r = 0; r <= 5; r++) {
    useHotkey(
      String(r),
      () => {
        const sId = toValue(sessionId);
        if (!sId) {
          setRating(r);
        }
      },
      {
        description: `设置评分为 ${r} 星`,
      },
    );
  }

  return {
    setRating,
  };
}
