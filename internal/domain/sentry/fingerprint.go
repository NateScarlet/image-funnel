package sentry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxDirectoryNameBytes 单个指纹目录名的字节上限。
// 拼接值超限时保留前缀并以 ...(N more) 标注被省略的字符数，
// 使同一类事件始终落到同一个可读且长度受限的目录。
const MaxDirectoryNameBytes = 120

// DirectoryName 把所有指纹元素拼成单层目录名。
// 元素间用下划线连接；清洗只替换文件系统非法字符，Unicode 原样保留，
// 保证结果不含路径分隔符、遍历序列或平台非法字符。
func DirectoryName(elements []string) string {
	parts := make([]string, 0, len(elements))
	for _, element := range elements {
		if sanitized := sanitize(element); sanitized != "" {
			parts = append(parts, sanitized)
		}
	}
	if len(parts) == 0 {
		return "default"
	}

	joined := strings.Join(parts, "_")
	if len(joined) <= MaxDirectoryNameBytes {
		return joined
	}

	// 标记长度依赖省略数，反向找到能装进上限的保留长度
	keep := MaxDirectoryNameBytes
	for {
		marker := fmt.Sprintf("...(%d more)", len(joined)-keep)
		if keep+len(marker) <= MaxDirectoryNameBytes {
			return truncateAtRuneBoundary(joined, keep) + marker
		}
		keep--
	}
}

// truncateAtRuneBoundary 在字节上限内截断，且不会把多字节字符切成非法 UTF-8。
func truncateAtRuneBoundary(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

// invalidFileNameCharPattern 只匹配真正不能出现在文件名中的字符与形态：
// 路径/控制字符、点序列（防止 .. 遍历）、尾点空格与 Windows 保留名。
// Unicode 字母数字不在其中，中文错误消息得以在目录名中保持可读。
var invalidFileNameCharPattern = regexp.MustCompile(`[/?<>\\:*|"\x00-\x1f\x80-\x9f]|\.{2,}|^\.+$|[\. ]+$|(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// Sanitize 把任意字符串转成文件系统安全的单个名称片段，供指纹目录与事件文件名共用。
func Sanitize(value string) string {
	return sanitize(value)
}

func sanitize(value string) string {
	// strings.Map 遍历时会把非法 UTF-8 字节解码成 U+FFFD，这里直接归一为下划线，
	// 避免落盘路径出现不可读、难以在命令行引用的替换字符。
	// 正则命中项整体替换为一个下划线：点序列、尾点空格与保留名按整段匹配，
	// 单个路径/控制字符则逐字替换。
	normalized := strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return '_'
		}
		return r
	}, value)
	return invalidFileNameCharPattern.ReplaceAllLiteralString(normalized, "_")
}

// EventFingerprint 优先返回事件显式设置的指纹；缺失时按 message/exception/logentry
// 的优先级推导可读元素，全部缺失时用事件自身时间戳生成 Event<UnixMicro> 标识。
// 不再回退到事件 JSON 的哈希：事件自带时间戳与 ID，哈希既不可读也不具备聚合能力。
func EventFingerprint(eventJSON []byte) []string {
	// UseNumber 保留数字原文，避免浮点格式化破坏时间戳的微秒精度
	decoder := json.NewDecoder(bytes.NewReader(eventJSON))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		// eventJSON 已由 ParseEnvelope 校验为合法 JSON，解码失败是不变量被破坏
		panic(fmt.Errorf("decode sentry event fingerprint: %w", err))
	}
	fields, _ := decoded.(map[string]any) // 非对象事件等价于没有任何可读字段

	if explicit := explicitFingerprint(fields); len(explicit) > 0 {
		return explicit
	}
	if message, ok := fields["message"]; ok && message != nil {
		return []string{fmt.Sprintf("%s: %s", fieldString(fields["level"]), fieldString(message))}
	}
	if exception, ok := fields["exception"]; ok && exception != nil {
		if fingerprint := exceptionFingerprint(exception); fingerprint != "" {
			return []string{fingerprint}
		}
		return []string{eventFallback(fields)}
	}
	if logEntry, ok := fields["logentry"]; ok && logEntry != nil {
		return []string{fmt.Sprintf("%s: %s: %s",
			fieldString(fields["level"]), fieldString(fields["logger"]), fieldString(logEntry),
		)}
	}
	return []string{eventFallback(fields)}
}

// #region 事件字段推导
// explicitFingerprint 取显式指纹中的非空元素。
func explicitFingerprint(fields map[string]any) []string {
	elements, ok := fields["fingerprint"].([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, element := range elements {
		if text := fieldString(element); text != "" {
			result = append(result, text)
		}
	}
	return result
}

// exceptionFingerprint 取第一个异常元素的 type 与 value；value 为空时只保留 type。
func exceptionFingerprint(value any) string {
	exception, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	values, ok := exception["values"].([]any)
	if !ok || len(values) == 0 {
		return ""
	}
	first, ok := values[0].(map[string]any)
	if !ok {
		return ""
	}
	typ := fieldString(first["type"])
	message := fieldString(first["value"])
	switch {
	case typ == "":
		return message
	case message == "":
		return typ
	default:
		return typ + ": " + message
	}
}

// eventFallback 用事件自身时间戳的微秒数生成可读标识。
func eventFallback(fields map[string]any) string {
	return fmt.Sprintf("Event%d", eventTime(fields["timestamp"]).UnixMicro())
}

// fieldString 把字段值转成可读文本：字符串取内容，数字保留原文，对象/数组保留 JSON 文本。
func fieldString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			panic(err) // 来自 JSON 解码的值不可能 marshal 失败
		}
		return string(data)
	}
}

// timeFromUnixSecondsText 按「秒.微秒」文本精确还原时间，浮点相减会吞掉末位微秒。
func timeFromUnixSecondsText(text string) time.Time {
	wholeText, fracText, hasFraction := strings.Cut(text, ".")
	seconds, err := strconv.ParseInt(wholeText, 10, 64)
	if err != nil {
		// 指数记法等非常规表示回退到浮点解析，json.Number 保证文本合法
		value, parseErr := strconv.ParseFloat(text, 64)
		if parseErr != nil {
			panic(parseErr)
		}
		return time.UnixMicro(int64(value * 1e6))
	}
	if !hasFraction {
		return time.Unix(seconds, 0)
	}
	if len(fracText) > 6 {
		fracText = fracText[:6]
	}
	fracText += strings.Repeat("0", 6-len(fracText))
	micros, err := strconv.ParseInt(fracText, 10, 64)
	if err != nil {
		panic(err) // 小数部分只含数字，json.Number 保证文本合法
	}
	return time.Unix(seconds, micros*int64(time.Microsecond))
}

// eventTime 解析事件自身时间戳，零值是确定性的安全兜底，绝不退化成当前时间。
func eventTime(value any) time.Time {
	switch typed := value.(type) {
	case json.Number:
		return timeFromUnixSecondsText(typed.String())
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, typed)
		if err != nil {
			// 外部事件可能携带非 RFC3339 时间戳，按缺失处理
			return time.Time{}
		}
		return parsed
	default:
		return time.Time{}
	}
}

// #endregion
