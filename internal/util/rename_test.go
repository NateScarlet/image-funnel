package util

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenameWithRetry 重命名重试的平台无关行为。
// 用注入的 rename 函数模拟占用：非瞬时错误必须立即返回，不做无谓重试。
// 「瞬时占用会重试」属于平台判据，见 rename_windows_test.go。
func TestRenameWithRetry(t *testing.T) {
	t.Run("should not retry on success", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src")
		dst := filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(src, []byte("A"), 0o644))

		calls := 0
		err := renameWithRetry(src, dst, func(oldPath, newPath string) error {
			calls++
			return os.Rename(oldPath, newPath)
		})
		require.NoError(t, err)
		assert.Equal(t, 1, calls, "成功不应重试")
	})

	t.Run("should fail fast on non transient error", func(t *testing.T) {
		calls := 0
		err := renameWithRetry("src", "dst", func(oldPath, newPath string) error {
			calls++
			return &os.PathError{Op: "rename", Path: oldPath, Err: syscall.ENOENT}
		})
		require.Error(t, err)
		assert.Equal(t, 1, calls, "非瞬时错误应立即返回，不重试")
		assert.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("should complete rename on disk", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src")
		dst := filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(src, []byte("A"), 0o644))

		require.NoError(t, RenameWithRetry(src, dst))
		data, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, []byte("A"), data)
		_, err = os.Stat(src)
		assert.True(t, os.IsNotExist(err))
	})
}
