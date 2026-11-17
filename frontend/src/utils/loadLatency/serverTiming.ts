/** 从 Server-Timing 响应头解析出的单个指标 */
export interface ServerTimingMetric {
  name: string;
  durationMs: number;
  description?: string;
}

/**
 * 解析 Server-Timing 头。服务端固定输出 queue/decode/encode/cache 四项；
 * 读取侧不假设指标齐全，缺 dur 的项按 0 处理，畸形项直接跳过。
 */
export function parseServerTiming(header: string | null | undefined): ServerTimingMetric[] {
  if (!header) {
    return [];
  }
  return splitTopLevel(header, ",")
    .map(parseMetric)
    .filter((metric): metric is ServerTimingMetric => metric !== undefined);
}

function parseMetric(part: string): ServerTimingMetric | undefined {
  const [name, ...params] = splitTopLevel(part, ";").map((token) => token.trim());
  if (!name) {
    return undefined;
  }
  const metric: ServerTimingMetric = { name, durationMs: 0 };
  for (const param of params) {
    const equal = param.indexOf("=");
    if (equal < 0) {
      continue;
    }
    const key = param.slice(0, equal).trim();
    let value = param.slice(equal + 1).trim();
    if (value.length >= 2 && value.startsWith('"') && value.endsWith('"')) {
      value = value.slice(1, -1).replace(/\\(["\\])/g, "$1");
    }
    if (key === "dur") {
      const duration = Number.parseFloat(value);
      if (Number.isFinite(duration)) {
        metric.durationMs = duration;
      }
    } else if (key === "desc") {
      metric.description = value;
    }
  }
  return metric;
}

/** 按分隔符切分，忽略引号内与转义后的分隔符 */
function splitTopLevel(input: string, delimiter: string): string[] {
  const parts: string[] = [];
  let current = "";
  let inQuotes = false;
  let escaped = false;
  for (const char of input) {
    if (escaped) {
      current += char;
      escaped = false;
      continue;
    }
    if (char === "\\" && inQuotes) {
      current += char;
      escaped = true;
      continue;
    }
    if (char === '"') {
      inQuotes = !inQuotes;
      current += char;
      continue;
    }
    if (char === delimiter && !inQuotes) {
      parts.push(current);
      current = "";
      continue;
    }
    current += char;
  }
  parts.push(current);
  return parts;
}
