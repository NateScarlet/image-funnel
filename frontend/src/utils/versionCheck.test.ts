import { describe, test, expect, vi } from "vitest";
import { createVersionCheck } from "./versionCheck";
import type { VersionCheckDeps } from "./versionCheck";

function makeDeps(overrides: Partial<VersionCheckDeps> = {}): VersionCheckDeps {
  return {
    builtVersion: "v1.2.3",
    fetchServerVersion: vi.fn(async () => "v1.2.3"),
    showStalePrompt: vi.fn(),
    clearStalePrompt: vi.fn(),
    ...overrides,
  };
}

describe("createVersionCheck", () => {
  test("服务端版本与构建版本不一致时显示失配提示", async () => {
    const deps = makeDeps({ fetchServerVersion: vi.fn(async () => "v2.0.0") });
    const check = createVersionCheck(deps);

    await check.checkOnConnected();

    expect(deps.showStalePrompt).toHaveBeenCalledExactlyOnceWith("v2.0.0");
    expect(deps.clearStalePrompt).not.toHaveBeenCalled();
  });

  test("失配提示已显示时重复检查不重复提示", async () => {
    const deps = makeDeps({ fetchServerVersion: vi.fn(async () => "v2.0.0") });
    const check = createVersionCheck(deps);

    await check.checkOnConnected();
    await check.checkOnConnected();
    await check.checkOnConnected();

    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);
  });

  test("服务端回滚到与构建一致的版本时清除已有提示", async () => {
    const deps = makeDeps({
      fetchServerVersion: vi.fn().mockResolvedValueOnce("v2.0.0").mockResolvedValueOnce("v1.2.3"),
    });
    const check = createVersionCheck(deps);

    await check.checkOnConnected();
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);

    await check.checkOnConnected();
    expect(deps.clearStalePrompt).toHaveBeenCalledTimes(1);
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);
  });

  test("版本一致且未显示提示时不做任何操作", async () => {
    const deps = makeDeps();
    const check = createVersionCheck(deps);

    await check.checkOnConnected();

    expect(deps.showStalePrompt).not.toHaveBeenCalled();
    expect(deps.clearStalePrompt).not.toHaveBeenCalled();
  });

  test("服务端查询失败时静默跳过本次比对且不清除已有提示", async () => {
    const deps = makeDeps({
      fetchServerVersion: vi.fn().mockResolvedValueOnce("v2.0.0").mockResolvedValue(undefined),
    });
    const check = createVersionCheck(deps);

    // 先制造一条已显示的失配提示
    await check.checkOnConnected();
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);

    await check.checkOnConnected();

    // 查询失败：既不新增提示，也不清除已有提示（等待下次连接再查）
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);
    expect(deps.clearStalePrompt).not.toHaveBeenCalled();
  });

  test("任一侧版本为 dev 时视为未知并跳过比对", async () => {
    const devBuiltDeps = makeDeps({
      builtVersion: "dev",
      fetchServerVersion: vi.fn(async () => "v2.0.0"),
    });
    const devBuiltCheck = createVersionCheck(devBuiltDeps);
    await devBuiltCheck.checkOnConnected();
    expect(devBuiltDeps.showStalePrompt).not.toHaveBeenCalled();

    const devServerDeps = makeDeps({
      fetchServerVersion: vi.fn(async () => "dev"),
    });
    const devServerCheck = createVersionCheck(devServerDeps);
    await devServerCheck.checkOnConnected();
    expect(devServerDeps.showStalePrompt).not.toHaveBeenCalled();
  });

  test("任一侧版本为空时视为未知并跳过比对", async () => {
    const emptyBuiltDeps = makeDeps({
      builtVersion: "",
      fetchServerVersion: vi.fn(async () => "v2.0.0"),
    });
    const emptyBuiltCheck = createVersionCheck(emptyBuiltDeps);
    await emptyBuiltCheck.checkOnConnected();
    expect(emptyBuiltDeps.showStalePrompt).not.toHaveBeenCalled();

    const emptyServerDeps = makeDeps({
      fetchServerVersion: vi.fn(async () => ""),
    });
    const emptyServerCheck = createVersionCheck(emptyServerDeps);
    await emptyServerCheck.checkOnConnected();
    expect(emptyServerDeps.showStalePrompt).not.toHaveBeenCalled();
  });

  test("懒加载资源失败时无论版本是否一致直接显示提示", () => {
    const deps = makeDeps();
    const check = createVersionCheck(deps);

    check.reportPreloadFailure();

    expect(deps.showStalePrompt).toHaveBeenCalledExactlyOnceWith(undefined);
  });

  test("提示已显示时资源失败不重复提示", async () => {
    const deps = makeDeps({ fetchServerVersion: vi.fn(async () => "v2.0.0") });
    const check = createVersionCheck(deps);

    await check.checkOnConnected();
    check.reportPreloadFailure();
    check.reportPreloadFailure();

    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);
  });

  test("手动关闭提示后再次检查失配会重新提示", async () => {
    const deps = makeDeps({ fetchServerVersion: vi.fn(async () => "v2.0.0") });
    const check = createVersionCheck(deps);

    await check.checkOnConnected();
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(1);

    check.dismissPrompt();
    expect(deps.clearStalePrompt).toHaveBeenCalledTimes(1);

    await check.checkOnConnected();
    expect(deps.showStalePrompt).toHaveBeenCalledTimes(2);
  });

  test("未显示提示时手动关闭不做任何操作", () => {
    const deps = makeDeps();
    const check = createVersionCheck(deps);

    check.dismissPrompt();

    expect(deps.clearStalePrompt).not.toHaveBeenCalled();
  });
});
