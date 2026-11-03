/**
 * 目录取页等待被取消（查询变量切换或作用域销毁）。
 *
 * 这不是分页请求失败：本次「合并结果可见」的等待目标已经失效。
 * 面向列表的 UI 消费方应静默结束；只有需要严格语义的流程（如完成视图的自动切换）
 * 才继续向上暴露，并由调用方识别后静默中止。
 */
export class DirectoryPageWaitCancelledError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "DirectoryPageWaitCancelledError";
  }
}

export function isDirectoryPageWaitCancelled(
  error: unknown,
): error is DirectoryPageWaitCancelledError {
  return error instanceof DirectoryPageWaitCancelledError;
}
