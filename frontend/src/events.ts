import defineCustomEvent from "@/utils/defineCustomEvent";

export const openImageViewerByFilename = defineCustomEvent<{
  filename: string;
}>();
