package sentry

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// #region 脱敏

var (
	// 领域 ID 在消息里以带前缀的形式出现，换前缀 pseudonymize 后仍可分类比对
	idPattern = regexp.MustCompile(`(dir:[^\s"'<>]+|img:\d+:[^\s"'<>]+)`)
	// 裸路径（含 Windows 盘符与反斜杠）不能原样离开进程
	pathPattern = regexp.MustCompile(`[^\s"'<>()]+[\\/][^\s"'<>()]*`)
	// URL userinfo 可能携带凭据，整段替换
	userInfoPattern = regexp.MustCompile(`(?i)(https?://)[^/\s@]+@`)
	// 常见凭据赋值：token/password/api_key 等
	credentialPattern = regexp.MustCompile(`(?i)\b(token|secret|password|passwd|api[_-]?key|authorization|setup_token)\s*[=:]\s*([^\s"'&,;]+)`)
	bearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand 不可用时无法安全上报，快速失败
	}
	return hex.EncodeToString(buf)
}

func newSalt() string { return randomHex(16) }

func newEventID() string { return randomHex(16) }

// pseudonymize 与前端 telemetryScrub 共用同一算法：HMAC-SHA256 取前 8 字节。
func pseudonymize(value, salt string) string {
	mac := hmac.New(sha256.New, []byte(salt))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

func scrubID(value, salt, prefix string) string {
	return prefix + pseudonymize(value, salt)
}

// scrubString 按「凭据 → 标识 → 路径」顺序清洗，保证前一步产生的假名不被后续规则二次改写。
func scrubString(value, salt string) string {
	value = userInfoPattern.ReplaceAllString(value, "$1redacted@")
	// Bearer 必须先处理：否则 credentialPattern 会把 "Authorization: Bearer" 整体吃掉，
	// 留下后面的明文 token。
	value = bearerPattern.ReplaceAllStringFunc(value, func(match string) string {
		return "bearer " + scrubID(strings.TrimSpace(match[len("bearer"):]), salt, "cred:")
	})
	value = credentialPattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := credentialPattern.FindStringSubmatch(match)
		return groups[1] + "=" + scrubID(groups[2], salt, "cred:")
	})
	value = idPattern.ReplaceAllStringFunc(value, func(match string) string {
		prefix := "dir:"
		if len(match) >= 4 && match[:4] == "img:" {
			prefix = "img:"
		}
		return scrubID(match, salt, prefix)
	})
	value = pathPattern.ReplaceAllStringFunc(value, func(match string) string {
		return scrubID(match, salt, "path:")
	})
	return value
}

// scrubValue 递归脱敏整个事件；scrubSalt 等同名标记字段原样保留。
func scrubValue(value any, salt string) any {
	switch typed := value.(type) {
	case string:
		return scrubString(typed, salt)
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = scrubValue(item, salt)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "scrubSalt" {
				result[key] = item
				continue
			}
			result[key] = scrubValue(item, salt)
		}
		return result
	default:
		return value
	}
}

// #endregion
