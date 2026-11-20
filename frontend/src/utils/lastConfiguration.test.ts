import { describe, expect, test } from "vitest";
import {
  resolveLastConfiguration,
  type LastConfigurationContext,
  type LastConfigurationSnapshot,
} from "./lastConfiguration";

// #region 测试数据工厂
/** 状态快照中的上次会话：字段与 active 会话全部冲突，用于验证来源优先级 */
const stateSession = {
  filter: { rating: [1], label: ["state-label"], query: "state-query" },
  targetKeep: 5,
};

/** 服务端仍在的 active 上次会话 */
const activeSession = {
  filter: { rating: [2], label: ["active-label"], query: "active-query" },
  targetKeep: 9,
};

const writeActions = { keepRating: 7, shelveRating: 3, rejectRating: 1 };

/** 完成切换场景的回退值（沿用当前会话的条件与目标） */
const completionContext: LastConfigurationContext = {
  kind: "completion",
  filter: { rating: [3], label: ["fallback-label"], query: "fallback-query" },
  targetKeep: 4,
};
// #endregion

describe("resolveLastConfiguration 合并矩阵", () => {
  test.each([
    {
      name: "home-new × 无上次配置",
      snapshot: {},
      context: { kind: "home-new" } as const,
      expected: {
        filter: { rating: undefined, label: undefined, query: undefined },
        targetKeep: 0,
        writeActions: undefined,
      },
    },
    {
      name: "home-new × 仅状态快照",
      snapshot: { lastSessionState: stateSession },
      context: { kind: "home-new" } as const,
      expected: {
        filter: { rating: [1], label: ["state-label"], query: "state-query" },
        targetKeep: 5,
        writeActions: undefined,
      },
    },
    {
      name: "home-new × 仅 active 会话",
      snapshot: { lastSession: activeSession },
      context: { kind: "home-new" } as const,
      expected: {
        filter: { rating: [2], label: ["active-label"], query: "active-query" },
        targetKeep: 9,
        writeActions: undefined,
      },
    },
    {
      name: "home-new × 两者字段冲突时 active 优先",
      snapshot: {
        lastSession: activeSession,
        lastSessionState: stateSession,
        defaultState: { writeActions },
      },
      context: { kind: "home-new" } as const,
      expected: {
        filter: { rating: [2], label: ["active-label"], query: "active-query" },
        targetKeep: 9,
        writeActions,
      },
    },
    {
      name: "completion × 无上次配置（只并 rating 并用当前会话回退）",
      snapshot: {},
      context: completionContext,
      expected: {
        filter: { rating: [3] },
        targetKeep: 4,
        writeActions: undefined,
      },
    },
    {
      name: "completion × 仅状态快照（只并 rating）",
      snapshot: { lastSessionState: stateSession },
      context: completionContext,
      expected: {
        filter: { rating: [1] },
        targetKeep: 5,
        writeActions: undefined,
      },
    },
    {
      name: "completion × 仅 active 会话（只并 rating）",
      snapshot: { lastSession: activeSession },
      context: completionContext,
      expected: {
        filter: { rating: [2] },
        targetKeep: 9,
        writeActions: undefined,
      },
    },
    {
      name: "completion × 两者字段冲突时 active 优先且忽略当前会话的 label/query",
      snapshot: {
        lastSession: activeSession,
        lastSessionState: stateSession,
        defaultState: { writeActions },
      },
      context: completionContext,
      expected: {
        filter: { rating: [2] },
        targetKeep: 9,
        writeActions,
      },
    },
  ])("$name", ({ snapshot, context, expected }) => {
    const result = resolveLastConfiguration(snapshot as LastConfigurationSnapshot, context);
    expect(result).toEqual(expected);
  });
});

describe("resolveLastConfiguration 空值处理", () => {
  test("空数组/空串按 undefined 处理，不向下游透传空切片", () => {
    const snapshot: LastConfigurationSnapshot = {
      lastSession: { filter: { rating: [], label: [], query: "" } },
      defaultState: { writeActions: null },
    };

    const homeNew = resolveLastConfiguration(snapshot, { kind: "home-new" });
    expect(homeNew.filter).toEqual({ rating: undefined, label: undefined, query: undefined });
    expect(homeNew.targetKeep).toBe(0);
    expect(homeNew.writeActions).toBeUndefined();

    const completion = resolveLastConfiguration(snapshot, completionContext);
    expect(completion.filter).toEqual({ rating: [3] });
    expect(completion.targetKeep).toBe(4);
  });
});
