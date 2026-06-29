import { describe, test, expect } from "vitest";
import {
  parseUsage,
  getArgsContext,
  getSuggestionsForRules,
} from "./directiveAutocomplete";

const sampleUsage = `
/adjust lora <name> <weight> [-u] [-j <N>] [--no-skip]
/adjust prompt <text> <weight> [-u] [-j <N>] [--skip-add] [--no-skip] [--neg] [--region <region>]... [--node <node-id>]...
/adjust cfg <weight> [-u] [-j <N>] [--no-skip] [--node <node-id>]...

调整当前目录所有 4 星图片的 ComfyUI 工作流中的提示词权重、Lora 权重或者 KSampler 的 CFG 权重。

选项说明：
--skip-add             提示词不存在时跳过添加，默认不存在时会添加
--no-skip              即使没有任何修改也强制发送
-u --update-seed       强制启用随机种子更新
-j --jobs <N>          每个权重版本发送工作流次数
--neg                  当没有 region 或 node 匹配时使用负向
--region <region>      指定提示词目标区域，可多次指定
--node <node-id>       指定提示词目标节点，可多次指定
`;

describe("directiveAutocomplete", () => {
  test("parseUsage", () => {
    const rules = parseUsage(sampleUsage);
    expect(rules).toHaveLength(3);

    // 1. 检查指令名和描述
    const loraRule = rules[0];
    expect(loraRule.directive).toBe("adjust");
    expect(loraRule.description).toContain("调整当前目录所有 4 星图片");

    // 2. 检查模式列表
    expect(loraRule.pattern).toEqual([
      { type: "subcommand", value: "lora" },
      { type: "positional", value: "<name>" },
      { type: "positional", value: "<weight>" },
    ]);

    // 3. 检查选项专属描述绑定
    const jobsOpt = loraRule.options.find((o) => o.name === "--jobs");
    expect(jobsOpt).toBeDefined();
    expect(jobsOpt?.shortName).toBe("-j");
    expect(jobsOpt?.placeholder).toBe("<N>");
    expect(jobsOpt?.description).toBe("每个权重版本发送工作流次数");

    const skipAddRule = rules[1];
    const regionOpt = skipAddRule.options.find((o) => o.name === "--region");
    expect(regionOpt).toBeDefined();
    expect(regionOpt?.placeholder).toBe("<region>");
    expect(regionOpt?.description).toBe("指定提示词目标区域，可多次指定");

    const negOpt = skipAddRule.options.find((o) => o.name === "--neg");
    expect(negOpt).toBeDefined();
    expect(negOpt?.description).toBe("当没有 region 或 node 匹配时使用负向");
  });

  test("getArgsContext", () => {
    // 1. 输入完指令名后按空格，未打其他字符
    const ctx1 = getArgsContext(" ");
    expect(ctx1.confirmedTokens).toEqual([]);
    expect(ctx1.currentQuery).toBe("");

    // 2. 正在打第一个子命令
    const ctx2 = getArgsContext(" lo");
    expect(ctx2.confirmedTokens).toEqual([]);
    expect(ctx2.currentQuery).toBe("lo");

    // 3. 第一个子命令打完且输入了空格
    const ctx3 = getArgsContext(" lora ");
    expect(ctx3.confirmedTokens).toEqual(["lora"]);
    expect(ctx3.currentQuery).toBe("");

    // 4. 打了多个参数且正在打选项
    const ctx4 = getArgsContext(" lora name --re");
    expect(ctx4.confirmedTokens).toEqual(["lora", "name"]);
    expect(ctx4.currentQuery).toBe("--re");
  });

  test("getSuggestionsForRules", () => {
    const rules = parseUsage(sampleUsage);

    // 1. 输入 "/adjust " 推荐所有可选子命令
    const sugs1 = getSuggestionsForRules(rules, [], "");
    const cmdNames = sugs1
      .filter((s) => s.type === "subcommand")
      .map((s) => s.text);
    expect(cmdNames).toEqual(["cfg", "lora", "prompt"]);

    // 2. 输入 "/adjust l" 匹配 "lora"
    const sugs2 = getSuggestionsForRules(rules, [], "l");
    expect(sugs2).toHaveLength(1);
    expect(sugs2[0].text).toBe("lora");

    // 3. 输入 "/adjust lora " 推荐下一个位置参数 "<name>" 同时也推荐该分支所有可选选项
    const sugs3 = getSuggestionsForRules(rules, ["lora"], "");
    expect(sugs3.length).toBeGreaterThan(1);
    expect(sugs3[0].type).toBe("positional");
    expect(sugs3[0].text).toBe("<name>");
    expect(sugs3[0].placeholder).toBe("<name>");
    expect(
      sugs3.some((s) => s.type === "option" && s.text === "--no-skip"),
    ).toBe(true);

    // 4. 输入 "/adjust lora name " 推荐下一个位置参数 "<weight>" 同时也推荐所有可选选项
    const sugs4 = getSuggestionsForRules(rules, ["lora", "name"], "");
    expect(sugs4.length).toBeGreaterThan(1);
    expect(sugs4[0].text).toBe("<weight>");
    expect(
      sugs4.some((s) => s.type === "option" && s.text === "--no-skip"),
    ).toBe(true);

    // 5. 输入 "/adjust lora name weight --" 过滤出该分支下的所有选项
    const sugs5 = getSuggestionsForRules(
      rules,
      ["lora", "name", "weight"],
      "--",
    );
    const optNames = sugs5.map((s) => s.text);
    expect(optNames).toContain("--no-skip");
    expect(optNames).not.toContain("--region <region>");

    // 6. 输入 "/adjust prompt name weight --re" 过滤出 "--region <region>"
    const sugs6 = getSuggestionsForRules(
      rules,
      ["prompt", "name", "weight"],
      "--re",
    );
    expect(sugs6).toHaveLength(1);
    expect(sugs6[0].text).toBe("--region <region>");
    expect(sugs6[0].placeholder).toBe("<region>");

    // 7. 测试涉及到 region 的查询，即使没输入 '-'，输入 "reg" 也应该能召回
    const sugs7 = getSuggestionsForRules(
      rules,
      ["prompt", "name", "weight"],
      "reg",
    );
    const regSugs = sugs7.filter((s) => s.text.startsWith("--region"));
    expect(regSugs).toHaveLength(1);
    expect(regSugs[0].text).toBe("--region <region>");
    expect(regSugs[0].placeholder).toBe("<region>");
  });
});
