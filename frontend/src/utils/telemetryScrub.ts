import { hmac } from "@noble/hashes/hmac.js";
import { sha256 } from "@noble/hashes/sha2.js";

// #region 脱敏契约
/** 事件中被视为标识/用户数据、必须在发送前替换的字段 */
const ID_FIELD_PREFIXES: Record<string, string> = {
  sessionId: "session:",
  directoryId: "dir:",
  nextDirectoryId: "dir:",
  imageId: "img:",
  directoryRelPath: "path:",
};

/** 游标字段编码的是目录名 hex、排序键或用户搜索词，全部按标识处理 */
const CURSOR_FIELDS = new Set(["after", "sortKey", "query"]);

export interface ScrubbableEvent {
  message?: string;
  request?: { url?: string };
  exception?: {
    values?: {
      type?: string;
      value?: string;
      stacktrace?: {
        frames?: {
          filename?: string;
          abs_path?: string;
          function?: string;
          module?: string;
          context_line?: string;
          pre_context?: string[];
          post_context?: string[];
        }[];
      };
    }[];
  };
  extra?: Record<string, unknown>;
  tags?: Record<string, unknown>;
  contexts?: Record<string, unknown>;
  breadcrumbs?: { message?: string; data?: Record<string, unknown> }[];
}
// #endregion

/** 每个事件独立随机盐：同事件内可互相比较，不同事件之间不可关联 */
export function newSalt(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/** 用盐做 HMAC-SHA256，只取前 8 字节，保留足够区分度又便于阅读 */
export function pseudonymize(value: string, salt: string): string {
  const digest = hmac(sha256, new TextEncoder().encode(salt), new TextEncoder().encode(value));
  return Array.from(digest.slice(0, 8), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/** 保留类型前缀（dir:/img:/session:），换成不可反推的伪标识 */
export function scrubId(value: string, salt: string, prefix: string): string {
  return prefix + pseudonymize(value, salt);
}

function isUrlLike(value: string): boolean {
  return /^https?:\/\//i.test(value) || value.startsWith("/");
}

function scrubUrl(url: string, salt: string): string {
  try {
    const parsed = new URL(url, "http://local");
    const origin = parsed.origin === "http://local" ? "" : parsed.origin;
    return origin + scrubId(parsed.pathname + parsed.search, salt, "/path:");
  } catch {
    return scrubId(url, salt, "url:");
  }
}

function scrubString(value: string, salt: string): string {
  return (
    value
      .replace(/(dir:[^\s"'<>]+|img:\d+:[^\s"'<>]+)/g, (match) => {
        const prefix = match.startsWith("img:") ? "img:" : "dir:";
        return scrubId(match, salt, prefix);
      })
      // 错误消息里的裸路径（含搜索词/项目名）同样不能原样上报
      .replace(/[^\s"'<>()]+[\\/][^\s"'<>()]*/g, (match) => scrubId(match, salt, "path:"))
  );
}

/** 已知字段用语义前缀；其它任何以 Id 结尾的字段也按 ID 脱敏，避免未来新增 ID 漏网 */
function idPrefix(key: string): string | undefined {
  return ID_FIELD_PREFIXES[key] ?? (key.endsWith("Id") ? "id:" : undefined);
}

function scrubRecord(data: Record<string, unknown>, salt: string): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(data)) {
    if (key === "scrubSalt") {
      result[key] = value;
    } else if (typeof value === "string" && idPrefix(key)) {
      const prefix = idPrefix(key);
      if (prefix) result[key] = scrubId(value, salt, prefix);
    } else if (typeof value === "string" && key === "message") {
      result[key] = scrubString(value, salt);
    } else if (typeof value === "string" && key === "url") {
      result[key] = isUrlLike(value) ? scrubUrl(value, salt) : scrubId(value, salt, "path:");
    } else if (key === "cursor" && value && typeof value === "object" && !Array.isArray(value)) {
      const cursor: Record<string, unknown> = {};
      for (const [cursorKey, cursorValue] of Object.entries(value)) {
        cursor[cursorKey] =
          typeof cursorValue === "string" && CURSOR_FIELDS.has(cursorKey)
            ? scrubId(cursorValue, salt, "cursor:")
            : cursorValue;
      }
      result[key] = cursor;
    } else if (key === "stats" && Array.isArray(value)) {
      result[key] = value.map((item) => {
        const entry = item as Record<string, unknown>;
        const scrubbed = scrubRecord(entry, salt);
        if (typeof entry.id === "string") {
          scrubbed.id = scrubId(entry.id, salt, "dir:");
        }
        return scrubbed;
      });
    } else if (Array.isArray(value)) {
      result[key] = value.map((item) =>
        item && typeof item === "object"
          ? scrubRecord(item as Record<string, unknown>, salt)
          : item,
      );
    } else if (value && typeof value === "object") {
      result[key] = scrubRecord(value as Record<string, unknown>, salt);
    } else {
      result[key] = value;
    }
  }
  return result;
}

/**
 * 发送前统一脱敏：所有标识字段用同一盐替换，盐写进 extra 供有本地目录的用户还原；
 * 统计、决策分支、错误分类等纯应用数据原样保留，不清空任何面包屑。
 */
function scrubOptionalString(value: string | undefined, salt: string): string | undefined {
  return value === undefined ? undefined : scrubString(value, salt);
}

function scrubStringList(values: string[] | undefined, salt: string): string[] | undefined {
  return values?.map((value) => scrubString(value, salt));
}

export function scrubEvent<T extends ScrubbableEvent>(event: T, salt: string): T {
  return {
    ...event,
    message: scrubOptionalString(event.message, salt),
    request: event.request?.url
      ? { ...event.request, url: scrubUrl(event.request.url, salt) }
      : event.request,
    exception: event.exception
      ? {
          ...event.exception,
          values: event.exception.values?.map((value) => ({
            ...value,
            type: scrubOptionalString(value.type, salt),
            value: scrubOptionalString(value.value, salt),
            stacktrace: value.stacktrace
              ? {
                  ...value.stacktrace,
                  frames: value.stacktrace.frames?.map((frame) => ({
                    ...frame,
                    filename: scrubOptionalString(frame.filename, salt),
                    abs_path: scrubOptionalString(frame.abs_path, salt),
                    function: scrubOptionalString(frame.function, salt),
                    module: scrubOptionalString(frame.module, salt),
                    context_line: scrubOptionalString(frame.context_line, salt),
                    pre_context: scrubStringList(frame.pre_context, salt),
                    post_context: scrubStringList(frame.post_context, salt),
                  })),
                }
              : value.stacktrace,
          })),
        }
      : event.exception,
    extra: { ...event.extra, ...scrubRecord(event.extra ?? {}, salt), scrubSalt: salt },
    tags: event.tags ? scrubRecord(event.tags, salt) : event.tags,
    contexts: event.contexts ? scrubRecord(event.contexts, salt) : event.contexts,
    breadcrumbs: event.breadcrumbs?.map((breadcrumb) => ({
      ...breadcrumb,
      message: breadcrumb.message
        ? isUrlLike(breadcrumb.message)
          ? scrubUrl(breadcrumb.message, salt)
          : scrubString(breadcrumb.message, salt)
        : breadcrumb.message,
      data: breadcrumb.data ? scrubRecord(breadcrumb.data, salt) : breadcrumb.data,
    })),
  };
}
