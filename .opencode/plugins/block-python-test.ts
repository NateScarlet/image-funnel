import type { Plugin } from "@opencode-ai/plugin";

export const BlockPythonTest: Plugin = async () => {
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") return;
      const command: string = output.args.command ?? "";
      if (
        /\bpytest\b/.test(command) ||
        /\bpython\s+-m\s+unittest\b/.test(command)
      ) {
        throw new Error(
          "[项目规则]禁止直接执行 pytest 或 python -m unittest。请使用 `pwsh scripts/check-python.ps1` 运行 Python 类型检查、测试和格式化。"
        );
      }
    },
  };
};
