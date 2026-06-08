import useStorage from "./useStorage";
import useNotification from "./useNotification";
import useQuery from "@/graphql/utils/useQuery";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import randomUUID from "@/utils/randomUUID";
import { toValue } from "vue";
import type { MaybeRefOrGetter } from "vue";
import {
  MetaDocument,
  ComfyUiWorkflowDocument,
  AttachFileToClipboardDocument,
} from "@/graphql/generated";

// 在模块级持久化保存上次确认不支持增强剪贴板的服务器启动时间
const { model: lastUnsupportedServerStartTime } = useStorage<
  string | undefined
>(localStorage, "last_unsupported_start_time_u2h8a9", () => undefined);

export function useClipboard(options: {
  fullFilePath: MaybeRefOrGetter<string>;
  imageId: MaybeRefOrGetter<string>;
}) {
  const { fullFilePath, imageId } = options;
  const { showSuccess, showError } = useNotification();
  const { data: metaData } = useQuery(MetaDocument);

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

  // 降级文本复制：获取并复制 ComfyUI 工作流（没有则复制路径）
  async function copyWorkflowOrPath(
    filePath: string,
    id: string,
    defaultSuccessMessage = "已复制图片路径!",
  ) {
    let textToCopy = filePath;
    let successMessage = defaultSuccessMessage;

    try {
      const result = await query(ComfyUiWorkflowDocument, {
        variables: { id },
        fetchPolicy: "cache-first",
      });
      if (result.data?.comfyUIWorkflow) {
        textToCopy = result.data.comfyUIWorkflow;
        successMessage = "已复制 ComfyUI 工作流数据!";
      }
    } catch {
      showError("获取工作流数据失败");
    }

    await writeToClipboard(textToCopy);
    showSuccess(successMessage);
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
  async function tryAttachFile(filePath: string): Promise<boolean> {
    if (!filePath) return false;

    const nonce = randomUUID();
    const escapedText = filePath
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
    const html = `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="${nonce}"/></head><body><pre>${escapedText}</pre></body></html>`;

    // 先写入绝对路径和含随机数的 HTML
    const writeOk = await writeToClipboard(filePath, html);
    if (!writeOk) return false;

    try {
      const res = await mutate(AttachFileToClipboardDocument, {
        variables: {
          input: {
            paths: [filePath],
            nonce,
          },
        },
      });
      const supported = res.data?.attachFileToClipboard?.supported ?? false;
      if (supported === false) {
        const serverStartTime = metaData.value?.meta?.serverStartTime;
        if (serverStartTime) {
          lastUnsupportedServerStartTime.value = serverStartTime;
        }
      }
      return supported;
    } catch (err) {
      console.error("增强剪贴板失败", err);
      return false;
    }
  }

  // 处理复制操作 (优先使用增强文件附加，如果成功则不提取工作流)
  async function handleCopy() {
    const filePath = toValue(fullFilePath);
    const id = toValue(imageId);

    // 如果已知不支持，直接获取并复制工作流（如果获取不到则复制路径）
    if (isServerKnownUnsupported()) {
      await copyWorkflowOrPath(filePath, id);
      return;
    }

    // 尝试文件增强复制
    const supported = await tryAttachFile(filePath);
    if (supported) {
      showSuccess("已复制图片文件!");
    } else {
      // 降级：由于增强失败，拉取工作流数据并重新写入剪贴板
      await copyWorkflowOrPath(filePath, id);
    }
  }

  // 总是直接复制路径 (成功附加时提示复制文件，降级时提示已复制绝对路径)
  async function copyAbsoluteFilePath() {
    const filePath = toValue(fullFilePath);
    if (!filePath) return;

    if (isServerKnownUnsupported()) {
      await writeToClipboard(filePath);
      showSuccess("已复制绝对路径!");
      return;
    }

    // 尝试文件增强复制
    const supported = await tryAttachFile(filePath);
    if (supported) {
      showSuccess("已复制图片文件!");
    } else {
      // 降级：仅复制绝对路径
      await writeToClipboard(filePath);
      showSuccess("已复制绝对路径!");
    }
  }

  return {
    handleCopy,
    copyAbsoluteFilePath,
  };
}
