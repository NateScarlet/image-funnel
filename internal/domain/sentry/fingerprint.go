package sentry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// MaxDirectoryNameBytes 单个指纹目录名的字节上限。
// 拼接值超限时保留前缀并以 ...(N more) 标注被省略的字符数，
// 使同一类事件始终落到同一个可读且长度受限的目录。
const MaxDirectoryNameBytes = 120

// DirectoryName 把所有指纹元素拼成单层目录名。
// 元素间用下划线连接；非字母数字与下划线/连字符的字符一律替换，
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
			return joined[:keep] + marker
		}
		keep--
	}
}

// Sanitize 把任意字符串转成文件系统安全的单个名称片段，供指纹目录与事件文件名共用。
func Sanitize(value string) string {
	return sanitize(value)
}

func sanitize(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteByte('_')
		}
	}
	return result.String()
}

// ComputedFingerprint 在埋点未指定指纹时按事件内容计算稳定指纹。
func ComputedFingerprint(eventJSON []byte) string {
	sum := sha256.Sum256(eventJSON)
	return "sha256-" + hex.EncodeToString(sum[:])
}

// EventFingerprint 优先返回事件显式设置的指纹；缺失或全空时按内容计算。
func EventFingerprint(eventJSON []byte) []string {
	var event struct {
		Fingerprint []string `json:"fingerprint"`
	}
	if err := json.Unmarshal(eventJSON, &event); err == nil {
		var explicit []string
		for _, element := range event.Fingerprint {
			if element != "" {
				explicit = append(explicit, element)
			}
		}
		if len(explicit) > 0 {
			return explicit
		}
	}
	return []string{ComputedFingerprint(eventJSON)}
}
