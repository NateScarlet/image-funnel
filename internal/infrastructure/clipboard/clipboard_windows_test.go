//go:build windows

package clipboard

import (
	"os"
	"strings"
	"testing"
)

func TestAddFiles_ThenReadBack(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := tmpDir + `\test_paste.png`
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	c := NewClipboard()

	// 调用 AddFiles 附加文件
	err := c.AddFiles([]string{tmpFile})
	if err != nil {
		// 无 GUI 会话时剪贴板操作不可用，跳过测试
		// PowerShell 报错可能包含 "Requested Clipboard" 或 "Clipboard" 等关键词
		errMsg := err.Error()
		if strings.Contains(errMsg, "Requested Clipboard") ||
			strings.Contains(errMsg, "Clipboard operation did not succeed") ||
			strings.Contains(errMsg, "CLIPBRD_E") {
			t.Skip("clipboard unavailable in non-GUI environment")
		}
		t.Fatalf("AddFiles failed: %v", err)
	}

	// 验证剪贴板格式列表中包含 CF_HDROP
	formats := c.ListFormats()
	found := false
	for _, f := range formats {
		if f == "CF_HDROP" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CF_HDROP not found in clipboard formats after AddFiles. Formats: %v", formats)
	}
}
