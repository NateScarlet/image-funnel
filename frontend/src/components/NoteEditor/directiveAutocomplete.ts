export interface OptionInfo {
  name: string; // 选项名，例如 '--region' 或 '-u'
  shortName?: string; // 选项简写，例如 '-u'
  placeholder?: string; // 选项参数的占位符，例如 '<region>'
  raw: string; // 原始字符串，例如 '--region <region>' 或 '-u'
  description?: string; // 该选项的具体说明
  repeatable?: boolean; // 该选项是否可多次指定，即语法定义中跟随 '...'
}

export interface PatternToken {
  type: "subcommand" | "positional";
  value: string; // 例如 'lora' 或 '<name>'
}

export interface DirectiveRule {
  directive: string; // 例如 'adjust'
  pattern: PatternToken[];
  options: OptionInfo[];
  description: string; // 整体描述
  generalDescription?: string; // 通用说明
}

export interface Suggestion {
  type: string;
  text: string; // 插入到文本框的完整文本，例如 'lora' 或 '--region <region>' 或 '<prompt>'
  displayText: string; // 浮层中显示的友好文本
  placeholder?: string; // 占位符信息，例如 '<region>'
  description?: string; // 提示的描述文本
  style?: string; // 额外的视觉显示样式状态，如 'muted'
}

/**
 * 解析 usage 文本，提取其中定义的语法规则、描述和选项说明
 */
export function parseUsage(usage: string): DirectiveRule[] {
  const lines = usage.split(/\r?\n/);
  const optionDescriptions = new Map<string, string>();
  const shortToLong = new Map<string, string>();
  const longToShort = new Map<string, string>();

  // 正则匹配选项说明行，如：
  // "--skip-add             提示词不存在时跳过添加"
  // "-u --update-seed       强制启用随机种子更新"
  // "-j --jobs <N>          每个权重版本发送工作流次数"
  // 支持描述仅以单个空格分隔的情况
  const optionDescRegex =
    /^[ \t]*(?:(-[a-zA-Z0-9])(?:,?\s+(--[a-zA-Z0-9_-]+))?|(--[a-zA-Z0-9_-]+))(?:\s+(<[^>]+>))?\s+(.+)$/;

  const rules: DirectiveRule[] = [];
  const rulePrivateDescs = new Map<DirectiveRule, string[]>();
  const generalDescLines: string[] = [];

  let currentRule: DirectiveRule | null = null;
  let isCollectingDesc = false;

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      isCollectingDesc = false;
      continue;
    }

    if (trimmed.startsWith("/")) {
      // 匹配并解析规则行
      const dirMatch = trimmed.match(/^\/([a-zA-Z0-9_-]+)(.*)$/);
      if (dirMatch) {
        const directive = dirMatch[1];
        const remaining = dirMatch[2].trim();
        const options: OptionInfo[] = [];

        // 匹配并移除所有中括号包裹的可选选项，如 [-u] 或 [--region <region>]...
        const optRegex = /\[([^\]]+)\](?:\.\.\.)?/g;
        let optMatch;

        while ((optMatch = optRegex.exec(remaining)) !== null) {
          const matchedStr = optMatch[0];
          const inner = optMatch[1].trim();
          const parts = inner.split(/\s+/);
          if (parts.length === 0) continue;

          const name = parts[0];
          let placeholder: string | undefined;
          if (parts.length > 1) {
            placeholder = parts.slice(1).join(" ");
          }

          options.push({
            name,
            placeholder,
            raw: inner,
            repeatable: matchedStr.endsWith("..."),
          });
        }

        // 移除所有方括号块
        const cleanedRemaining = remaining.replace(optRegex, "").trim();

        // 此时 cleanedRemaining 只有子命令和位置参数，如 "lora <name> <weight>"
        const patternTokens: PatternToken[] = [];
        const rawTokens = cleanedRemaining.split(/\s+/).filter(Boolean);

        for (const token of rawTokens) {
          if (token.startsWith("<") && (token.endsWith(">") || token.endsWith(">..."))) {
            patternTokens.push({
              type: "positional",
              value: token,
            });
          } else {
            patternTokens.push({
              type: "subcommand",
              value: token,
            });
          }
        }

        const rule: DirectiveRule = {
          directive,
          pattern: patternTokens,
          options,
          description: "", // 稍后填充
        };

        rules.push(rule);
        rulePrivateDescs.set(rule, []);
        currentRule = rule;
        isCollectingDesc = true;
      }
    } else {
      const match = line.match(optionDescRegex);
      if (match) {
        isCollectingDesc = false;
        const shortOpt = match[1]; // 例如 '-u'
        const longOpt = match[2] || match[3]; // 例如 '--update-seed'
        const desc = match[5].trim();

        if (longOpt) {
          optionDescriptions.set(longOpt, desc);
        }
        if (shortOpt) {
          optionDescriptions.set(shortOpt, desc);
        }
        if (shortOpt && longOpt) {
          shortToLong.set(shortOpt, longOpt);
          longToShort.set(longOpt, shortOpt);
        }
      } else {
        // 普通描述行
        const isOptionsHeader = /^[ \t]*选项(说明)?[：:]?$/.test(trimmed);
        if (!isOptionsHeader) {
          if (isCollectingDesc && currentRule) {
            const descs = rulePrivateDescs.get(currentRule);
            if (descs) {
              descs.push(trimmed);
            }
          } else {
            generalDescLines.push(trimmed);
          }
        }
      }
    }
  }

  const generalDescription = generalDescLines.join("\n");

  for (const rule of rules) {
    // 规范化选项名称：短选项规范为长选项，并保存 alias 短选项
    for (const opt of rule.options) {
      const longName = shortToLong.get(opt.name);
      if (longName) {
        opt.shortName = opt.name;
        opt.name = longName;
      } else {
        const shortName = longToShort.get(opt.name);
        if (shortName) {
          opt.shortName = shortName;
        }
      }
    }

    // 填充选项的专属说明
    for (const opt of rule.options) {
      const desc =
        optionDescriptions.get(opt.name) ||
        (opt.shortName ? optionDescriptions.get(opt.shortName) : undefined);
      if (desc) {
        opt.description = desc;
      }
    }

    // 组装最终描述
    const privateDesc = rulePrivateDescs.get(rule)?.join("\n") || "";
    rule.generalDescription = generalDescription;
    if (privateDesc) {
      rule.description = privateDesc;
    } else {
      rule.description = generalDescription;
    }
  }

  return rules;
}

