package util

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicSave(t *testing.T) {
	t.Run("temp file should be hidden with dot prefix", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)

		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("B"))
			assert.True(t, strings.HasPrefix(filepath.Base(file.Name()), "."),
				"临时文件应以点开头以便 Obsidian 忽略: %s", file.Name())
			assert.True(t, strings.HasSuffix(file.Name(), ".tmp"))
			assert.NotEqual(t, filepath.Join(dir, "file"), file.Name())
			return
		})
		require.NoError(t, err)
	})
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
	t.Run("should apply requested perm", func(t *testing.T) {
		var dir, err = os.MkdirTemp(os.TempDir(), "test-atomic-save-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		err = os.WriteFile(filepath.Join(dir, "file"), []byte("A"), 0o644)
		require.NoError(t, err)

		// 只读权限在 Windows 与 POSIX 下都会如实反映在 Mode().Perm() 上，
		// 因此用 0444 断言权限确实被落位文件继承
		err = AtomicSave(filepath.Join(dir, "file"), func(file *os.File) (err error) {
			_, err = file.Write([]byte("B"))
			return
		}, AtomicSaveWithPerm(0o444))
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(dir, "file"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())
	})
}

// TestAtomicSave_ConcurrentReaderSeesWholeContent 原子写的读者可见性契约：
// 写入进行中的并发读者只会读到旧全文或新全文，不会读到只写了一半的内容。
func TestAtomicSave_ConcurrentReaderSeesWholeContent(t *testing.T) {
	// 单次写入分成的块数，块间停顿用于拉长写入窗口
	const chunkCount = 16

	var name = filepath.Join(t.TempDir(), "file")
	var oldContent = strings.Repeat("A", 128*1024)
	var newContent = strings.Repeat("B", 128*1024)
	require.NoError(t, os.WriteFile(name, []byte(oldContent), 0o644))

	var (
		mu      sync.Mutex
		seen    = make(map[string]int)
		readErr error
	)
	var stop = make(chan struct{})
	var readerDone = make(chan struct{})
	var firstRead = make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(name)
			if err != nil {
				// 替换目标文件的一瞬间（Windows 上），读者的打开请求可能被系统以「占用」类错误拒绝。
				// 这与写入侧遇到的是同一类瞬时占用（判据同 isTransientRenameError），
				// 并不代表读到了不完整内容，故不计入观测也不视为失败。
				if isTransientRenameError(err) {
					continue
				}
				mu.Lock()
				readErr = err
				mu.Unlock()
				return
			}
			mu.Lock()
			seen[string(data)]++
			mu.Unlock()
			select {
			case <-firstRead:
			default:
				close(firstRead)
			}
			// 读者以毫秒级间隔重复读取：既能覆盖写入窗口内的多次采样，
			// 又留出足够的空隙让替换目标文件时的瞬时占用得以化解
			time.Sleep(time.Millisecond)
		}
	}()

	// 等读者先完成一次读取再开始写，保证本次测试确实覆盖了「写入开始前读者已在读」的时序
	<-firstRead

	var chunkSize = len(newContent) / chunkCount
	err := AtomicSave(name, func(file *os.File) (err error) {
		for i := range chunkCount {
			if _, err = file.WriteString(newContent[i*chunkSize : (i+1)*chunkSize]); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
		return
	})
	require.NoError(t, err)

	close(stop)
	<-readerDone

	mu.Lock()
	observed := maps.Clone(seen)
	gotReadErr := readErr
	mu.Unlock()

	require.NoError(t, gotReadErr, "并发读不应因瞬时占用以外的原因失败")
	assert.Contains(t, observed, oldContent, "读者应在写入完成前读到过旧全文")
	for content, count := range observed {
		if content != oldContent && content != newContent {
			t.Fatalf("读者读到了不完整内容：%d 字节（观测 %d 次）", len(content), count)
		}
	}

	finalContent, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, newContent, string(finalContent), "写入完成后目标路径应为新全文")
}
