package localfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mockRecycleBinSupported = true
	expectedAllowUndo       = false
)

func TestTrashSweepers_Empty(t *testing.T) {
	for _, sweeper := range []TrashSweeper{NewPhysicalDeleteSweeper(), NewSystemTrashSweeper()} {
		assert.NoError(t, sweeper.Sweep(nil))
		assert.NoError(t, sweeper.Sweep([]string{}))
	}
}

func TestPhysicalDeleteSweeper_DeleteFiles(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "physical_test_file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("file content"), 0644))
	assert.FileExists(t, testFile)

	// 物理删除 adapter 直接调用 Go 原生删除，不依赖平台回收站 API
	require.NoError(t, NewPhysicalDeleteSweeper().Sweep([]string{testFile}))
	assert.NoFileExists(t, testFile)
}

func TestSystemTrashSweeper_Success(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skip windows-specific recycle bin test on other platforms")
	}

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "recycle_success_file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("file content"), 0644))

	// 模拟驱动器支持回收站，并期待 fofAllowUndo 标志
	mockRecycleBinSupported = true
	expectedAllowUndo = true

	require.NoError(t, NewSystemTrashSweeper().Sweep([]string{testFile}))

	// SHFileOperationW 已被 mock，文件仍在磁盘上，手动清理
	assert.FileExists(t, testFile)
	require.NoError(t, os.Remove(testFile))
}

func TestSystemTrashSweeper_UnsupportedError(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "recycle_unsupported_file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("file content"), 0644))

	if runtime.GOOS == "windows" {
		mockRecycleBinSupported = false
	}

	err := NewSystemTrashSweeper().Sweep([]string{testFile})
	require.Error(t, err)
	// 平台不支持时由 adapter 返回带引导信息的错误，该行为在非 Windows 与不支持回收站的卷上一致
	assert.True(t, strings.Contains(err.Error(), "does not support system recycle bin") ||
		strings.Contains(err.Error(), "system recycle bin is not supported on this platform"))
	assert.Contains(t, err.Error(), "please set IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN=false")

	assert.FileExists(t, testFile)
	require.NoError(t, os.Remove(testFile))
}

func TestPhysicalDeleteSweeper_MultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("file1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("file2"), 0644))

	require.NoError(t, NewPhysicalDeleteSweeper().Sweep([]string{file1, file2}))
	assert.NoFileExists(t, file1)
	assert.NoFileExists(t, file2)
}
