import { describe, test, expect } from "vitest";
import { evaluateDirectoryVisibility } from "./directoryVisibility";

// #region 测试夹具
// 目标保留 0 张评级 1 的图片，实际有 3 张，故目录未达标；其中 3 张未评级图片用于未评级上限维度
const uncompletedDir = {
  lastSession: {
    filter: { rating: [1] },
    targetKeep: 0,
  },
};

const ratingCounts = [
  { rating: 0, count: 3 },
  { rating: 1, count: 3 },
];

const leafStats = { subdirectoryCount: 0, ratingCounts };
const parentStats = { subdirectoryCount: 2, ratingCounts };
// #endregion

// #region 目录可见性判定测试
describe("evaluateDirectoryVisibility", () => {
  // 叶子 / 含子目录 × 两个开关开闭 × 上限命中/未命中/未设置
  test.each([
    {
      name: "未达标叶子目录，两个开关关闭且未评级超限",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: true },
    },
    {
      name: "未达标含子目录目录，两个开关关闭且未评级超限",
      hasSubdirectory: true,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "未达标叶子目录，仅关闭显示未评级较多开关且未评级超限",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: true,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: true },
    },
    {
      name: "未达标含子目录目录，仅关闭显示未评级较多开关且未评级超限",
      hasSubdirectory: true,
      showUncompleted: false,
      showLargeUnrated: true,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "未达标叶子目录，仅关闭显示未达标开关且未评级超限",
      hasSubdirectory: false,
      showUncompleted: true,
      showLargeUnrated: false,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "未达标含子目录目录，仅关闭显示未达标开关且未评级超限",
      hasSubdirectory: true,
      showUncompleted: true,
      showLargeUnrated: false,
      unratedLimit: 1,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "未达标叶子目录，两个开关均开启",
      hasSubdirectory: false,
      showUncompleted: true,
      showLargeUnrated: true,
      unratedLimit: 1,
      expected: { isFilteredOut: false, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "未达标含子目录目录，两个开关均开启",
      hasSubdirectory: true,
      showUncompleted: true,
      showLargeUnrated: true,
      unratedLimit: 1,
      expected: { isFilteredOut: false, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "未达标叶子目录，两个开关关闭但未评级数量未超限",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 3,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: true },
    },
    {
      name: "未达标含子目录目录，两个开关关闭但未评级数量未超限",
      hasSubdirectory: true,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 3,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "未达标叶子目录，两个开关关闭且未设置未评级上限",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: undefined,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: true },
    },
    {
      name: "未达标含子目录目录，两个开关关闭且未设置未评级上限",
      hasSubdirectory: true,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: undefined,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "达标叶子目录，两个开关关闭",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "达标含子目录目录，两个开关关闭",
      hasSubdirectory: true,
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "达标目录，两个开关均开启",
      hasSubdirectory: false,
      showUncompleted: true,
      showLargeUnrated: true,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: false, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "达标叶子目录，开启显示未达标开关但未评级超限",
      hasSubdirectory: false,
      showUncompleted: true,
      showLargeUnrated: false,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: true, isForcedShown: false, isHiddenByUncompleted: false },
    },
    {
      name: "达标含子目录目录，开启显示未达标开关但未评级超限",
      hasSubdirectory: true,
      showUncompleted: true,
      showLargeUnrated: false,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: true, isForcedShown: true, isHiddenByUncompleted: false },
    },
    {
      name: "达标叶子目录，仅关闭显示未评级较多开关但未评级超限",
      hasSubdirectory: false,
      showUncompleted: false,
      showLargeUnrated: true,
      unratedLimit: 1,
      completed: true,
      expected: { isFilteredOut: false, isForcedShown: false, isHiddenByUncompleted: false },
    },
  ])(
    "$name",
    ({ hasSubdirectory, showUncompleted, showLargeUnrated, unratedLimit, completed, expected }) => {
      // 达标目录放宽 targetKeep 使 keepCount(3) <= targetKeep(10)
      const dir = completed
        ? { lastSession: { filter: { rating: [1] }, targetKeep: 10 } }
        : uncompletedDir;

      const result = evaluateDirectoryVisibility(dir, hasSubdirectory ? parentStats : leafStats, {
        showUncompleted,
        showLargeUnrated,
        unratedLimit,
      });

      expect(result).toEqual(expected);
    },
  );

  test("当 stats 缺失时，未评分数按 0 处理，且因无子目录信息不强制显示", () => {
    const result = evaluateDirectoryVisibility(uncompletedDir, undefined, {
      showUncompleted: false,
      showLargeUnrated: false,
      unratedLimit: 1,
    });

    // 无统计数据时目录视为未达标；未评级 0 未超上限，故只由达标维度被过滤
    expect(result).toEqual({
      isFilteredOut: true,
      isForcedShown: false,
      isHiddenByUncompleted: true,
    });
  });

  test("当 stats 缺失且未评级上限为 0 时，仍不因未评级维度被过滤", () => {
    const result = evaluateDirectoryVisibility(uncompletedDir, undefined, {
      showUncompleted: true,
      showLargeUnrated: false,
      unratedLimit: 0,
    });

    expect(result).toEqual({
      isFilteredOut: false,
      isForcedShown: false,
      isHiddenByUncompleted: false,
    });
  });
});
// #endregion
