import type { Plugin } from "@opencode-ai/plugin";

export const BlockGoRun: Plugin = async () => {
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      if (!/\bgo\s+run\b/.test(output.args.command)) return;
      throw new Error(
        "禁止使用 `go run`。请使用 `go test` 运行测试，使用 `go tool` 或 scripts/ 下的对应脚本运行工具。"
      );
    },
  };
};
