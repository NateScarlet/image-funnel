import useStorage from "./useStorage";
import useNotification from "./useNotification";
import useQuery from "@/graphql/utils/useQuery";
import query from "@/graphql/utils/query";
import mutate from "@/graphql/utils/mutate";
import randomUUID from "@/utils/randomUUID";
import {
  MetaDocument,
  ComfyUiWorkflowDocument,
  AttachFileToClipboardDocument,
} from "@/graphql/generated";

// 在模块级持久化保存上次确认不支持增强剪贴板的服务器启动时间
const { model: lastUnsupportedServerStartTime } = useStorage<
  string | undefined
>(localStorage, "last_unsupported_start_time_u2h8a9", () => undefined);

export function useClipboard() {
  const { showSuccess } = useNotification();
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

  // 处理单张图片的复制操作（优先尝试只复制 ComfyUI 工作流，如果不存在或失败再复制文件）
  async function copyWorkflowOrFile(filePath: string, imageId: string) {
    if (!filePath || !imageId) return;

    let workflow: string | null | undefined = undefined;
    try {
      // 优先获取图片的 ComfyUI 工作流数据
      const result = await query(ComfyUiWorkflowDocument, {
        variables: { id: imageId },
        fetchPolicy: "cache-first",
      });
      workflow = result.data?.comfyUIWorkflow;
    } catch (err) {
      console.error("获取工作流数据失败", err);
    }

    if (workflow) {
      const ok = await writeToClipboard(workflow);
      if (ok) {
        showSuccess("已复制 ComfyUI 工作流数据!");
        return;
      }
    }

    // 无法获取或复制工作流时，降级为复制图片文件
    if (isServerKnownUnsupported()) {
      await writeToClipboard(filePath);
      showSuccess("已复制图片路径!");
      return;
    }

    // 尝试增强剪贴板文件复制
    const supported = await tryAttachFiles([filePath]);
    if (supported) {
      showSuccess("已复制图片文件!");
    } else {
      // 增强复制失败时，降级复制为绝对路径文本
      await writeToClipboard(filePath);
      showSuccess("已复制图片路径!");
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

    const nonce = randomUUID();
    const textToCopy = filePaths.join("\r\n");
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

  // 复制多个文件：如果支持复制文件就复制文件，否则复制绝对路径每行一个
  async function copyFiles(...filePaths: string[]) {
    const validPaths = filePaths.filter((p) => !!p);
    if (validPaths.length === 0) return;

    if (isServerKnownUnsupported()) {
      await writeToClipboard(validPaths.join("\r\n"));
      showSuccess("已复制绝对路径!");
      return;
    }

    // 尝试文件增强复制
    const supported = await tryAttachFiles(validPaths);
    if (supported) {
      showSuccess("已复制图片文件!");
    } else {
      // 降级：仅复制绝对路径
      await writeToClipboard(validPaths.join("\r\n"));
      showSuccess("已复制绝对路径!");
    }
  }

  return {
    copyWorkflowOrFile,
    copyFiles,
  };
}
