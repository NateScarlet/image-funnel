import { describe, expect, it } from "vitest";
import { createLoadLatencyRunId } from "./runId";

describe("createLoadLatencyRunId", () => {
  it("生成服务端接受的 32 位小写十六进制 runId", () => {
    const runId = createLoadLatencyRunId();

    expect(runId).toMatch(/^[0-9a-f]{32}$/);
    expect(runId).not.toContain("-");
  });

  it("每次调用生成不同的 run", () => {
    expect(createLoadLatencyRunId()).not.toBe(createLoadLatencyRunId());
  });
});
