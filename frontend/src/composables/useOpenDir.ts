import { ref } from "vue";
import openURL from "@/utils/openURL";

// 全局唯一的帮助弹窗显示状态
const showOpenDirHelp = ref(false);

/**
 * 本地路径在资源管理器中展示与定位的 Composable
 */
export function useOpenDir() {
  /**
   * 在资源管理器中展示目录，或定位文件并聚焦
   * @param path 本地绝对物理路径
   */
  const revealInExplorer = async (path: string): Promise<boolean> => {
    if (!path) return false;

    // 调用现有的 openURL 尝试调起注册协议
    const success = await openURL(
      `io.github.natescarlet.open-dir:${encodeURIComponent(path)}`,
      {
        timeout: 1500, // 若 1.5 秒内未发生跳转或失焦则判定失败
      },
    );

    // 失败则弹出安装帮助引导
    if (!success) {
      showOpenDirHelp.value = true;
    }

    return success;
  };

  return {
    revealInExplorer,
    showOpenDirHelp,
  };
}
