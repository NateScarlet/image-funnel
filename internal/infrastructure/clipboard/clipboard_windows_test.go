//go:build windows

package clipboard

import (
	"encoding/binary"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
)

func TestAddFiles_ThenReadBack(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := tmpDir + `\test_paste.png`
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// 与前端写入的 HTML 结构一致，含 nonce meta（验证 round-trip 后 nonce 仍可读）
	html := `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="abc-123"/></head><body><pre>` + tmpFile + `</pre></body></html>`

	c := NewClipboard()

	// 调用 AddFiles 附加文件并写回 HTML
	err := c.AddFiles([]string{tmpFile}, html)
	if err != nil {
		// 无 GUI 会话或剪贴板被外部进程持续占用时剪贴板操作不可用，跳过测试
		errMsg := err.Error()
		if strings.Contains(errMsg, "OpenClipboard failed") ||
			strings.Contains(errMsg, "EmptyClipboard failed") ||
			strings.Contains(errMsg, "clipboard busy") {
			t.Skip("clipboard unavailable in non-GUI environment: " + errMsg)
		}
		t.Fatalf("AddFiles failed: %v", err)
	}

	// MuMu 等进程会读取并重写剪贴板，重写期间存在清空旧数据的空窗；
	// 有限时间内重试读回，直到目标格式出现或超时
	const readBackTimeout = 3 * time.Second
	var formats []string
	foundDrop, foundHTML := false, false
	deadline := time.Now().Add(readBackTimeout)
	for time.Now().Before(deadline) {
		formats = c.ListFormats()
		foundDrop, foundHTML = false, false
		for _, f := range formats {
			if f == "CF_HDROP" {
				foundDrop = true
			}
			if f == "HTML Format" {
				foundHTML = true
			}
		}
		if foundDrop && foundHTML {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !foundDrop {
		t.Errorf("CF_HDROP not found in clipboard formats after AddFiles. Formats: %v", formats)
	}
	if !foundHTML {
		t.Errorf("HTML Format not found in clipboard formats after AddFiles. Formats: %v", formats)
	}

	// 读回 HTML，验证 nonce 内容仍在（写入-读取 round-trip）
	gotHTML, err := c.ReadHTMLFormat()
	if err != nil {
		t.Fatalf("ReadHTMLFormat failed: %v", err)
	}
	if !strings.Contains(gotHTML, "abc-123") {
		t.Errorf("nonce missing after round-trip. HTML: %s", gotHTML)
	}
}

func TestBuildCFHTML_RoundTrip(t *testing.T) {
	html := `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="abc-123"/></head><body><pre>C:\test.png</pre></body></html>`

	blob := buildCFHTML(html)
	raw := string(blob)

	// 头部必须以 Version:0.9 开头
	if !strings.HasPrefix(raw, "Version:0.9\r\n") {
		t.Fatalf("CF_HTML should start with Version:0.9, got: %q", raw[:min(len(raw), 32)])
	}

	// 偏移字段必须为 10 位定宽（读取端 findCFHTMLOffset 依赖此格式）
	for _, key := range []string{"StartHTML:", "EndHTML:", "StartFragment:", "EndFragment:"} {
		off := findCFHTMLOffset(raw, key)
		if off < 0 {
			t.Fatalf("missing %s in header: %q", key, raw)
		}
	}

	// 读回内容必须与写入的 html 一致（round-trip）
	startHTML := findCFHTMLOffset(raw, "StartHTML:")
	endHTML := findCFHTMLOffset(raw, "EndHTML:")
	if startHTML < 0 || endHTML < 0 || startHTML >= endHTML || endHTML > len(raw) {
		t.Fatalf("invalid CF_HTML offsets: start=%d end=%d len=%d", startHTML, endHTML, len(raw))
	}
	if got := raw[startHTML:endHTML]; got != html {
		t.Errorf("round-trip mismatch:\n want: %s\n  got: %s", html, got)
	}
}

func TestBuildDropFiles_Layout(t *testing.T) {
	paths := []string{`C:\tmp\a.png`, `C:\tmp\o'brien.png`}
	data := buildDropFiles(paths)

	// DROPFILES 头布局：pFiles=20（文件列表偏移），fNC=0，fWide=1
	if got := binary.LittleEndian.Uint32(data[0:4]); got != 20 {
		t.Errorf("pFiles=%d, want 20", got)
	}
	if got := binary.LittleEndian.Uint32(data[16:20]); got != 1 {
		t.Errorf("fWide=%d, want 1", got)
	}

	// 从偏移 20 开始解析 UTF-16LE 路径列表，直到双 NULL 结尾
	off := 20
	var decoded []string
	for off+1 < len(data) {
		var u []uint16
		for off+1 < len(data) {
			c := binary.LittleEndian.Uint16(data[off:])
			off += 2
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		if len(u) == 0 {
			break // 空路径表示列表结束（双 NULL 结尾）
		}
		decoded = append(decoded, string(utf16.Decode(u)))
	}

	if len(decoded) != len(paths) {
		t.Fatalf("decoded %d paths, want %d: %v", len(decoded), len(paths), decoded)
	}
	for i, want := range paths {
		if decoded[i] != want {
			t.Errorf("path[%d]=%q, want %q", i, decoded[i], want)
		}
	}

	// 列表必须以双 NULL 结尾
	if data[len(data)-1] != 0 || data[len(data)-2] != 0 {
		t.Error("file list should end with double NULL terminator")
	}
}

func TestAddFiles_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + `\concurrent.png`
	if err := os.WriteFile(tmpFile, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	html := `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="cc"/></head><body></body></html>`

	c := NewClipboard()
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = c.AddFiles([]string{tmpFile}, html)
		}(i)
	}
	wg.Wait()

	// 无 GUI 会话时剪贴板不可用，跳过
	for _, err := range errs {
		if err == nil {
			continue
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "OpenClipboard failed") ||
			strings.Contains(errMsg, "EmptyClipboard failed") ||
			strings.Contains(errMsg, "clipboard busy") {
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
