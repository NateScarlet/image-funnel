import { describe, test, expect } from "vitest";
import { evaluateDirectoryCompletion } from "./directoryCompletion";

// #region 目录达标判定测试
describe("evaluateDirectoryCompletion", () => {
  test("当目录无默认设置 (lastSession 和 state.lastSession 皆为空) 时，应判定为不达标 (isCompleted = false)", () => {
    const dir = {
      lastSession: undefined,
      state: undefined,
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [
        { rating: 0, count: 5 },
        { rating: 1, count: 0 },
      ],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(false);
    expect(result.keepCount).toBe(0);
    expect(result.lastSession).toBeUndefined();
  });

  test("当 stats 为 undefined 或 null（无数据）时，即使有默认设置也应判定为不达标 (isCompleted = false)", () => {
    const dir = {
      lastSession: {
        filter: { rating: [1] },
        targetKeep: 5,
      },
    };

    const resultUndefined = evaluateDirectoryCompletion(dir, undefined);
    expect(resultUndefined.isCompleted).toBe(false);

    const resultNull = evaluateDirectoryCompletion(dir, null);
    expect(resultNull.isCompleted).toBe(false);
  });

  test("当目录有默认设置且 keepCount <= targetKeep 时，应判定为已达标 (isCompleted = true)", () => {
    const dir = {
      lastSession: {
        filter: { rating: [1] },
        targetKeep: 2,
      },
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [
        { rating: 0, count: 10 },
        { rating: 1, count: 1 },
      ],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(true);
    expect(result.keepCount).toBe(1);
    expect(result.filterRating).toEqual([1]);
    expect(result.targetKeep).toBe(2);
  });

  test("当目录有默认设置但 keepCount > targetKeep 时，应判定为不达标 (isCompleted = false)", () => {
    const dir = {
      lastSession: {
        filter: { rating: [1, 2] },
        targetKeep: 1,
      },
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [
        { rating: 1, count: 1 },
        { rating: 2, count: 2 },
      ],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(false);
    expect(result.keepCount).toBe(3);
  });

  test("当目录有子目录 (subdirectoryCount > 0) 且 keepCount <= targetKeep 时，应判定为达标 (isCompleted = true)", () => {
    const dir = {
      lastSession: {
        filter: { rating: [1] },
        targetKeep: 5,
      },
    };
    const stats = {
      subdirectoryCount: 2,
      ratingCounts: [{ rating: 1, count: 0 }],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(true);
  });

  test("当 dir.lastSession 为空但 dir.state.lastSession 存在时，应正确取 state.lastSession", () => {
    const dir = {
      lastSession: null,
      state: {
        lastSession: {
          filter: { rating: [3] },
          targetKeep: 0,
        },
      },
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [{ rating: 3, count: 0 }],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(true);
    expect(result.filterRating).toEqual([3]);
  });

  test("当 rating 为 null 或 undefined 时代表不过滤评级（匹配所有文件），若总数 > targetKeep 应判定为未达标", () => {
    const dir = {
      lastSession: {
        filter: { rating: null },
        targetKeep: 4,
      },
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [
        { rating: 0, count: 500 },
        { rating: 1, count: 2 },
      ],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(false);
    expect(result.keepCount).toBe(502);
  });

  test("当 rating 为 [] 时代表匹配 0 文件，keepCount 为 0，若 0 <= targetKeep 则判定为已达标", () => {
    const dir = {
      lastSession: {
        filter: { rating: [] },
        targetKeep: 4,
      },
    };
    const stats = {
      subdirectoryCount: 0,
      ratingCounts: [{ rating: 0, count: 500 }],
    };

    const result = evaluateDirectoryCompletion(dir, stats);

    expect(result.isCompleted).toBe(true);
    expect(result.keepCount).toBe(0);
  });
});
// #endregion
