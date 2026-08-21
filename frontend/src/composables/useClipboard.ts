import { type Ref } from "vue";
import useStorage from "./useStorage";
import useNotification from "./useNotification";
import useQuery from "@/graphql/utils/useQuery";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import randomUUID from "@/utils/randomUUID";
import {
  MetaDocument,
  ImageCopyContentDocument,
  AttachFileToClipboardDocument,
} from "@/graphql/generated";

// 在模块级持久化保存上次确认不支持增强剪贴板的服务器启动时间
const { model: lastUnsupportedServerStartTime } = useStorage<string | undefined>(
  localStorage,
  "last_unsupported_start_time_u2h8a9",
  () => undefined,
);

// 模块级保存当前会话曾复制过的图片 ID 列表，以实现跨组件、跨查看周期的共享状态
const { model: copiedImageIds, flush: flushCopiedImageIds } = useStorage<string[]>(
  sessionStorage,
  "copied_image_ids_s7f8g9",
  () => [],
);

// 将文本（和可选的 HTML）写入剪贴板
async function writeToClipboard(text: string, html?: string) {
  try {
    if (html) {
      const data = new ClipboardItem({
        "text/plain": new Blob([text], { type: "text/plain" }),
        "text/html": new Blob([html], { type: "text/html" }),
      });
      await window.navigator.clipboard.write([data]);
    } else {
      await window.navigator.clipboard.writeText(text);
    }
    return true;
  } catch {
    try {
      await window.navigator.clipboard.writeText(text);
      return true;
    } catch {
      return false;
    }
  }
}

export function useClipboard(options?: { loadingCount?: Ref<number> }) {
  const { showSuccess } = useNotification();
  const { data: metaData } = useQuery(MetaDocument);

  function addCopiedImageId(id: string) {
    if (!id) return;
    if (!copiedImageIds.value.includes(id)) {
      copiedImageIds.value.push(id);
      flushCopiedImageIds();
    }
  }

  // 处理单张图片的复制操作（优先尝试复制增强内容，未配置钩子或不适用时降级复制文件）
  async function copyEnhancedOrFile(filePath: string, imageId: string) {
    if (!filePath || !imageId) return;

    // 防止并发重复操作
    if (options?.loadingCount) {
      if (options.loadingCount.value > 0) return;
      options.loadingCount.value++;
    }

    try {
      let enhancedContent: string | null | undefined = undefined;
      let enhancedDescription: string | null | undefined = undefined;
      try {
        // 优先获取复制增强钩子提供的内容（cache-first：命中缓存不重复 spawn 脚本）
        const result = await query(ImageCopyContentDocument, {
          variables: { id: imageId },
          fetchPolicy: "cache-first",
        });
        enhancedContent = result.data?.imageCopyContent?.content ?? null;
        enhancedDescription = result.data?.imageCopyContent?.description;
      } catch {
        // 获取失败则降级复制图片文件/路径；
        // 错误通知由全局 ErrorLink（graphql/client.ts）负责弹出，此处不重复提示
      }

      if (enhancedContent) {
        const ok = await writeToClipboard(enhancedContent);
        if (ok) {
          showSuccess(enhancedDescription ?? "已复制增强内容!");
          addCopiedImageId(imageId);
          return;
        }
      }

      // 无法获取或复制增强内容时，降级为复制图片文件/路径
      const supported = await tryAttachFiles([filePath]);
      if (supported) {
        showSuccess("已复制图片文件!");
      } else {
        showSuccess("已复制图片路径!");
      }
      addCopiedImageId(imageId);
    } finally {
      if (options?.loadingCount) {
        options.loadingCount.value--;
      }
    }
  }

  // 判断当前连接的服务器是否已知不支持剪贴板增强
  function isServerKnownUnsupported(): boolean {
    const serverStartTime = metaData.value?.meta?.serverStartTime;
    if (!serverStartTime || !lastUnsupportedServerStartTime.value) {
      return false;
    }
    try {
      return (
        new Date(serverStartTime).getTime() <=
        new Date(lastUnsupportedServerStartTime.value).getTime()
      );
    } catch {
      return serverStartTime === lastUnsupportedServerStartTime.value;
    }
  }

  // 尝试向服务器申请文件增强，返回是否附加成功
  async function tryAttachFiles(filePaths: string[]): Promise<boolean> {
    if (filePaths.length === 0) return false;

    const textToCopy = filePaths.join("\r\n");

    // 如果已知服务器不支持，直接写入绝对路径文本并返回不支持
    if (isServerKnownUnsupported()) {
      await writeToClipboard(textToCopy);
      return false;
    }

    const nonce = randomUUID();
    const escapedText = textToCopy
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
    const html = `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="${nonce}"/></head><body><pre>${escapedText}</pre></body></html>`;

    // 先写入绝对路径文本和含随机数的 HTML
    const writeOk = await writeToClipboard(textToCopy, html);
    if (!writeOk) return false;

    try {
      const res = await mutate(AttachFileToClipboardDocument, {
        variables: {
          input: {
            paths: filePaths,
            nonce,
          },
        },
      });
      const supported = res.data?.attachFileToClipboard?.supported ?? false;
      if (!supported) {
        const serverStartTime = metaData.value?.meta?.serverStartTime;
        if (serverStartTime) {
          lastUnsupportedServerStartTime.value = serverStartTime;
        }
      }
      return supported;
    } catch {
      // 错误由全局 ErrorLink 显示
      return false;
    }
  }

  // 复制多个文件：如果支持复制文件就复制文件，否则复制绝对路径每行一个
  async function copyFiles(...filePaths: string[]) {
    const validPaths = filePaths.filter((p) => !!p);
    if (validPaths.length === 0) return;

    // 防止并发重复操作
    if (options?.loadingCount) {
      if (options.loadingCount.value > 0) return;
      options.loadingCount.value++;
    }

    try {
      // 尝试文件增强复制
      const supported = await tryAttachFiles(validPaths);
      if (supported) {
        showSuccess("已复制图片文件!");
      } else {
        showSuccess("已复制绝对路径!");
      }
    } finally {
      if (options?.loadingCount) {
        options.loadingCount.value--;
      }
    }
  }

  return {
    copyEnhancedOrFile,
    copyFiles,
    copiedImageIds,
  };
}
