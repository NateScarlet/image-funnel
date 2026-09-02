import { ImageFormat } from "@/graphql/generated";

// #region 浏览器图片格式能力探测

/** WebP 测试图片（1x1 透明，通过 magick convert -size 1x1 xc:transparent tiny.webp 创建） */
const WEBP_TEST =
  "data:image/webp;base64,UklGRkAAAABXRUJQVlA4WAoAAAAQAAAAAAAAAAAAQUxQSAIAAAAAAFZQOCAYAAAAMAEAnQEqAQABAAIANCWkAANwAP77/VAA";

/** AVIF 测试图片（1x1 透明，通过 magick convert -size 1x1 xc:transparent tiny.avif 创建） */
const AVIF_TEST =
  "data:image/avif;base64,AAAAHGZ0eXBhdmlmAAAAAGF2aWZtaWYxbWlhZgAAAeptZXRhAAAAAAAAACFoZGxyAAAAAAAAAABwaWN0AAAAAAAAAAAAAAAAAAAAAA5waXRtAAAAAAABAAAANGlsb2MAAAAAREAAAgACAAAAAAIOAAEAAAAAAAAAFgABAAAAAAIkAAEAAAAAAAAAGgAAADhpaW5mAAAAAAACAAAAFWluZmUCAAAAAAEAAGF2MDEAAAAAFWluZmUCAAAAAAIAAGF2MDEAAAABKWlwcnAAAAEBaXBjbwAAABNjb2xybmNseAACAAIABoAAAAAMYXYxQ4FAfAAAAAAUaXNwZQAAAAAAAAABAAAAAQAAAChjbGFwAAAAAQAAAAEAAAABAAAAAf////EAAAAC////8QAAAAIAAAAOcGl4aQAAAAABDAAAADhhdXhDAAAAAHVybjptcGVnOm1wZWdCOmNpY3A6c3lzdGVtczphdXhpbGlhcnk6YWxwaGEAAAAADGF2MUOBQGwAAAAAFGlzcGUAAAAAAAAAAQAAAAEAAAAoY2xhcAAAAAEAAAABAAAAAQAAAAH////xAAAAAv////EAAAACAAAAEHBpeGkAAAAAAwwMDAAAACBpcG1hAAAAAAAAAAIAAQWBhwiJigACBYIDhIWGAAAAGmlyZWYAAAAAAAAADmF1eGwAAgABAAEAAAA4bWRhdBIACgVYAAa6gDILGAAAAUAAAiEdy2ASAAoIWAAGtAgIG4QyDBgAAABQAAAAsBNL2A==";

const testImages = new Map<string, string>([
  ["image/webp", WEBP_TEST],
  ["image/avif", AVIF_TEST],
]);

/** 测试浏览器是否能解码给定 data URL 的图片格式 */
async function testImage(dataURL: string): Promise<boolean> {
  try {
    // 优先使用 createImageBitmap（现代浏览器）
    if (typeof createImageBitmap === "function") {
      const r = await fetch(dataURL);
      const blob = await r.blob();
      const m = await createImageBitmap(blob);
      m.close();
      return true;
    }

    // 兜底：使用 Image 对象检测
    return await new Promise<boolean>((resolve) => {
      const img = new Image();
      const timer = setTimeout(() => resolve(false), 60_000);
      img.onload = () => {
        clearTimeout(timer);
        resolve(true);
      };
      img.onerror = () => {
        clearTimeout(timer);
        resolve(false);
      };
      img.src = dataURL;
    });
  } catch {
    return false;
  }
}

/**
 * 检查当前浏览器是否支持给定的图片格式，可能需要一次性的异步检测。
 * 结果会被缓存，后续调用直接返回已缓存的值。
 */
export default function isImageFormatSupported(
  mimeType: string,
): Readonly<{ value?: boolean } & Promise<boolean>> {
  let ret = cache.get(mimeType);
  if (ret == null) {
    const v = check(mimeType);
    if (typeof v === "boolean") {
      ret = Object.assign(Promise.resolve(v), { value: v });
    } else {
      ret = v;
      void v.then((ok) => {
        cache.set(
          mimeType,
          Object.assign(Promise.resolve(ok), { value: ok }),
        );
      });
    }
    cache.set(mimeType, ret);
  }
  return ret;
}

const universallySupportedFormats = new Set([
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/bmp",
  "image/x-icon",
  "image/svg+xml",
]);

function check(mimeType: string): boolean | Promise<boolean> {
  if (universallySupportedFormats.has(mimeType)) {
    return true;
  }
  const dataURL = testImages.get(mimeType);
  if (dataURL) {
    return testImage(dataURL);
  }
  // ImageDecoder 仅在部分浏览器中可用，isTypeSupported 返回 Promise
  const decoderCtor = globalThis.ImageDecoder as
    | { isTypeSupported?: (type: string) => Promise<boolean> }
    | undefined;
  if (decoderCtor?.isTypeSupported) {
    return decoderCtor.isTypeSupported(mimeType);
  }
  return false;
}

const cache = new Map<string, ReturnType<typeof isImageFormatSupported>>();

// 内置预热：检测 WebP 支持
isImageFormatSupported("image/webp");

// #endregion

// #region 格式查询变量

/**
 * 根据 AVIF 支持能力返回 GraphQL 查询使用的格式变量值。
 * 支持 AVIF 返回 ImageFormat.AVIF，否则返回 ImageFormat.WEBP。
 */
export function getPreferredFormat(): ImageFormat {
  const avifSupport = isImageFormatSupported("image/avif");
  return avifSupport.value === true ? ImageFormat.AVIF : ImageFormat.WEBP;
}

// #endregion
