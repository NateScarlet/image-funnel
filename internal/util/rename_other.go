//go:build !windows

package util

import "os"

// isTransientRenameError 在非 Windows 平台恒为 false：
// 类 Unix 系统允许对被打开的文件重命名（目录项与 inode 分离），不存在同类的瞬时占用失败，
// 故不做重试、保持快速失败。
func isTransientRenameError(error) bool {
	return false
}

// renameFile 执行一次重命名，平台实现直接委托标准库。
func renameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
