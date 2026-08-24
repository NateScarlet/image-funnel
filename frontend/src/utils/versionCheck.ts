export interface VersionCheckDeps {
  /** 查询服务端当前版本；undefined 表示查询失败或版本未知 */
  fetchServerVersion: () => Promise<string | undefined>;
  /** 前端构建时注入的版本号（与后端同源的 git describe） */
  builtVersion: string;
  /** 显示失配提示；serverVersion 为 undefined 表示由资源加载失败触发，新版本未知 */
  showStalePrompt: (serverVersion: string | undefined) => void;
  /** 清除已显示的失配提示 */
  clearStalePrompt: () => void;
}

export interface VersionCheck {
  /** WS 连接建立后调用（含首次连接与重连）：比对服务端版本与构建版本，决定显示或清除提示 */
  checkOnConnected: () => Promise<void>;
  /** 懒加载资源加载失败时调用：页面已不可靠，跳过比对直接提示刷新 */
  reportPreloadFailure: () => void;
  /** 用户手动关闭提示后调用：复位提示状态，使下次检查失配时能重新提醒 */
  dismissPrompt: () => void;
}

// 版本已知 = 非空且非 dev；dev 是构建脚本的兜底值，表示无法判定版本
function isKnownVersion(version: string | undefined): version is string {
  return version !== undefined && version !== "" && version !== "dev";
}

export function createVersionCheck(deps: VersionCheckDeps): VersionCheck {
  // 失配提示至多存在一个实例：两个触发点共用同一状态去重
  let promptVisible = false;

  return {
    async checkOnConnected() {
      const serverVersion = await deps.fetchServerVersion();
      if (!isKnownVersion(serverVersion) || !isKnownVersion(deps.builtVersion)) {
        // 无法判定版本（查询失败 / dev 构建）：跳过本次比对，
        // 既不新增提示也不清除已有提示，等待下次连接再查
        return;
      }

      if (serverVersion !== deps.builtVersion) {
        if (!promptVisible) {
          promptVisible = true;
          deps.showStalePrompt(serverVersion);
        }
        return;
      }

      // 版本再次一致（如服务器回滚）：自动清除过时警告
      if (promptVisible) {
        promptVisible = false;
        deps.clearStalePrompt();
      }
    },
    reportPreloadFailure() {
      if (promptVisible) {
        return;
      }
      promptVisible = true;
      deps.showStalePrompt(undefined);
    },
    dismissPrompt() {
      if (!promptVisible) {
        return;
      }
      promptVisible = false;
      deps.clearStalePrompt();
    },
  };
}
