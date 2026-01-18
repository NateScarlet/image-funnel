package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicSave(t *testing.T) {
	t.Run("should update file", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("B"))
			return
		})
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, []byte("B"), data)

		_, err = os.Stat(filepath.Join(dir, "file~"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("should update file without backup", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("B"))
			return
		}, AtomicSaveWithBackupSuffix(""))
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, []byte("B"), data)

		_, err = os.Stat(filepath.Join(dir, "file~"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("should preserve old data if error during write", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			return fmt.Errorf("test error")
		})
		require.Error(t, err, "test error")

		data, err := os.ReadFile(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, []byte("A"), data)

		_, err = os.Stat(filepath.Join(dir, "file~"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("should remove old backup", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "file~"), []byte("B"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("C"))
			return
		})
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, []byte("C"), data)

		_, err = os.Stat(filepath.Join(dir, "file~"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("should preserve existing file if rename error", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "file~"), []byte("B"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("C"))
			return
		}, func(opts *AtomicSaveOptions) {
			opts.forceRenameErrorForTest = fmt.Errorf("test error")
		})
		require.Error(t, err, "test error")

		data, err := os.ReadFile(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, []byte("A"), data)

		data, err = os.ReadFile(filepath.Join(dir, "file~"))
		require.NoError(t, err)
		assert.Equal(t, []byte("B"), data)
	})
}
