import type { NavigationOutcome } from "@/composables/useAutoNavigateAfterCommit";

/** 导航后某次采样时看到的视图状态 */
export interface ViewSample {
  route: string;
  /** 完成视图是否仍挂载——路由变了但完成视图没消失，仍算没切换 */
  completed: boolean;
}

/**
 * 由导航前后的视图采样判定结果。
 * 独立成纯函数使"停在完成视图/闪烁"的判据可被单测覆盖，不依赖组件挂载与计时器。
 */
export function classifyNavigationOutcome(
  beforeRoute: string,
  first: ViewSample,
  second: ViewSample,
): NavigationOutcome {
  if (second.completed) {
    // 完成视图重新出现，或一直没消失
    return first.completed ? "unchanged" : "flicker";
  }
  if (first.route === beforeRoute && second.route === beforeRoute) {
    return "unchanged";
  }
  return "switched";
}
