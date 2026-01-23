package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

func EnsurePathInRoot(rootDir, relPath string) error {
	// 将路径统一转换为正斜杠分隔符，以便跨平台处理（特别是输入可能包含 Windows 风格的路径）
	normalizedRel := filepath.ToSlash(relPath)

	// 检查是否为绝对路径
	// filepath.IsAbs 在 Windows 上可以处理驱动器盘符，但在 Linux 上不行
	// 因此我们需要手动检查驱动器盘符（如 C:）以及是否以斜杠开头以保证安全性
	if filepath.IsAbs(normalizedRel) || (len(normalizedRel) >= 2 && normalizedRel[1] == ':') || strings.HasPrefix(normalizedRel, "/") {
		return fmt.Errorf("absolute path not allowed")
	}

	// 使用 filepath.Clean 清理路径，它会处理路径中的 . 和 ..
	cleanedRel := filepath.Clean(normalizedRel)

	// 检查是否为路径遍历：需要检查路径元素而非字符串前缀
	// 路径等于 ".." 或以 "../" 开头才是真正的路径遍历
	// 注意：不能简单用 HasPrefix，否则会误判 "..not_escape" 这样的合法文件名
	if cleanedRel == ".." || strings.HasPrefix(cleanedRel, "../") {
		return fmt.Errorf("path escapes root directory")
	}

	// 将清理后的路径与根目录拼接
	// filepath.Join 会根据当前操作系统自动选择分隔符
	absPath := filepath.Join(rootDir, filepath.FromSlash(cleanedRel))

	// 使用 filepath.Rel 进行最终验证，确保计算出的相对路径不会逃离根目录
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 检查相对路径是否以 ".." 作为路径元素开头（而不是字符串前缀）
	// 例如 "../escape" 应该被拒绝，但 "..not_escape" 应该被接受
	// 注意：filepath.Rel 保证返回相对路径，不需要检查 IsAbs
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 0 && parts[0] == ".." {
		return fmt.Errorf("path escapes root directory")
	}

	return nil

}