/**
 * 分词并识别当前输入行的参数上下文
 */
export function getArgsContext(argsText: string): {
  confirmedTokens: string[];
  currentQuery: string;
} {
  // 剥离最前导的一个空格，若 argsText 本身只包含空白则 cleaned 为空
  const cleaned = argsText.replace(/^\s/, "");
  const tokens: string[] = [];
  let current = "";
  let inQuote: string | null = null;

  for (let i = 0; i < cleaned.length; i++) {
    const char = cleaned[i];
    if (inQuote) {
      current += char;
      if (char === inQuote) {
        inQuote = null;
      }
    } else {
      if (char === '"' || char === "'") {
        inQuote = char;
        current += char;
      } else if (/\s/.test(char)) {
        if (current !== "") {
          tokens.push(current);
          current = "";
        }
      } else {
        current += char;
      }
    }
  }
  tokens.push(current); // 最后一个即使是空也保留，代表正在输入

  const confirmedTokens = tokens.slice(0, -1);
  const currentQuery = tokens[tokens.length - 1];

  return { confirmedTokens, currentQuery };
}

/**
 * 根据已输入完毕的 tokens 列表，过滤出特定规则模式下的模式匹配 token 序列
 */
function getPatternTokens(confirmedTokens: string[], rule: DirectiveRule): string[] {
  const result: string[] = [];
  let i = 0;
  while (i < confirmedTokens.length) {
    const token = confirmedTokens[i];
    // 匹配选项定义（同时支持长选项与短选项匹配识别）
    const opt = rule.options.find((o) => o.name === token || o.shortName === token);
    if (opt) {
      if (opt.placeholder) {
        // 如果是带参选项，忽略它自己和它的参数值
        i += 2;
      } else {
        // 如果是无参选项，仅忽略它自己
        i += 1;
      }
    } else {
      result.push(token);
      i += 1;
    }
  }
  return result;
}

/**
 * 核心补全匹配算法：根据已有的 token 匹配各个用法定义，并给出当前光标下的补全建议
 */
