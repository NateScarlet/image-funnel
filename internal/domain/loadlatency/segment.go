// Package loadlatency 定义加载延迟归因段的边界规则。
// 段内容由客户端产生，服务端只把它当作不透明字节追加；这里只守住可信边界的格式与体积。
package loadlatency

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// MaxSegmentBytes 单段序列化后的体积上限（2MB）。
// 超限必须整段拒绝，不能静默截断——截断会让动作序列不完整且无法被读取方察觉。
const MaxSegmentBytes = 2 << 20

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidateRunID 校验客户端生成的 runId：32 位小写十六进制（UUIDv4 去连字符形态）。
func ValidateRunID(runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("invalid run id %q: must be 32 lowercase hex characters", runID)
	}
	return nil
}

// ValidateSegmentSize 校验段体积。上限含边界值，超出即拒绝。
func ValidateSegmentSize(data []byte) error {
	if len(data) > MaxSegmentBytes {
		return fmt.Errorf("segment too large: %d bytes exceeds limit %d", len(data), MaxSegmentBytes)
	}
	return nil
}

// SegmentPath 由落盘目录与 runId 计算段文件路径。
// 路径是 runId 的纯函数：不维护索引、不读已有内容、不依赖内存状态。
func SegmentPath(dir, runID string) string {
	return filepath.Join(dir, runID+".jsonl")
}
