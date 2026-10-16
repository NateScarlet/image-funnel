//go:build windows

package util

import (
	"errors"
	"os"
	"syscall"
)

// Windows 文件占用相关错误码。
// 这些错误在「对方只是短暂持有句柄」时会出现（如编辑器/同步客户端/杀毒软件扫描刚写出的文件、
// 目录监视器对新文件做 stat），持有方通常在毫秒级释放，因此值得短暂退避后重试。
const (
	errnoAccessDenied     = 5  // ERROR_ACCESS_DENIED：目标被占用或权限不足
	errnoSharingViolation = 32 // ERROR_SHARING_VIOLATION：文件被以不共享方式打开
	errnoLockViolation    = 33 // ERROR_LOCK_VIOLATION：文件区域被锁定
)

// isTransientRenameError 判定重命名错误是否为「等待后可能自行消失」的瞬时占用错误。
// 只认上述三个错误码：其余（磁盘满、路径不存在、只读介质等）重试无意义，
// 必须立即上抛以保持快速失败。
func isTransientRenameError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch uintptr(errno) {
	case errnoAccessDenied, errnoSharingViolation, errnoLockViolation:
		return true
	}
	return false
}

// renameFile 执行一次重命名，平台实现直接委托标准库。
func renameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
