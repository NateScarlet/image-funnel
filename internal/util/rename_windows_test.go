//go:build windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsTransientRenameError 判据只认 Windows 瞬时占用错误码：
// 其余错误（不存在、磁盘满等）必须判为不可重试，否则会把真正的失败拖成延迟失败。
func TestIsTransientRenameError(t *testing.T) {
	t.Run("access denied should be transient", func(t *testing.T) {
		err := &os.PathError{Op: "rename", Path: "x", Err: syscall.Errno(errnoAccessDenied)}
		assert.True(t, isTransientRenameError(err))
	})
	t.Run("sharing violation should be transient", func(t *testing.T) {
		err := &os.PathError{Op: "rename", Path: "x", Err: syscall.Errno(errnoSharingViolation)}
		assert.True(t, isTransientRenameError(err))
	})
	t.Run("lock violation should be transient", func(t *testing.T) {
		err := &os.PathError{Op: "rename", Path: "x", Err: syscall.Errno(errnoLockViolation)}
		assert.True(t, isTransientRenameError(err))
	})
	t.Run("not exist should not be transient", func(t *testing.T) {
		err := &os.PathError{Op: "rename", Path: "x", Err: syscall.ENOENT}
		assert.False(t, isTransientRenameError(err), "文件不存在重试无意义，应快速失败")
	})
	t.Run("non path error should not be transient", func(t *testing.T) {
		assert.False(t, isTransientRenameError(errors.New("some other error")))
	})
	t.Run("nil should not be transient", func(t *testing.T) {
		assert.False(t, isTransientRenameError(nil))
	})
}

// TestRenameWithRetry_TransientOccupancy 重命名遇到瞬时占用时的重试行为（判据仅 Windows 生效）
func TestRenameWithRetry_TransientOccupancy(t *testing.T) {
	t.Run("should retry transient error until success", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src")
		dst := filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(src, []byte("A"), 0o644))

		// 前 2 次模拟瞬时占用，第 3 次放行
		calls := 0
		err := renameWithRetry(src, dst, func(oldPath, newPath string) error {
			calls++
			if calls <= 2 {
				return &os.PathError{Op: "rename", Path: oldPath, Err: syscall.Errno(errnoAccessDenied)}
			}
			return os.Rename(oldPath, newPath)
		}, WithRenameBackoff([]time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}))
		require.NoError(t, err)
		assert.Equal(t, 3, calls, "应在第 3 次尝试成功")
	})

	t.Run("should retry immediately with empty backoff", func(t *testing.T) {
		calls := 0
		err := renameWithRetry("src", "dst", func(oldPath, newPath string) error {
			calls++
			if calls == 1 {
				return &os.PathError{Op: "rename", Path: oldPath, Err: syscall.Errno(errnoAccessDenied)}
			}
			return nil
		}, WithRenameBackoff(nil))
		require.NoError(t, err)
		assert.Equal(t, 2, calls, "空退避表表示立即重试，不应等待也不应越界")
	})

	t.Run("should return last error after exhausting retries", func(t *testing.T) {
		calls := 0
		sentinel := fmt.Errorf("injected denial")
		err := renameWithRetry("src", "dst", func(oldPath, newPath string) error {
			calls++
			return &os.PathError{Op: "rename", Path: oldPath, Err: syscall.Errno(errnoAccessDenied)}
		}, WithRenameBackoff([]time.Duration{time.Millisecond}), WithRenameMaxRetries(2))
		require.Error(t, err, sentinel)
		assert.Equal(t, 3, calls, "首次 + 2 次重试 = 3 次尝试")
		// 底层错误须可经 errors.Is 命中，便于调用方诊断
		assert.True(t, errors.Is(err, syscall.Errno(errnoAccessDenied)))
		assert.True(t, isTransientRenameError(err))
	})
}
