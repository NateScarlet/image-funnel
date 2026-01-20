import type { ImageFragment } from "@/graphql/generated";

export function getImageUrlByZoom(
  image: ImageFragment,
  zoomLevel: number,
): string {
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
