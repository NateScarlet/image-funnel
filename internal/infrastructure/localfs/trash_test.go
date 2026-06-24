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

func TestMoveToRecycleBin_Empty(t *testing.T) {
	err := trashOrDelete(nil, false)
	assert.NoError(t, err)

	err = trashOrDelete([]string{}, false)
	assert.NoError(t, err)
}

func TestMoveToRecycleBin_PhysicalDelete(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "physical_test_file.txt")
	err := os.WriteFile(testFile, []byte("file content"), 0644)
	require.NoError(t, err)

	assert.FileExists(t, testFile)

	// 显式不使用系统回收站，执行物理直接删除（均直接调用 Go 原生的物理删除，不走 Windows Shell API）
	err = trashOrDelete([]string{testFile}, false)
	assert.NoError(t, err)

	// 所有平台都应已物理删除干净
	assert.NoFileExists(t, testFile)
}

func TestMoveToRecycleBin_UseSystemRecycleBin_Success(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skip windows-specific recycle bin test on other platforms")
	}

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "recycle_success_file.txt")
	err := os.WriteFile(testFile, []byte("file content"), 0644)
	require.NoError(t, err)

	// 模拟驱动器支持回收站，并期待 fofAllowUndo 标志
	mockRecycleBinSupported = true
	expectedAllowUndo = true

	err = trashOrDelete([]string{testFile}, true)
	assert.NoError(t, err)

	// 手动清理
	assert.FileExists(t, testFile)
	err = os.Remove(testFile)
	assert.NoError(t, err)
}

func TestMoveToRecycleBin_UseSystemRecycleBin_UnsupportedError(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "recycle_unsupported_file.txt")
	err := os.WriteFile(testFile, []byte("file content"), 0644)
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		// 模拟驱动器不支持回收站
		mockRecycleBinSupported = false
	}

	err = trashOrDelete([]string{testFile}, true)
	// 期望报错，并且包含相应的说明引导信息
	assert.Error(t, err)
	errStr := err.Error()
	assert.True(t, strings.Contains(errStr, "does not support system recycle bin") ||
		strings.Contains(errStr, "system recycle bin is not supported on this platform"))
	assert.True(t, strings.Contains(errStr, "please set IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN=false"))

	// 手动清理
	assert.FileExists(t, testFile)
	err = os.Remove(testFile)
	assert.NoError(t, err)
}

func TestMoveToRecycleBin_MultipleFiles_Physical(t *testing.T) {
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")

	require.NoError(t, os.WriteFile(file1, []byte("file1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("file2"), 0644))

	assert.FileExists(t, file1)
	assert.FileExists(t, file2)

	err := trashOrDelete([]string{file1, file2}, false)
	assert.NoError(t, err)

	// 所有平台都应已物理删除干净
	assert.NoFileExists(t, file1)
	assert.NoFileExists(t, file2)
}
