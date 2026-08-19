import type { ImageFragment } from "@/graphql/generated";

// #region 图像 URL 分辨率匹配
/**
 * 根据缩放级别获取最佳分辨率的图片 URL
 */
export function getImageUrlByZoom(image: ImageFragment, zoomLevel: number): string {
  const targetWidth = Math.ceil(image.width * zoomLevel);

  if (targetWidth <= 256) {
    return image.url256;
  }
  if (targetWidth <= 512) {
    return image.url512;
  }
  if (targetWidth <= 1024) {
    return image.url1024;
  }
  if (targetWidth <= 2048) {
    return image.url2048;
  }
  if (targetWidth <= 4096) {
    return image.url4096;
  }
  return image.url;
}

/**
 * 获取在 object-fit: cover 模式渲染下所需的最佳分辨率图片 URL。
 * 根据容器宽度 W_box 与高度 H_box 以及图片原始宽高比，计算渲染所需覆盖的最少像素宽度：
 * W_req = max(W_box, H_box * (image.width / image.height))
 * 并向上匹配合适的分辨率（url256 -> url512 -> url1024 -> url2048 -> url4096 -> url）。
 */
export function getCoverImageUrl(
  image: ImageFragment,
  containerWidth = 256,
  containerHeight = 256,
): string {
  if (!image.width || !image.height) {
    return image.url256 || image.url;
  }

  // 计算 cover 模式覆盖容器所需渲染的最小宽度
  const requiredWidth = Math.ceil(
    Math.max(containerWidth, containerHeight * (image.width / image.height)),
  );

  if (requiredWidth <= 256) {
    return image.url256 || image.url;
  }
  if (requiredWidth <= 512) {
    return image.url512 || image.url;
  }
  if (requiredWidth <= 1024) {
    return image.url1024 || image.url;
  }
  if (requiredWidth <= 2048) {
    return image.url2048 || image.url;
  }
  if (requiredWidth <= 4096) {
    return image.url4096 || image.url;
  }
  return image.url;
}
// #endregion
