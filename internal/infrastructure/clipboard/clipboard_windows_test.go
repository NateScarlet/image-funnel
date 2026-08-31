//go:build windows

package clipboard

import (
	"fmt"
	"os"
	"strings"
	"sync"
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

func TestBuildAddFilesScript_UsesRetryingSetDataObject(t *testing.T) {
	script := buildAddFilesScript([]string{`C:\tmp\test.png`})

	// 写入必须使用 SetDataObject 四参重试重载，应对剪贴板被其他进程瞬时占用
	wantSet := fmt.Sprintf(
		`[System.Windows.Forms.Clipboard]::SetDataObject($new,$true,%d,%d)`,
		clipboardWriteRetryTimes, clipboardWriteRetryDelayMs,
	)
	if !strings.Contains(script, wantSet) {
		t.Errorf("script should use the retrying SetDataObject overload %q, got:\n%s", wantSet, script)
	}
	if !strings.Contains(script, `$files.Add('C:\tmp\test.png');`) {
		t.Errorf("script should add the file path, got:\n%s", script)
	}
	// 重试耗尽后仍失败必须向上抛出，不能静默吞错
	if !strings.Contains(script, `Write-Error $_.Exception.Message;exit 1;`) {
		t.Errorf("script should keep fail-fast error report, got:\n%s", script)
	}
}

func TestBuildAddFilesScript_EscapesSingleQuotes(t *testing.T) {
	script := buildAddFilesScript([]string{`C:\tmp\o'brien.png`})
	// PowerShell 单引号字符串中单引号需翻倍转义
	if !strings.Contains(script, `$files.Add('C:\tmp\o''brien.png');`) {
		t.Errorf("single quotes should be doubled in PowerShell single-quoted string, got:\n%s", script)
	}
}

func TestAddFiles_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + `\concurrent.png`
	if err := os.WriteFile(tmpFile, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	c := NewClipboard()
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = c.AddFiles([]string{tmpFile})
		}(i)
	}
	wg.Wait()

	// 无 GUI 会话时剪贴板不可用，跳过
	for _, err := range errs {
		if err == nil {
			continue
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "Requested Clipboard") ||
			strings.Contains(errMsg, "Clipboard operation did not succeed") ||
			strings.Contains(errMsg, "CLIPBRD_E") {
			t.Skip("clipboard unavailable in non-GUI environment")
			return
		}
	}

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent AddFiles #%d failed: %v", i, err)
		}
	}
}
