import randomUUID from "@/utils/randomUUID";

/**
 * 页面加载时生成的 runId：去掉 UUID 连字符后即 32 位小写十六进制，
 * 与服务端 ValidateRunID 的可信边界规则一致。
 */
export function createLoadLatencyRunId(): string {
  return randomUUID().replace(/-/g, "");
}
