import { computed } from "vue";
import useQuery from "@/graphql/utils/useQuery";
import { MetaDocument } from "@/graphql/generated";
import basename from "@/utils/basename";

// #region 推导内核

/**
 * 根相对路径 → 绝对物理路径。
 *
 * 边界语义：
 * - 根绝对路径缺失（元数据未加载或为空）→ `""`，无从推导起算点
 * - `relPath` 为 `undefined` → `""`，语义是「未知」，与确知为根目录的零值区分开
 * - `relPath` 为 `""` → 根绝对路径本身，即根目录
 *
 * 分隔符判定沿用既有启发式：根绝对路径含 `\` 即为 Windows 风格。
 */
export function resolveAbsPath(
  rootAbsPath: string | undefined,
  relPath: string | undefined,
): string {
  if (!rootAbsPath || relPath === undefined) {
    return "";
  }
  if (relPath === "") {
    return rootAbsPath;
  }
  if (rootAbsPath.includes("\\")) {
    return rootAbsPath + "\\" + relPath.replace(/\//g, "\\");
  }
  return rootAbsPath + "/" + relPath.replace(/\\/g, "/");
}

/**
 * 根相对路径 → 展示名。边界语义与 {@link resolveAbsPath} 一致，
 * 区别仅在于非根目录取 basename 而非拼接完整路径。
 */
export function resolveDisplayName(
  rootAbsPath: string | undefined,
  relPath: string | undefined,
): string {
  if (!rootAbsPath || relPath === undefined) {
    return "";
  }
  if (relPath === "") {
    return rootAbsPath;
  }
  return basename(relPath);
}
// #endregion

/**
 * 文件系统上下文：持有服务端根路径，并作为「根相对路径 → 绝对路径 / 展示名」
 * 的唯一推导入口。占位内容（如 `…`）由调用方自行决定，本模块只负责推导。
 */
export function useFileSystemContext() {
  const { data } = useQuery(MetaDocument);
  const rootPath = computed(() => data.value?.meta.rootAbsPath || "");

  return {
    /** 服务端根目录的绝对物理路径；元数据未加载时为空串 */
    rootPath,
    absPath: (relPath: string | undefined): string => resolveAbsPath(rootPath.value, relPath),
    displayName: (relPath: string | undefined): string =>
      resolveDisplayName(rootPath.value, relPath),
  };
}
