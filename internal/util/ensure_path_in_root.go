package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

func EnsurePathInRoot(rootDir, relPath string) error {
	// 将输入的路径（通常是 Web/API 使用的正斜杠分隔路径）转换为当前操作系统的原生格式
	nativeRel := filepath.FromSlash(relPath)

	// 检查是否为绝对路径
	// 1. 使用 filepath.IsAbs (在 Windows 上需要有盘符或 UNC 路径才算绝对)
	// 2. 检查 Windows 卷名 (处理 C:foo 这种路径)
	// 3. 检查是否以路径分隔符开头 (处理 Windows 上的 \foo 驱动器相对路径，以及 Linux 上的 /foo)
	if filepath.IsAbs(nativeRel) || filepath.VolumeName(nativeRel) != "" || strings.HasPrefix(nativeRel, string(filepath.Separator)) {
		return fmt.Errorf("absolute path not allowed")
	}

	// 使用 filepath.Clean 清理路径，处理 . 和 ..
	cleanedRel := filepath.Clean(nativeRel)

	// 将清理后的相对路径拼接到根目录
	absPath := filepath.Join(rootDir, cleanedRel)

	// 使用 filepath.Rel 重新计算相对路径并验证
	// 这能有效防止通过 Join 产生的逃逸（例如在 Windows 上跨越驱动器盘符）
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 最终验证：检查得到的相对路径是否尝试回到父目录
	// 我们需要检查它是否等于 ".." 或者以 ".." 分隔符开头
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root directory")
	}

	return nil
}