export function getSuggestionsForRules(
  rules: DirectiveRule[],
  confirmedTokens: string[],
  query: string,
): Suggestion[] {
  const list: Suggestion[] = [];
  const q = query.toLowerCase();

  for (const rule of rules) {
    // 1. 判断 confirmedTokens 是否符合该 rule 模式
    const patternTokens = getPatternTokens(confirmedTokens, rule);
    if (patternTokens.length > rule.pattern.length) {
      // 已经超出了预定义的位置参数长度
      continue;
    }

    let isMatch = true;
    for (let idx = 0; idx < patternTokens.length; idx++) {
      const p = rule.pattern[idx];
      const t = patternTokens[idx];
      if (p.type === "subcommand" && p.value !== t) {
        isMatch = false;
        break;
      }
    }
    if (!isMatch) continue;

    // 2. 匹配成功，根据当前 query 产生建议
    // 情况 A：用户正在输入选项 (query 以 '-' 开头)
    if (q.startsWith("-")) {
      for (const opt of rule.options) {
        const matchesOption =
          opt.name.toLowerCase().includes(q) ||
          (opt.shortName && opt.shortName.toLowerCase().includes(q));
        if (matchesOption) {
          // 检查该选项是否已经存在于 confirmedTokens 中（根据语法规则判断是否为可多次指定选项，若是则允许再次出现）
          const isRepeatable = !!opt.repeatable;

          const hasAlready =
            confirmedTokens.includes(opt.name) ||
            (opt.shortName && confirmedTokens.includes(opt.shortName));
          if (!isRepeatable && hasAlready) {
            continue;
          }

          const optionText = opt.placeholder ? `${opt.name} ${opt.placeholder}` : opt.name;
          const display = opt.shortName ? `${optionText} (${opt.shortName})` : optionText;

          list.push({
            type: "option",
            text: optionText,
            displayText: display,
            placeholder: opt.placeholder,
            description: opt.description || rule.description,
          });
        }
      }
    } else {
      // 情况 B：用户没有输入 '-'。
      // 我们优先建议下一个位置参数/子命令
      const nextPattern = rule.pattern[patternTokens.length];
      if (nextPattern) {
        if (nextPattern.type === "subcommand") {
          if (nextPattern.value.toLowerCase().startsWith(q)) {
            list.push({
              type: "subcommand",
              text: nextPattern.value,
              displayText: nextPattern.value,
              description: rule.description,
            });
          }
        } else {
          // 下一个是位置参数 (如 <name> 或 <prompt>)
          // 位置参数只有在 query 为空时显示，或者用户打字正好符合时
          const val = nextPattern.value;
          // 去除 < > 并移除尾随的 ... 得到名字
          const cleanName = val.replace(/[<>]/g, "").replace(/\.\.\.$/, "");
          if (!q || cleanName.toLowerCase().startsWith(q) || val.toLowerCase().startsWith(q)) {
            list.push({
              type: "positional",
              text: val,
              displayText: val,
              placeholder: val,
              description: rule.description,
            });
          }
        }
      }

      // 我们也同时匹配符合 query 的选项（例如用户输入 'reg' 或 '-j' 时，匹配出 '--region <region>'）
      for (const opt of rule.options) {
        const isRepeatable = !!opt.repeatable;

        const hasAlready =
          confirmedTokens.includes(opt.name) ||
          (opt.shortName && confirmedTokens.includes(opt.shortName));
        if (!isRepeatable && hasAlready) {
          continue;
        }

        // 无论 q 是否为空，都推荐选项（若 q 为空，则推荐所有选项；若不为空，则进行过滤，支持匹配长选项和短选项）
        const matchesOption =
          !q ||
          opt.name.toLowerCase().includes(q) ||
          (opt.shortName && opt.shortName.toLowerCase().includes(q)) ||
          opt.raw.toLowerCase().includes(q);
        if (matchesOption) {
          const optionText = opt.placeholder ? `${opt.name} ${opt.placeholder}` : opt.name;
          const display = opt.shortName ? `${optionText} (${opt.shortName})` : optionText;

          list.push({
            type: "option",
            text: optionText,
            displayText: display,
            placeholder: opt.placeholder,
            description: opt.description || rule.description,
          });
        }
      }
    }
  }

  // 3. 去重与排序
  const seen = new Set<string>();
  const uniqueList: Suggestion[] = [];
  for (const item of list) {
    const key = `${item.type}:${item.text}`;
    if (!seen.has(key)) {
      seen.add(key);
      uniqueList.push(item);
    }
  }

  // 排序：优先子命令/位置参数，然后是选项；同类型保持定义顺序
  const typeOrder: Record<string, number> = { subcommand: 1, positional: 2, option: 3 };
  return uniqueList.toSorted((a, b) => {
    const aOrder = typeOrder[a.type];
    const bOrder = typeOrder[b.type];
    return (aOrder ?? 9) - (bOrder ?? 9);
  });
}
